package handler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/api/middleware"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/api/response"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/game"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/realtime"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/settlement"
	"github.com/gin-gonic/gin"
)

// roomApplicationService 是 Room Handler 当前需要的房间业务能力。
type roomApplicationService interface {
	CreateRoom(host *game.Player) (*game.Room, error)
	ListRooms() []*game.Room
	GetCurrentRoom(userID int64) (game.PlayerRoomSnapshot, error)
	JoinRoom(roomID string, player *game.Player) (*game.Room, error)
	StartCurrentRoom(userID int64) (*game.Room, error)
	SubmitMove(userID int64, move game.Move) (game.RoundStateSnapshot, error)
	LeaveCurrentRoom(userID int64) (roomID string, roomDeleted bool, err error)
}

// userEventPusher 表示 RoomHandler 需要的“按用户推送实时事件”能力。
// Hub 满足这个接口，但 Handler 不依赖 Hub 的连接管理实现细节。
type userEventPusher interface {
	SendToUser(userID int64, event realtime.Event) error
}

// roundSettlementService 表示 Handler 在一局产生结果后需要的持久化结算能力。
// 使用小接口后，测试可以注入内存假实现，生产环境注入 *settlement.Service。
type roundSettlementService interface {
	Settle(ctx context.Context, command settlement.Command) (settlement.Outcome, error)
}

// RoomHandler 负责房间相关 HTTP 请求与领域对象之间的转换。
type RoomHandler struct {
	roomService       roomApplicationService
	eventPusher       userEventPusher
	settlementService roundSettlementService
}

// NewRoomHandler 显式注入房间业务、实时推送与持久化结算三个依赖。
// 单元测试不关心某个外部依赖时可以传 nil。
func NewRoomHandler(
	roomService roomApplicationService,
	eventPusher userEventPusher,
	settlementService roundSettlementService,
) *RoomHandler {
	return &RoomHandler{
		roomService:       roomService,
		eventPusher:       eventPusher,
		settlementService: settlementService,
	}
}

type roomPlayerResponse struct {
	UserID   int64  `json:"userId"`
	Username string `json:"username"`
}

type roomResponse struct {
	ID         string               `json:"id"`
	HostUserID int64                `json:"hostUserId"`
	Status     game.RoomStatus      `json:"status"`
	Players    []roomPlayerResponse `json:"players"`
}

// currentRoundResponse 只包含当前 JWT 用户有权看到的小局状态。
type currentRoundResponse struct {
	SubmittedCount int     `json:"submittedCount"`
	Submitted      bool    `json:"submitted"`
	Settled        bool    `json:"settled"`
	Move           *string `json:"move"`
	OpponentMove   *string `json:"opponentMove"`
	Result         string  `json:"result"`
}

// currentRoomResponse 用于页面刷新后恢复当前玩家的完整房间视图。
type currentRoomResponse struct {
	ID         string                `json:"id"`
	HostUserID int64                 `json:"hostUserId"`
	Status     game.RoomStatus       `json:"status"`
	Players    []roomPlayerResponse  `json:"players"`
	Round      *currentRoundResponse `json:"round"`
}

type leaveRoomResponse struct {
	RoomID      string `json:"roomId"`
	RoomDeleted bool   `json:"roomDeleted"`
}

type submitMoveRequest struct {
	Move string `json:"move" binding:"required"`
}

// submitMoveResponse 始终站在当前 JWT 用户的视角返回本局状态。
type submitMoveResponse struct {
	SubmittedCount int     `json:"submittedCount"`
	Settled        bool    `json:"settled"`
	Move           string  `json:"move"`
	OpponentMove   *string `json:"opponentMove"`
	Result         string  `json:"result"`
	ScoreChange    *int    `json:"scoreChange,omitempty"`
	ScoreAfter     *int    `json:"scoreAfter,omitempty"`
}

// roundSettledEventData 是推送给某一名玩家的本局结算视角。
// 双方收到的 Move 与 OpponentMove 顺序、Result 都不相同。
type roundSettledEventData struct {
	Move         string `json:"move"`
	OpponentMove string `json:"opponentMove"`
	Result       string `json:"result"`
	ScoreChange  *int   `json:"scoreChange,omitempty"`
	ScoreAfter   *int   `json:"scoreAfter,omitempty"`
}

// moveSubmittedEventData 只公布提交人数，绝不包含尚未结算的具体拳型。
type moveSubmittedEventData struct {
	SubmittedCount int `json:"submittedCount"`
}

// Create 创建房间，并把 JWT 代表的当前用户作为房主加入房间。
func (h *RoomHandler) Create(c *gin.Context) {
	// 这两个值由 JWT 鉴权中间件校验后写入当前请求 Context。
	userID, hasUserID := middleware.CurrentUserID(c)
	username, hasUsername := middleware.CurrentUsername(c)
	if !hasUserID || !hasUsername {
		c.JSON(http.StatusUnauthorized, response.Error(
			http.StatusUnauthorized,
			"invalid or missing access token",
		))
		return
	}

	room, err := h.roomService.CreateRoom(&game.Player{
		UserID:   userID,
		Username: username,
	})
	if err != nil {
		if errors.Is(err, game.ErrPlayerAlreadyInAnotherRoom) {
			c.JSON(http.StatusConflict, response.Error(
				http.StatusConflict,
				"player is already in a room",
			))
			return
		}

		log.Printf("create room: %v", err)
		c.JSON(http.StatusInternalServerError, response.Error(
			response.CodeInternalError,
			"internal server error",
		))
		return
	}

	c.JSON(http.StatusCreated, response.Success(toRoomResponse(room)))
}

// List 返回当前服务器内存中的全部房间。
func (h *RoomHandler) List(c *gin.Context) {
	rooms := h.roomService.ListRooms()
	roomResponses := make([]roomResponse, 0, len(rooms))
	for _, room := range rooms {
		roomResponses = append(roomResponses, toRoomResponse(room))
	}

	c.JSON(http.StatusOK, response.Success(roomResponses))
}

// Current 返回 JWT 当前用户所在房间，以及该用户有权看到的当前小局状态。
// 该接口主要用于 H5 首次进入或刷新页面后恢复状态，不承担实时事件推送。
func (h *RoomHandler) Current(c *gin.Context) {
	// 当前玩家身份只取自鉴权中间件写入的 Context，不接受客户端传入 userID。
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, response.Error(
			http.StatusUnauthorized,
			"invalid or missing access token",
		))
		return
	}

	// Service 根据用户索引找到房间，并由 Room 在读锁内生成防泄露快照。
	snapshot, err := h.roomService.GetCurrentRoom(userID)
	if err != nil {
		if errors.Is(err, game.ErrPlayerNotInRoom) {
			c.JSON(http.StatusNotFound, response.Error(
				http.StatusNotFound,
				"player is not in a room",
			))
			return
		}

		// 内存索引不一致等内部问题只记录日志，不向客户端暴露实现信息。
		log.Printf("get current room: %v", err)
		c.JSON(http.StatusInternalServerError, response.Error(
			response.CodeInternalError,
			"internal server error",
		))
		return
	}

	c.JSON(http.StatusOK, response.Success(toCurrentRoomResponse(snapshot)))
}

// Join 让 JWT 代表的当前用户加入路径参数指定的房间。
func (h *RoomHandler) Join(c *gin.Context) {
	userID, hasUserID := middleware.CurrentUserID(c)
	username, hasUsername := middleware.CurrentUsername(c)
	if !hasUserID || !hasUsername {
		c.JSON(http.StatusUnauthorized, response.Error(
			http.StatusUnauthorized,
			"invalid or missing access token",
		))
		return
	}

	room, err := h.roomService.JoinRoom(c.Param("roomID"), &game.Player{
		UserID:   userID,
		Username: username,
	})
	if err != nil {
		switch {
		case errors.Is(err, game.ErrRoomNotFound):
			c.JSON(http.StatusNotFound, response.Error(
				http.StatusNotFound,
				"room not found",
			))
		case errors.Is(err, game.ErrPlayerAlreadyInAnotherRoom):
			c.JSON(http.StatusConflict, response.Error(
				http.StatusConflict,
				"player is already in a room",
			))
		case errors.Is(err, game.ErrPlayerAlreadyInRoom):
			c.JSON(http.StatusConflict, response.Error(
				http.StatusConflict,
				"player is already in this room",
			))
		case errors.Is(err, game.ErrRoomFull):
			c.JSON(http.StatusConflict, response.Error(
				http.StatusConflict,
				"room is full",
			))
		case errors.Is(err, game.ErrRoomNotJoinable):
			c.JSON(http.StatusConflict, response.Error(
				http.StatusConflict,
				"room is not joinable",
			))
		default:
			log.Printf("join room: %v", err)
			c.JSON(http.StatusInternalServerError, response.Error(
				response.CodeInternalError,
				"internal server error",
			))
		}
		return
	}

	c.JSON(http.StatusOK, response.Success(toRoomResponse(room)))
}

// Start 让当前房主开始已经准备就绪的房间。
func (h *RoomHandler) Start(c *gin.Context) {
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, response.Error(
			http.StatusUnauthorized,
			"invalid or missing access token",
		))
		return
	}

	room, err := h.roomService.StartCurrentRoom(userID)
	if err != nil {
		switch {
		case errors.Is(err, game.ErrPlayerNotInRoom):
			c.JSON(http.StatusConflict, response.Error(
				http.StatusConflict,
				"player is not in a room",
			))
		case errors.Is(err, game.ErrOnlyHostCanStart):
			c.JSON(http.StatusForbidden, response.Error(
				http.StatusForbidden,
				"only room host can start the game",
			))
		case errors.Is(err, game.ErrRoomNotReady):
			c.JSON(http.StatusConflict, response.Error(
				http.StatusConflict,
				"room is not ready",
			))
		default:
			log.Printf("start room: %v", err)
			c.JSON(http.StatusInternalServerError, response.Error(
				response.CodeInternalError,
				"internal server error",
			))
		}
		return
	}

	c.JSON(http.StatusOK, response.Success(toRoomResponse(room)))
}

// SubmitMove 处理当前登录玩家提交本局出拳的 HTTP 请求。
//
// 这个 Handler 只负责 HTTP 层工作：
//  1. 从 JWT 鉴权中间件写入的 gin.Context 中取得当前用户 ID；
//  2. 校验 JSON 请求体，并把 rock/scissors/paper 转换成领域类型 Move；
//  3. 调用 RoomService，由业务层定位玩家所在房间并更新当前 RoundState；
//  4. 把领域错误转换成合适的 HTTP 状态码；
//  5. 按当前玩家视角返回等待状态或本局结算结果。
//
// 请求不能携带 userID，玩家身份必须来自校验通过的 JWT，防止冒充其他玩家出拳。
// 先提交者只会得到 pending，不会看到对手的选择；后提交者触发结算后才能看到双方出拳。
func (h *RoomHandler) SubmitMove(c *gin.Context) {
	// 身份校验：CurrentUserID 只能读取鉴权中间件校验 JWT 后写入的可信用户 ID。
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, response.Error(
			http.StatusUnauthorized,
			"invalid or missing access token",
		))
		return
	}

	// 请求体校验：要求客户端发送 {"move":"rock"} 这样的合法 JSON，且 move 不能为空。
	var request submitMoveRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(
			http.StatusBadRequest,
			"invalid request body",
		))
		return
	}

	// 领域值校验：JSON 格式正确不代表拳型合法，ParseMove 只接受三种约定字符串。
	move, err := game.ParseMove(request.Move)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.Error(
			http.StatusBadRequest,
			err.Error(),
		))
		return
	}

	// 业务处理：Service 根据 userID 找到当前房间，Room 再检查状态并保存本局出拳。
	roundState, err := h.roomService.SubmitMove(userID, move)
	if err != nil {
		// 将领域错误映射成对客户端有意义的 HTTP 状态和消息。
		switch {
		case errors.Is(err, game.ErrPlayerNotInRoom):
			// 用户尚未创建或加入房间，当前没有可提交出拳的目标。
			c.JSON(http.StatusConflict, response.Error(
				http.StatusConflict,
				"player is not in a room",
			))
		case errors.Is(err, game.ErrRoomNotPlaying):
			// 房主还没开始游戏，或者上一小局已经结算，当前房间不处于 playing。
			c.JSON(http.StatusConflict, response.Error(
				http.StatusConflict,
				"room is not playing",
			))
		case errors.Is(err, game.ErrMoveAlreadySubmitted):
			// 当前用户本局已经出过拳，禁止覆盖原选择，保证“落子无悔”。
			c.JSON(http.StatusConflict, response.Error(
				http.StatusConflict,
				"move already submitted",
			))
		default:
			// 未预料的内部错误只记录在服务端日志中，不把实现细节泄露给客户端。
			log.Printf("submit move: %v", err)
			c.JSON(http.StatusInternalServerError, response.Error(
				response.CodeInternalError,
				"internal server error",
			))
		}
		return
	}

	// RoundState 中保存双方数据；HTTP 响应只组装当前 JWT 用户自己的结果视角。
	// 第一名提交者尚未结算时 Map 中不存在结果，Result 的零值正好是 pending。
	result := roundState.Results[userID]
	moveResponse := submitMoveResponse{
		SubmittedCount: roundState.SubmittedCount,
		Settled:        roundState.Settled,
		Move:           move.String(),
		Result:         result.String(),
	}

	// 未结算时不返回其他玩家的出拳；第二人提交完成结算后才公开双方选择。
	if roundState.Settled {
		// 只有 MySQL 事务成功后，才允许返回并推送正式结算结果。
		settlementOutcome, err := h.persistRoundSettlement(c.Request.Context(), userID, roundState)
		if err != nil {
			log.Printf("persist round settlement: %v", err)
			c.JSON(http.StatusInternalServerError, response.Error(
				response.CodeInternalError,
				"settlement persistence failed",
			))
			return
		}

		for submittedUserID, submittedMove := range roundState.Moves {
			if submittedUserID != userID {
				opponentMove := submittedMove.String()
				moveResponse.OpponentMove = &opponentMove
				break
			}
		}
		moveResponse.ScoreChange, moveResponse.ScoreAfter = settlementScoreForUser(settlementOutcome, userID)

		// 第二名玩家提交后已经产生完整结果，此时主动把双方各自的视角推送出去。
		h.pushRoundSettled(roundState, settlementOutcome)
	} else {
		// 第一名玩家提交后只通知对手“已有一人提交”，不发送具体拳型。
		h.pushMoveSubmitted(userID, roundState.SubmittedCount)
	}

	// 无论当前是等待还是已经结算，只要本次提交成功，HTTP 状态都返回 200。
	c.JSON(http.StatusOK, response.Success(moveResponse))
}

// persistRoundSettlement 把无固定顺序的 RoundState Map 整理成稳定的双方记录，
// 再交给 SettlementService 用一个 MySQL 事务更新积分并写入战绩。
func (h *RoomHandler) persistRoundSettlement(
	ctx context.Context,
	currentUserID int64,
	roundState game.RoundStateSnapshot,
) (settlement.Outcome, error) {
	// 测试某些纯房间行为时可以不注入数据库结算服务。
	if h.settlementService == nil {
		return settlement.Outcome{}, nil
	}

	// RoundState 不保存房间号，因此从当前用户所在房间的安全快照取得 RoomID。
	roomSnapshot, err := h.roomService.GetCurrentRoom(currentUserID)
	if err != nil {
		return settlement.Outcome{}, fmt.Errorf("load room for settlement: %w", err)
	}

	userIDs := make([]int64, 0, len(roundState.Moves))
	for userID := range roundState.Moves {
		userIDs = append(userIDs, userID)
	}
	if len(userIDs) != 2 {
		return settlement.Outcome{}, fmt.Errorf("settled round contains %d moves, want 2", len(userIDs))
	}
	// Go Map 没有遍历顺序，按用户 ID 排序后，战绩中的 Player1/Player2 方向保持稳定。
	sort.Slice(userIDs, func(i, j int) bool {
		return userIDs[i] < userIDs[j]
	})

	outcome, err := h.settlementService.Settle(ctx, settlement.Command{
		RoomID:      roomSnapshot.Room.ID,
		Player1ID:   userIDs[0],
		Player1Move: roundState.Moves[userIDs[0]],
		Player2ID:   userIDs[1],
		Player2Move: roundState.Moves[userIDs[1]],
	})
	if err != nil {
		return settlement.Outcome{}, fmt.Errorf("settle round transaction: %w", err)
	}
	return outcome, nil
}

// pushMoveSubmitted 把不含拳型的提交进度推送给同房间的其他玩家。
func (h *RoomHandler) pushMoveSubmitted(submitterUserID int64, submittedCount int) {
	if h.eventPusher == nil {
		return
	}

	// 通过当前用户的安全快照取得房间成员，不让 HTTP Handler 接触 Room 内部 Map。
	snapshot, err := h.roomService.GetCurrentRoom(submitterUserID)
	if err != nil {
		log.Printf("load room for move submitted event: %v", err)
		return
	}

	for _, player := range snapshot.Room.Players {
		if player.UserID == submitterUserID {
			continue
		}
		if err := h.eventPusher.SendToUser(player.UserID, realtime.Event{
			Type: "move_submitted",
			Data: moveSubmittedEventData{SubmittedCount: submittedCount},
		}); err != nil {
			// 对方离线不影响本次出拳，重连后仍可通过 /rooms/me 恢复进度。
			log.Printf("push move submitted to user %d: %v", player.UserID, err)
		}
	}
}

// pushRoundSettled 给本局两名玩家分别组装并推送自己的结果视角。
// WebSocket 离线或发送队列异常不影响已经完成的领域结算，只记录日志供排查。
func (h *RoomHandler) pushRoundSettled(roundState game.RoundStateSnapshot, outcome settlement.Outcome) {
	if h.eventPusher == nil {
		return
	}

	for userID, result := range roundState.Results {
		ownMove, exists := roundState.Moves[userID]
		if !exists {
			continue
		}

		var opponentMove game.Move
		for otherUserID, submittedMove := range roundState.Moves {
			if otherUserID != userID {
				opponentMove = submittedMove
				break
			}
		}
		scoreChange, scoreAfter := settlementScoreForUser(outcome, userID)

		err := h.eventPusher.SendToUser(userID, realtime.Event{
			Type: "round_settled",
			Data: roundSettledEventData{
				Move:         ownMove.String(),
				OpponentMove: opponentMove.String(),
				Result:       result.String(),
				ScoreChange:  scoreChange,
				ScoreAfter:   scoreAfter,
			},
		})
		if err != nil {
			log.Printf("push round result to user %d: %v", userID, err)
		}
	}
}

// settlementScoreForUser 从事务提交后的双方结果中提取指定用户自己的积分变化。
func settlementScoreForUser(outcome settlement.Outcome, userID int64) (*int, *int) {
	switch userID {
	case outcome.Record.Player1ID:
		change := outcome.Record.Player1ScoreChange
		after := outcome.Player1Score.Score
		return &change, &after
	case outcome.Record.Player2ID:
		change := outcome.Record.Player2ScoreChange
		after := outcome.Player2Score.Score
		return &change, &after
	default:
		// 未注入 SettlementService 的纯房间测试没有真实积分数据，因此保持字段缺省。
		return nil, nil
	}
}

// LeaveCurrent 让 JWT 代表的当前用户退出所在房间。
func (h *RoomHandler) LeaveCurrent(c *gin.Context) {
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, response.Error(
			http.StatusUnauthorized,
			"invalid or missing access token",
		))
		return
	}

	roomID, roomDeleted, err := h.roomService.LeaveCurrentRoom(userID)
	if err != nil {
		if errors.Is(err, game.ErrPlayerNotInRoom) {
			c.JSON(http.StatusConflict, response.Error(
				http.StatusConflict,
				"player is not in a room",
			))
			return
		}

		log.Printf("leave current room: %v", err)
		c.JSON(http.StatusInternalServerError, response.Error(
			response.CodeInternalError,
			"internal server error",
		))
		return
	}

	c.JSON(http.StatusOK, response.Success(leaveRoomResponse{
		RoomID:      roomID,
		RoomDeleted: roomDeleted,
	}))
}

func toRoomResponse(room *game.Room) roomResponse {
	snapshot := room.Snapshot()
	playerResponses := make([]roomPlayerResponse, 0, len(snapshot.Players))
	for _, player := range snapshot.Players {
		playerResponses = append(playerResponses, roomPlayerResponse{
			UserID:   player.UserID,
			Username: player.Username,
		})
	}

	return roomResponse{
		ID:         snapshot.ID,
		HostUserID: snapshot.HostUserID,
		Status:     snapshot.Status,
		Players:    playerResponses,
	}
}

// toCurrentRoomResponse 把领域快照转换成面向前端的 JSON DTO。
func toCurrentRoomResponse(snapshot game.PlayerRoomSnapshot) currentRoomResponse {
	playerResponses := make([]roomPlayerResponse, 0, len(snapshot.Room.Players))
	for _, player := range snapshot.Room.Players {
		playerResponses = append(playerResponses, roomPlayerResponse{
			UserID:   player.UserID,
			Username: player.Username,
		})
	}

	result := currentRoomResponse{
		ID:         snapshot.Room.ID,
		HostUserID: snapshot.Room.HostUserID,
		Status:     snapshot.Room.Status,
		Players:    playerResponses,
	}
	if snapshot.Round == nil {
		return result
	}

	roundResponse := &currentRoundResponse{
		SubmittedCount: snapshot.Round.SubmittedCount,
		Submitted:      snapshot.Round.Submitted,
		Settled:        snapshot.Round.Settled,
		Result:         snapshot.Round.Result.String(),
	}
	// Move 的零值是 Rock，只有 Submitted=true 时才能把它当作真实选择返回。
	if snapshot.Round.Submitted {
		move := snapshot.Round.Move.String()
		roundResponse.Move = &move
	}
	// 对手拳只在 Room 确认已经结算后才进入 PlayerRoundSnapshot。
	if snapshot.Round.Settled {
		opponentMove := snapshot.Round.OpponentMove.String()
		roundResponse.OpponentMove = &opponentMove
	}
	result.Round = roundResponse

	return result
}
