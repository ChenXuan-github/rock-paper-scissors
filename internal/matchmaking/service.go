package matchmaking

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/game"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/realtime"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/user"
)

// StateStatus 是当前玩家相对于匹配系统的状态。
type StateStatus string

const (
	StateIdle    StateStatus = "idle"
	StateWaiting StateStatus = "waiting"
	StateMatched StateStatus = "matched"
)

// State 用于 HTTP 接口返回当前排队位置或已经匹配到的房间。
type State struct {
	Status   StateStatus
	Position int
	RoomID   string
}

// MatchFoundEventData 是服务端匹配完成后通过既有 WebSocket 推送的数据。
type MatchFoundEventData struct {
	RoomID string `json:"roomId"`
}

// roomService 是自动匹配真正需要的最小房间能力，生产环境由 *game.RoomService 实现。
type roomService interface {
	CreateRoom(host *game.Player) (*game.Room, error)
	JoinRoom(roomID string, player *game.Player) (*game.Room, error)
	GetCurrentRoom(userID int64) (game.PlayerRoomSnapshot, error)
	LeaveCurrentRoom(userID int64) (roomID string, roomDeleted bool, err error)
}

// userService 用于把队列里的 UserID 还原成创建 Player 所需的用户资料。
type userService interface {
	GetByID(ctx context.Context, id int64) (user.User, error)
}

// realtimeGateway 同时承担在线校验和匹配成功后的主动通知。
type realtimeGateway interface {
	IsOnline(userID int64) bool
	SendToUser(userID int64, event realtime.Event) error
}

// Service 编排 Redis 队列、用户资料、内存房间和 WebSocket 通知。
// Queue 只负责“谁先排队”，Service 才负责“把哪两个人放进同一间房”。
type Service struct {
	queue    Queue
	rooms    roomService
	users    userService
	realtime realtimeGateway
}

// NewService 显式注入 Redis 队列、内存房间、用户查询和 WebSocket 网关。
func NewService(queue Queue, rooms roomService, users userService, realtime realtimeGateway) *Service {
	return &Service{
		queue:    queue,
		rooms:    rooms,
		users:    users,
		realtime: realtime,
	}
}

// Join 将当前玩家放入队列，并立即尝试原子取出队首两人创建房间。
func (s *Service) Join(ctx context.Context, userID int64) (State, error) {
	if userID <= 0 {
		return State{}, ErrInvalidUserID
	}
	if s.queue == nil || s.rooms == nil || s.users == nil || s.realtime == nil {
		return State{}, errors.New("matchmaking service dependencies are incomplete")
	}
	// 匹配成功依赖 WebSocket 主动通知先排队的玩家，因此离线用户不能入队。
	if !s.realtime.IsOnline(userID) {
		return State{}, ErrPlayerOffline
	}
	if _, err := s.rooms.GetCurrentRoom(userID); err == nil {
		return State{}, ErrPlayerAlreadyInRoom
	} else if !errors.Is(err, game.ErrPlayerNotInRoom) {
		return State{}, fmt.Errorf("check player room before matchmaking: %w", err)
	}

	err := s.queue.Enqueue(ctx, QueueEntry{UserID: userID, EnqueuedAt: time.Now()})
	if err != nil && !errors.Is(err, ErrAlreadyQueued) {
		return State{}, err
	}
	// 重复点击“开始匹配”采用幂等处理：不重复排队，直接返回已有状态。
	if errors.Is(err, ErrAlreadyQueued) {
		return s.Current(ctx, userID)
	}

	matchedUserIDs, roomID, err := s.tryMatch(ctx)
	if err != nil {
		return State{}, err
	}
	// 后入队的当前请求可能恰好触发配对；若自己在结果中，可直接通过 HTTP 得到房间号。
	for _, matchedUserID := range matchedUserIDs {
		if matchedUserID == userID {
			return State{Status: StateMatched, RoomID: roomID}, nil
		}
	}
	return s.Current(ctx, userID)
}

// Cancel 取消当前玩家尚未完成的排队；已经进入房间后应使用退出房间接口。
func (s *Service) Cancel(ctx context.Context, userID int64) error {
	removed, err := s.queue.Remove(ctx, userID)
	if err != nil {
		return err
	}
	if !removed {
		return ErrNotQueued
	}
	return nil
}

// Current 先查询内存房间，再查询 Redis 队列，从而支持页面刷新后的状态恢复。
func (s *Service) Current(ctx context.Context, userID int64) (State, error) {
	if userID <= 0 {
		return State{}, ErrInvalidUserID
	}
	// 房间状态优先于队列状态：PopPair 后用户已从 Redis 消失，但已经进入内存房间。
	room, err := s.rooms.GetCurrentRoom(userID)
	if err == nil {
		return State{Status: StateMatched, RoomID: room.Room.ID}, nil
	}
	if !errors.Is(err, game.ErrPlayerNotInRoom) {
		return State{}, fmt.Errorf("read matchmaking room state: %w", err)
	}

	position, exists, err := s.queue.Position(ctx, userID)
	if err != nil {
		return State{}, err
	}
	if exists {
		return State{Status: StateWaiting, Position: position}, nil
	}
	return State{Status: StateIdle}, nil
}

// tryMatch 每次最多创建一间房；无可用玩家时返回空结果。
// 遇到已离线或已经进房的旧队列成员时会丢弃该成员，并继续尝试下一对。
func (s *Service) tryMatch(ctx context.Context) ([]int64, string, error) {
	for {
		// PopPair 的“检查人数 + 取出两人”由 Redis Lua 原子完成，不会重复匹配同一玩家。
		pair, err := s.queue.PopPair(ctx)
		if err != nil {
			return nil, "", err
		}
		if len(pair) == 0 {
			return nil, "", nil
		}

		entries, players, err := s.loadAvailablePlayers(ctx, pair)
		if err != nil {
			// 临时基础设施错误不能让玩家凭空消失，按原时间放回队列。
			if requeueErr := s.queue.Requeue(ctx, pair...); requeueErr != nil {
				return nil, "", fmt.Errorf("%v; requeue pair: %w", err, requeueErr)
			}
			return nil, "", err
		}
		if len(players) < 2 {
			// 离线、已进房或已删除的账号不再入队；仍有效的一人保留原排队顺序。
			if err := s.queue.Requeue(ctx, entries...); err != nil {
				return nil, "", err
			}
			continue
		}

		// 队列中等待更久的第一名玩家创建房间，因此自然成为当前房主。
		room, err := s.rooms.CreateRoom(players[0])
		if err != nil {
			if errors.Is(err, game.ErrPlayerAlreadyInAnotherRoom) {
				if requeueErr := s.queue.Requeue(ctx, entries[1]); requeueErr != nil {
					return nil, "", requeueErr
				}
				continue
			}
			if requeueErr := s.queue.Requeue(ctx, entries...); requeueErr != nil {
				return nil, "", fmt.Errorf("create matched room: %v; requeue pair: %w", err, requeueErr)
			}
			return nil, "", fmt.Errorf("create matched room: %w", err)
		}

		if _, err := s.rooms.JoinRoom(room.ID, players[1]); err != nil {
			// 创建房间只成功了一半时先让房主退出，避免留下只有一人的自动匹配房间。
			_, _, rollbackErr := s.rooms.LeaveCurrentRoom(players[0].UserID)
			if requeueErr := s.queue.Requeue(ctx, entries...); requeueErr != nil {
				return nil, "", fmt.Errorf("join matched room: %v; rollback: %v; requeue: %w", err, rollbackErr, requeueErr)
			}
			if rollbackErr != nil {
				return nil, "", fmt.Errorf("join matched room: %v; rollback room: %w", err, rollbackErr)
			}
			if errors.Is(err, game.ErrPlayerAlreadyInAnotherRoom) {
				// 下一轮会清掉已经进入其他房间的玩家，并保留仍然有效的另一人。
				continue
			}
			return nil, "", fmt.Errorf("join matched room: %w", err)
		}

		matchedUserIDs := []int64{players[0].UserID, players[1].UserID}
		event := realtime.Event{
			Type: "match_found",
			Data: MatchFoundEventData{RoomID: room.ID},
		}
		for _, matchedUserID := range matchedUserIDs {
			if err := s.realtime.SendToUser(matchedUserID, event); err != nil {
				// 房间已经建立成功，推送失败不回滚；客户端可通过状态接口恢复房间号。
				log.Printf("push match_found to user %d: %v", matchedUserID, err)
			}
		}
		return matchedUserIDs, room.ID, nil
	}
}

func (s *Service) loadAvailablePlayers(
	ctx context.Context,
	pair []QueueEntry,
) ([]QueueEntry, []*game.Player, error) {
	// PopPair 与这里的复查之间存在时间窗口：玩家可能掉线、进了手动房间或账号被删除。
	// 所以 Redis 中曾经有效并不代表现在仍可用于创建房间。
	entries := make([]QueueEntry, 0, 2)
	players := make([]*game.Player, 0, 2)
	for _, entry := range pair {
		if !s.realtime.IsOnline(entry.UserID) {
			continue
		}
		if _, err := s.rooms.GetCurrentRoom(entry.UserID); err == nil {
			continue
		} else if !errors.Is(err, game.ErrPlayerNotInRoom) {
			return nil, nil, fmt.Errorf("check queued player room: %w", err)
		}

		foundUser, err := s.users.GetByID(ctx, entry.UserID)
		if err != nil {
			if errors.Is(err, user.ErrUserNotFound) {
				continue
			}
			return nil, nil, fmt.Errorf("load queued player %d: %w", entry.UserID, err)
		}
		entries = append(entries, entry)
		players = append(players, &game.Player{
			UserID:   foundUser.ID,
			Username: foundUser.Username,
		})
	}
	return entries, players, nil
}
