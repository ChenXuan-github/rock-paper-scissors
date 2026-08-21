package social

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/game"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/realtime"
)

var (
	ErrGameInviteeOffline         = errors.New("game invitation invitee is offline")
	ErrGameInviterOffline         = errors.New("game invitation inviter is offline")
	ErrGameInvitationPlayerInRoom = errors.New("game invitation player is already in a room")
)

// gameInvitationRoomService 只暴露邀战流程真正需要的房间能力。
// 小接口让 Service 不依赖具体 RoomService，也便于单元测试替换为内存实现。
type gameInvitationRoomService interface {
	CreateRoom(host *game.Player) (*game.Room, error)
	JoinRoom(roomID string, player *game.Player) (*game.Room, error)
	GetCurrentRoom(userID int64) (game.PlayerRoomSnapshot, error)
	LeaveCurrentRoom(userID int64) (roomID string, roomDeleted bool, err error)
}

// gameInvitationRealtime 抽象在线查询和单用户推送；生产实现由 WebSocket Hub 提供。
type gameInvitationRealtime interface {
	IsOnline(userID int64) bool
	SendToUser(userID int64, event realtime.Event) error
}

// GameInvitationEventData 是邀请创建时推给被邀请者的数据。
type GameInvitationEventData struct {
	InvitationID string      `json:"invitationId"`
	Inviter      UserSummary `json:"inviter"`
	ExpiresAt    string      `json:"expiresAt"`
}

// GameInvitationResultEventData 是邀请结束时推给另一方的数据。
type GameInvitationResultEventData struct {
	InvitationID string               `json:"invitationId"`
	Status       GameInvitationStatus `json:"status"`
	RoomID       string               `json:"roomId,omitempty"`
}

// GameInvitationService 编排好友校验、在线状态、邀请状态、房间和实时通知。
type GameInvitationService struct {
	manager     *GameInvitationManager
	friendships FriendshipRepository
	users       userQueryService
	rooms       gameInvitationRoomService
	realtime    gameInvitationRealtime
}

// NewGameInvitationService 显式注入邀请流程需要的五个协作者。
func NewGameInvitationService(
	manager *GameInvitationManager,
	friendships FriendshipRepository,
	users userQueryService,
	rooms gameInvitationRoomService,
	realtimeGateway gameInvitationRealtime,
) *GameInvitationService {
	return &GameInvitationService{
		manager:     manager,
		friendships: friendships,
		users:       users,
		rooms:       rooms,
		realtime:    realtimeGateway,
	}
}

// Invite 创建在线好友对战邀请，并实时推送给被邀请者。
func (s *GameInvitationService) Invite(
	ctx context.Context,
	inviterID, inviteeID int64,
) (GameInvitation, error) {
	// 构造函数虽然会注入依赖，但显式检查可以让错误配置返回可识别错误而不是 nil panic。
	if !s.ready() {
		return GameInvitation{}, ErrSocialServiceNotInitialized
	}
	if _, _, err := canonicalUserPair(inviterID, inviteeID); err != nil {
		return GameInvitation{}, err
	}

	// 邀战权限建立在当前好友关系之上，不能只相信前端好友页面传来的用户 ID。
	areFriends, err := s.friendships.Exists(ctx, inviterID, inviteeID)
	if err != nil {
		return GameInvitation{}, fmt.Errorf("check invitation friendship: %w", err)
	}
	if !areFriends {
		return GameInvitation{}, ErrFriendshipNotFound
	}
	// 当前产品只允许邀请在线用户；Hub 中存在有效 WebSocket 才算在线。
	if !s.realtime.IsOnline(inviteeID) {
		return GameInvitation{}, ErrGameInviteeOffline
	}
	// 任意一方已经在房间时都不能再创建新的好友邀战，避免一名玩家占用两个房间。
	if err := s.ensureNotInRoom(inviterID); err != nil {
		return GameInvitation{}, err
	}
	if err := s.ensureNotInRoom(inviteeID); err != nil {
		return GameInvitation{}, err
	}

	// 一次 IN 查询读取双方安全摘要，避免分别访问 users 表两次。
	usersByID, err := s.users.GetByIDs(ctx, []int64{inviterID, inviteeID})
	if err != nil {
		return GameInvitation{}, fmt.Errorf("load invitation users: %w", err)
	}
	inviter, inviterExists := usersByID[inviterID]
	invitee, inviteeExists := usersByID[inviteeID]
	if !inviterExists || !inviteeExists {
		return GameInvitation{}, ErrSocialUserNotFound
	}

	// Manager 在 Mutex 临界区内完成用户对判重、随机 ID、TTL 和两张 map 的写入。
	invitation, err := s.manager.Create(
		UserSummary{ID: inviter.ID, Username: inviter.Username},
		UserSummary{ID: invitee.ID, Username: invitee.Username},
	)
	if err != nil {
		return GameInvitation{}, err
	}

	// HTTP 响应只返回邀请者；被邀请者通过已有 WebSocket 被服务端主动通知。
	err = s.realtime.SendToUser(inviteeID, realtime.Event{
		Type: "game_invitation_received",
		Data: GameInvitationEventData{
			InvitationID: invitation.ID,
			Inviter: UserSummary{
				ID:       invitation.InviterID,
				Username: invitation.InviterUsername,
			},
			ExpiresAt: invitation.ExpiresAt.Format(time.RFC3339),
		},
	})
	if err != nil {
		// 在线检查和真正投递之间可能掉线；撤销刚创建的邀请，允许稍后重新发送。
		_, _ = s.manager.Cancel(invitation.ID, inviterID)
		if errors.Is(err, realtime.ErrUserOffline) {
			return GameInvitation{}, ErrGameInviteeOffline
		}
		return GameInvitation{}, fmt.Errorf("push game invitation: %w", err)
	}
	return invitation, nil
}

// Accept 接受邀请，自动创建以邀请者为房主的 1v1 房间并把双方加入。
func (s *GameInvitationService) Accept(
	ctx context.Context,
	invitationID string,
	currentUserID int64,
) (GameInvitation, error) {
	if !s.ready() {
		return GameInvitation{}, ErrSocialServiceNotInitialized
	}
	// 先读取快照；FindByID 会顺便把超过 TTL 的 pending 邀请惰性标记为 expired。
	invitation, err := s.manager.FindByID(invitationID)
	if err != nil {
		return GameInvitation{}, err
	}
	// currentUserID 来自 JWT 上下文，只有被邀请者本人有权接受。
	if invitation.InviteeID != currentUserID {
		return GameInvitation{}, ErrGameInvitationNotInvitee
	}
	if invitation.Status == GameInvitationExpired {
		return GameInvitation{}, ErrGameInvitationExpired
	}
	if invitation.Status != GameInvitationPending {
		return GameInvitation{}, ErrGameInvitationNotPending
	}
	// 从发出到接受之间状态可能变化，因此这里不能依赖 Invite 阶段的旧校验结果。
	if !s.realtime.IsOnline(invitation.InviterID) {
		return GameInvitation{}, ErrGameInviterOffline
	}

	// 接受前重新检查好友边，防止双方已删除好友却仍使用旧邀请建房。
	areFriends, err := s.friendships.Exists(ctx, invitation.InviterID, invitation.InviteeID)
	if err != nil {
		return GameInvitation{}, fmt.Errorf("recheck invitation friendship: %w", err)
	}
	if !areFriends {
		return GameInvitation{}, ErrFriendshipNotFound
	}

	// 以邀请者为房主创建房间；这是当前产品规则，不由接受方请求体决定。
	room, err := s.rooms.CreateRoom(&game.Player{
		UserID:   invitation.InviterID,
		Username: invitation.InviterUsername,
	})
	if err != nil {
		if errors.Is(err, game.ErrPlayerAlreadyInAnotherRoom) {
			return GameInvitation{}, ErrGameInvitationPlayerInRoom
		}
		return GameInvitation{}, fmt.Errorf("create invited room: %w", err)
	}
	// 第二步把被邀请者加入刚创建的同一个房间，形成完整 1v1 房间。
	if _, err := s.rooms.JoinRoom(room.ID, &game.Player{
		UserID:   invitation.InviteeID,
		Username: invitation.InviteeUsername,
	}); err != nil {
		// 邀请者房间创建成功但被邀请者加入失败时，立即删除这个半成品房间。
		_, _, _ = s.rooms.LeaveCurrentRoom(invitation.InviterID)
		if errors.Is(err, game.ErrPlayerAlreadyInAnotherRoom) {
			return GameInvitation{}, ErrGameInvitationPlayerInRoom
		}
		return GameInvitation{}, fmt.Errorf("join invited room: %w", err)
	}

	// 房间准备好后才把邀请置为 accepted，确保 accepted 一定对应可进入的 roomId。
	accepted, err := s.manager.Accept(invitation.ID, currentUserID, room.ID)
	if err != nil {
		// 邀请可能在房间创建期间被撤销或过期；把双方都移出，恢复房间系统原状。
		_, _, _ = s.rooms.LeaveCurrentRoom(invitation.InviteeID)
		_, _, _ = s.rooms.LeaveCurrentRoom(invitation.InviterID)
		return GameInvitation{}, err
	}

	// 接受方从本次 HTTP 响应得到 roomId；邀请方依靠 WebSocket 事件进入相同房间。
	s.pushResult(invitation.InviterID, accepted)
	return accepted, nil
}

// Reject 让被邀请者拒绝邀请，并通知邀请者。
func (s *GameInvitationService) Reject(invitationID string, currentUserID int64) (GameInvitation, error) {
	if !s.ready() {
		return GameInvitation{}, ErrSocialServiceNotInitialized
	}
	rejected, err := s.manager.Reject(invitationID, currentUserID)
	if err != nil {
		return GameInvitation{}, err
	}
	// 拒绝已经成功写入内存状态；推送失败不能把业务结果重新改回 pending。
	s.pushResult(rejected.InviterID, rejected)
	return rejected, nil
}

// Cancel 让邀请者撤销邀请，并通知被邀请者。
func (s *GameInvitationService) Cancel(invitationID string, currentUserID int64) (GameInvitation, error) {
	if !s.ready() {
		return GameInvitation{}, ErrSocialServiceNotInitialized
	}
	cancelled, err := s.manager.Cancel(invitationID, currentUserID)
	if err != nil {
		return GameInvitation{}, err
	}
	// 被邀请者可能刚好离线，离线通知失败属于正常情况，不影响撤销结果。
	s.pushResult(cancelled.InviteeID, cancelled)
	return cancelled, nil
}

// ensureNotInRoom 把房间模块的错误翻译成社交邀战领域错误。
func (s *GameInvitationService) ensureNotInRoom(userID int64) error {
	_, err := s.rooms.GetCurrentRoom(userID)
	if err == nil {
		return ErrGameInvitationPlayerInRoom
	}
	if errors.Is(err, game.ErrPlayerNotInRoom) {
		return nil
	}
	return fmt.Errorf("check invitation player room: %w", err)
}

// pushResult 根据最终状态拼出事件名，例如 game_invitation_accepted。
// 实时通知属于事务外的附加效果：用户离线或推送失败不能回滚已经完成的状态与房间。
func (s *GameInvitationService) pushResult(userID int64, invitation GameInvitation) {
	err := s.realtime.SendToUser(userID, realtime.Event{
		Type: "game_invitation_" + string(invitation.Status),
		Data: GameInvitationResultEventData{
			InvitationID: invitation.ID,
			Status:       invitation.Status,
			RoomID:       invitation.RoomID,
		},
	})
	if err != nil && !errors.Is(err, realtime.ErrUserOffline) {
		log.Printf("push game invitation result to user %d: %v", userID, err)
	}
}

// ready 集中检查所有协作者，避免业务方法各自遗漏某个 nil 依赖。
func (s *GameInvitationService) ready() bool {
	return s.manager != nil && s.friendships != nil && s.users != nil && s.rooms != nil && s.realtime != nil
}
