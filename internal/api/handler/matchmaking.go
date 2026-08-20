package handler

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/api/middleware"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/api/response"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/matchmaking"
	"github.com/gin-gonic/gin"
)

// matchmakingApplicationService 是匹配 HTTP 层依赖的最小业务接口。
type matchmakingApplicationService interface {
	Join(ctx context.Context, userID int64) (matchmaking.State, error)
	Cancel(ctx context.Context, userID int64) error
	Current(ctx context.Context, userID int64) (matchmaking.State, error)
}

// MatchmakingHandler 把当前 JWT 用户的开始匹配、取消和状态查询转换成业务调用。
type MatchmakingHandler struct {
	service matchmakingApplicationService
}

// NewMatchmakingHandler 注入匹配业务服务。
func NewMatchmakingHandler(service matchmakingApplicationService) *MatchmakingHandler {
	return &MatchmakingHandler{service: service}
}

// matchmakingStateResponse 根据状态按需返回排队位置或匹配到的房间号。
type matchmakingStateResponse struct {
	Status   matchmaking.StateStatus `json:"status"`
	Position *int                    `json:"position,omitempty"`
	RoomID   string                  `json:"roomId,omitempty"`
}

// Join 把当前登录玩家加入 Redis 匹配队列，并尝试匹配队首两名玩家。
func (h *MatchmakingHandler) Join(c *gin.Context) {
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		writeMatchmakingUnauthorized(c)
		return
	}

	state, err := h.service.Join(c.Request.Context(), userID)
	if err != nil {
		switch {
		case errors.Is(err, matchmaking.ErrPlayerOffline):
			c.JSON(http.StatusConflict, response.Error(
				http.StatusConflict,
				"websocket connection is required before matchmaking",
			))
		case errors.Is(err, matchmaking.ErrPlayerAlreadyInRoom):
			c.JSON(http.StatusConflict, response.Error(
				http.StatusConflict,
				"player is already in a room",
			))
		default:
			log.Printf("join matchmaking queue: %v", err)
			c.JSON(http.StatusInternalServerError, response.Error(
				response.CodeInternalError,
				"internal server error",
			))
		}
		return
	}

	c.JSON(http.StatusOK, response.Success(toMatchmakingStateResponse(state)))
}

// Cancel 从 Redis 队列删除当前玩家；已经匹配进房间的玩家应调用房间退出接口。
func (h *MatchmakingHandler) Cancel(c *gin.Context) {
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		writeMatchmakingUnauthorized(c)
		return
	}

	if err := h.service.Cancel(c.Request.Context(), userID); err != nil {
		if errors.Is(err, matchmaking.ErrNotQueued) {
			c.JSON(http.StatusConflict, response.Error(
				http.StatusConflict,
				"player is not in matchmaking queue",
			))
			return
		}
		log.Printf("cancel matchmaking: %v", err)
		c.JSON(http.StatusInternalServerError, response.Error(
			response.CodeInternalError,
			"internal server error",
		))
		return
	}

	c.JSON(http.StatusOK, response.Success(matchmakingStateResponse{
		Status: matchmaking.StateIdle,
	}))
}

// Current 返回 idle、waiting 或 matched，供刷新页面或WebSocket消息丢失后恢复状态。
func (h *MatchmakingHandler) Current(c *gin.Context) {
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		writeMatchmakingUnauthorized(c)
		return
	}

	state, err := h.service.Current(c.Request.Context(), userID)
	if err != nil {
		log.Printf("read matchmaking state: %v", err)
		c.JSON(http.StatusInternalServerError, response.Error(
			response.CodeInternalError,
			"internal server error",
		))
		return
	}
	c.JSON(http.StatusOK, response.Success(toMatchmakingStateResponse(state)))
}

// toMatchmakingStateResponse 只在 waiting 状态返回 Position，利用 omitempty 保持 JSON 简洁。
func toMatchmakingStateResponse(state matchmaking.State) matchmakingStateResponse {
	result := matchmakingStateResponse{
		Status: state.Status,
		RoomID: state.RoomID,
	}
	if state.Status == matchmaking.StateWaiting {
		position := state.Position
		result.Position = &position
	}
	return result
}

// writeMatchmakingUnauthorized 统一三个接口的鉴权失败响应。
func writeMatchmakingUnauthorized(c *gin.Context) {
	c.JSON(http.StatusUnauthorized, response.Error(
		http.StatusUnauthorized,
		"invalid or missing access token",
	))
}
