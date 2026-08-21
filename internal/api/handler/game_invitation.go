package handler

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/api/response"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/social"
	"github.com/gin-gonic/gin"
)

// gameInvitationApplicationService 是 Handler 所需的最小邀战能力，便于使用 fake Service 测试 HTTP 层。
type gameInvitationApplicationService interface {
	Invite(ctx context.Context, inviterID, inviteeID int64) (social.GameInvitation, error)
	Accept(ctx context.Context, invitationID string, currentUserID int64) (social.GameInvitation, error)
	Reject(invitationID string, currentUserID int64) (social.GameInvitation, error)
	Cancel(invitationID string, currentUserID int64) (social.GameInvitation, error)
}

// GameInvitationHandler 把好友对战邀请的 HTTP 操作交给业务服务。
type GameInvitationHandler struct {
	service gameInvitationApplicationService
}

func NewGameInvitationHandler(service gameInvitationApplicationService) *GameInvitationHandler {
	return &GameInvitationHandler{service: service}
}

type createGameInvitationRequest struct {
	// inviterID 来自 JWT；请求体只允许指定将要邀请的好友。
	InviteeID int64 `json:"inviteeId" binding:"required,gt=0"`
}

type gameInvitationUserResponse struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

type gameInvitationResponse struct {
	ID          string                      `json:"id"`
	Inviter     gameInvitationUserResponse  `json:"inviter"`
	Invitee     gameInvitationUserResponse  `json:"invitee"`
	Status      social.GameInvitationStatus `json:"status"`
	RoomID      string                      `json:"roomId,omitempty"`
	CreatedAt   time.Time                   `json:"createdAt"`
	ExpiresAt   time.Time                   `json:"expiresAt"`
	RespondedAt *time.Time                  `json:"respondedAt"`
}

// Invite 邀请 JSON 中的在线好友进行对战，邀请者身份来自 JWT。
func (h *GameInvitationHandler) Invite(c *gin.Context) {
	currentUserID, ok := socialCurrentUserID(c)
	if !ok {
		return
	}
	var request createGameInvitationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "invalid request body"))
		return
	}

	invitation, err := h.service.Invite(c.Request.Context(), currentUserID, request.InviteeID)
	if err != nil {
		writeGameInvitationError(c, "invite friend to game", err)
		return
	}
	c.JSON(http.StatusCreated, response.Success(toGameInvitationResponse(invitation)))
}

// Accept 让 JWT 当前用户接受邀请；成功响应中的 roomId 用于客户端进入房间页。
func (h *GameInvitationHandler) Accept(c *gin.Context) {
	currentUserID, invitationID, ok := gameInvitationOperationIdentity(c)
	if !ok {
		return
	}
	invitation, err := h.service.Accept(c.Request.Context(), invitationID, currentUserID)
	if err != nil {
		writeGameInvitationError(c, "accept game invitation", err)
		return
	}
	c.JSON(http.StatusOK, response.Success(toGameInvitationResponse(invitation)))
}

// Reject 让 JWT 当前用户拒绝收到的邀请。
func (h *GameInvitationHandler) Reject(c *gin.Context) {
	currentUserID, invitationID, ok := gameInvitationOperationIdentity(c)
	if !ok {
		return
	}
	invitation, err := h.service.Reject(invitationID, currentUserID)
	if err != nil {
		writeGameInvitationError(c, "reject game invitation", err)
		return
	}
	c.JSON(http.StatusOK, response.Success(toGameInvitationResponse(invitation)))
}

// Cancel 让 JWT 当前用户撤销自己发出的邀请。
func (h *GameInvitationHandler) Cancel(c *gin.Context) {
	currentUserID, invitationID, ok := gameInvitationOperationIdentity(c)
	if !ok {
		return
	}
	invitation, err := h.service.Cancel(invitationID, currentUserID)
	if err != nil {
		writeGameInvitationError(c, "cancel game invitation", err)
		return
	}
	c.JSON(http.StatusOK, response.Success(toGameInvitationResponse(invitation)))
}

// gameInvitationOperationIdentity 提取可信当前用户，并校验 URL 中的随机邀请 ID 非空。
func gameInvitationOperationIdentity(c *gin.Context) (int64, string, bool) {
	currentUserID, ok := socialCurrentUserID(c)
	if !ok {
		return 0, "", false
	}
	invitationID := strings.TrimSpace(c.Param("invitationID"))
	if invitationID == "" {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "invalid game invitation id"))
		return 0, "", false
	}
	return currentUserID, invitationID, true
}

// toGameInvitationResponse 将内部内存模型转换为稳定的小驼峰 JSON 响应，避免直接暴露领域结构。
func toGameInvitationResponse(invitation social.GameInvitation) gameInvitationResponse {
	return gameInvitationResponse{
		ID: invitation.ID,
		Inviter: gameInvitationUserResponse{
			ID:       invitation.InviterID,
			Username: invitation.InviterUsername,
		},
		Invitee: gameInvitationUserResponse{
			ID:       invitation.InviteeID,
			Username: invitation.InviteeUsername,
		},
		Status:      invitation.Status,
		RoomID:      invitation.RoomID,
		CreatedAt:   invitation.CreatedAt,
		ExpiresAt:   invitation.ExpiresAt,
		RespondedAt: invitation.RespondedAt,
	}
}

// writeGameInvitationError 将校验、权限、状态冲突和服务器错误分别映射为 4xx/5xx。
func writeGameInvitationError(c *gin.Context, operation string, err error) {
	switch {
	case errors.Is(err, social.ErrInvalidUserPair),
		errors.Is(err, social.ErrInvalidGameInvitation):
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
	case errors.Is(err, social.ErrGameInvitationNotFound),
		errors.Is(err, social.ErrSocialUserNotFound):
		c.JSON(http.StatusNotFound, response.Error(http.StatusNotFound, err.Error()))
	case errors.Is(err, social.ErrGameInvitationNotInvitee),
		errors.Is(err, social.ErrGameInvitationNotInviter),
		errors.Is(err, social.ErrFriendshipNotFound):
		c.JSON(http.StatusForbidden, response.Error(http.StatusForbidden, err.Error()))
	case errors.Is(err, social.ErrGameInvitationPending),
		errors.Is(err, social.ErrGameInvitationNotPending),
		errors.Is(err, social.ErrGameInvitationExpired),
		errors.Is(err, social.ErrGameInviteeOffline),
		errors.Is(err, social.ErrGameInviterOffline),
		errors.Is(err, social.ErrGameInvitationPlayerInRoom):
		c.JSON(http.StatusConflict, response.Error(http.StatusConflict, err.Error()))
	default:
		log.Printf("%s: %v", operation, err)
		c.JSON(http.StatusInternalServerError, response.Error(
			response.CodeInternalError,
			"internal server error",
		))
	}
}
