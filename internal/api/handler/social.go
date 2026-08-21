package handler

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/api/middleware"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/api/response"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/realtime"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/score"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/social"
	"github.com/gin-gonic/gin"
)

// socialApplicationService 是社交 HTTP 层需要的完整业务能力。
type socialApplicationService interface {
	SendFriendRequest(ctx context.Context, requesterID, receiverID int64) (social.FriendRequest, error)
	AcceptFriendRequest(ctx context.Context, requestID, currentUserID int64) (social.AcceptFriendRequestResult, error)
	RejectFriendRequest(ctx context.Context, requestID, currentUserID int64) (social.FriendRequest, error)
	CancelFriendRequest(ctx context.Context, requestID, currentUserID int64) (social.FriendRequest, error)
	RemoveFriend(ctx context.Context, currentUserID, friendUserID int64) error
	ListFriends(ctx context.Context, currentUserID int64) ([]social.UserSummary, error)
	ListIncomingFriendRequests(
		ctx context.Context,
		currentUserID int64,
		status social.FriendRequestStatus,
		limit, offset int,
	) ([]social.FriendRequestDetail, error)
	ListOutgoingFriendRequests(
		ctx context.Context,
		currentUserID int64,
		status social.FriendRequestStatus,
		limit, offset int,
	) ([]social.FriendRequestDetail, error)
}

// socialScoreQueryService 提供好友卡片所需的批量积分查询，避免逐个好友访问数据库。
type socialScoreQueryService interface {
	GetByUserIDs(ctx context.Context, userIDs []int64) (map[int64]score.PlayerScore, error)
}

// socialRealtimeGateway 同时提供社交事件推送和当前连接状态查询。
type socialRealtimeGateway interface {
	userEventPusher
	OnlineUsers(userIDs []int64) map[int64]bool
}

// SocialHandler 负责好友关系和好友申请的 HTTP 参数、身份及响应转换。
type SocialHandler struct {
	service      socialApplicationService
	scoreService socialScoreQueryService
	realtime     socialRealtimeGateway
}

// NewSocialHandler 注入社交业务、积分查询和现有 WebSocket Hub。
func NewSocialHandler(
	service socialApplicationService,
	scoreService socialScoreQueryService,
	realtimeGateway socialRealtimeGateway,
) *SocialHandler {
	return &SocialHandler{
		service:      service,
		scoreService: scoreService,
		realtime:     realtimeGateway,
	}
}

type sendFriendRequestRequest struct {
	// 发送者不从 JSON 接收，而是由 JWT 中间件写入 gin.Context，防止冒充其他用户。
	ReceiverID int64 `json:"receiverId" binding:"required,gt=0"`
}

type socialUserResponse struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

// friendResponse 是前端好友卡片需要的当前快照。
type friendResponse struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Score    int    `json:"score"`
	Online   bool   `json:"online"`
}

type friendRequestResponse struct {
	ID          int64                      `json:"id"`
	Requester   socialUserResponse         `json:"requester"`
	Receiver    socialUserResponse         `json:"receiver"`
	Status      social.FriendRequestStatus `json:"status"`
	CreatedAt   time.Time                  `json:"createdAt"`
	UpdatedAt   time.Time                  `json:"updatedAt"`
	RespondedAt *time.Time                 `json:"respondedAt"`
}

// friendRequestMutationResponse 用于创建和状态变化响应；列表查询才额外批量补充用户名。
type friendRequestMutationResponse struct {
	ID          int64                      `json:"id"`
	RequesterID int64                      `json:"requesterId"`
	ReceiverID  int64                      `json:"receiverId"`
	Status      social.FriendRequestStatus `json:"status"`
	CreatedAt   time.Time                  `json:"createdAt"`
	UpdatedAt   time.Time                  `json:"updatedAt"`
	RespondedAt *time.Time                 `json:"respondedAt"`
}

type friendshipResponse struct {
	UserIDLow  int64     `json:"userIdLow"`
	UserIDHigh int64     `json:"userIdHigh"`
	CreatedAt  time.Time `json:"createdAt"`
}

type acceptFriendRequestResponse struct {
	Request    friendRequestMutationResponse `json:"request"`
	Friendship friendshipResponse            `json:"friendship"`
}

// socialChangedEventData 只携带定位变化所需的 ID；客户端收到后重新查询 MySQL 真相源对应的接口。
type socialChangedEventData struct {
	RequestID int64 `json:"requestId,omitempty"`
	UserID    int64 `json:"userId,omitempty"`
}

// SendFriendRequest 以 JWT 当前用户为发送者，向 JSON 中的目标用户发送申请。
func (h *SocialHandler) SendFriendRequest(c *gin.Context) {
	currentUserID, ok := socialCurrentUserID(c)
	if !ok {
		return
	}

	var request sendFriendRequestRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "invalid request body"))
		return
	}

	created, err := h.service.SendFriendRequest(c.Request.Context(), currentUserID, request.ReceiverID)
	if err != nil {
		writeSocialError(c, "send friend request", err)
		return
	}

	// 变更接口返回双方 ID；需要用户名时由申请列表接口通过批量查询统一补充。
	h.pushSocialEvent(request.ReceiverID, "friend_request_received", socialChangedEventData{
		RequestID: created.ID,
		UserID:    currentUserID,
	})
	c.JSON(http.StatusCreated, response.Success(toFriendRequestMutationResponse(created)))
}

// AcceptFriendRequest 让 JWT 当前用户接受路径参数指定的申请。
func (h *SocialHandler) AcceptFriendRequest(c *gin.Context) {
	currentUserID, requestID, ok := socialOperationIdentity(c)
	if !ok {
		return
	}

	result, err := h.service.AcceptFriendRequest(c.Request.Context(), requestID, currentUserID)
	if err != nil {
		writeSocialError(c, "accept friend request", err)
		return
	}
	h.pushSocialEvent(result.Request.RequesterID, "friend_request_accepted", socialChangedEventData{
		RequestID: result.Request.ID,
		UserID:    currentUserID,
	})

	c.JSON(http.StatusOK, response.Success(acceptFriendRequestResponse{
		Request: toFriendRequestMutationResponse(result.Request),
		Friendship: friendshipResponse{
			UserIDLow:  result.Friendship.UserIDLow,
			UserIDHigh: result.Friendship.UserIDHigh,
			CreatedAt:  result.Friendship.CreatedAt,
		},
	}))
}

// RejectFriendRequest 让 JWT 当前用户拒绝路径参数指定的申请。
func (h *SocialHandler) RejectFriendRequest(c *gin.Context) {
	currentUserID, requestID, ok := socialOperationIdentity(c)
	if !ok {
		return
	}

	rejected, err := h.service.RejectFriendRequest(c.Request.Context(), requestID, currentUserID)
	if err != nil {
		writeSocialError(c, "reject friend request", err)
		return
	}
	h.pushSocialEvent(rejected.RequesterID, "friend_request_rejected", socialChangedEventData{
		RequestID: rejected.ID,
		UserID:    currentUserID,
	})
	c.JSON(http.StatusOK, response.Success(toFriendRequestMutationResponse(rejected)))
}

// CancelFriendRequest 让 JWT 当前用户撤销自己发出的申请。
func (h *SocialHandler) CancelFriendRequest(c *gin.Context) {
	currentUserID, requestID, ok := socialOperationIdentity(c)
	if !ok {
		return
	}

	cancelled, err := h.service.CancelFriendRequest(c.Request.Context(), requestID, currentUserID)
	if err != nil {
		writeSocialError(c, "cancel friend request", err)
		return
	}
	h.pushSocialEvent(cancelled.ReceiverID, "friend_request_cancelled", socialChangedEventData{
		RequestID: cancelled.ID,
		UserID:    currentUserID,
	})
	c.JSON(http.StatusOK, response.Success(toFriendRequestMutationResponse(cancelled)))
}

// ListFriends 返回 JWT 当前用户的直接好友摘要。
func (h *SocialHandler) ListFriends(c *gin.Context) {
	currentUserID, ok := socialCurrentUserID(c)
	if !ok {
		return
	}

	friends, err := h.service.ListFriends(c.Request.Context(), currentUserID)
	if err != nil {
		writeSocialError(c, "list friends", err)
		return
	}
	// 先收集 ID，再批量查询积分和在线状态；不能在下面的组装循环里逐个查询。
	friendIDs := make([]int64, 0, len(friends))
	for _, friend := range friends {
		friendIDs = append(friendIDs, friend.ID)
	}
	// 空 map 让没有积分记录的用户自然得到 Go 零值 0，也避免 nil 特判。
	scoresByUserID := make(map[int64]score.PlayerScore)
	if len(friendIDs) > 0 && h.scoreService != nil {
		scoresByUserID, err = h.scoreService.GetByUserIDs(c.Request.Context(), friendIDs)
		if err != nil {
			log.Printf("get friend scores: %v", err)
			c.JSON(http.StatusInternalServerError, response.Error(
				response.CodeInternalError,
				"internal server error",
			))
			return
		}
	}
	// 在线状态来自 WebSocket Hub 的当前内存快照，不是 users 表中的永久字段。
	onlineByUserID := make(map[int64]bool)
	if len(friendIDs) > 0 && h.realtime != nil {
		onlineByUserID = h.realtime.OnlineUsers(friendIDs)
	}

	// 两张 map 都以 userID 为索引，组装 N 个好友时不再产生额外 SQL 或线性搜索。
	result := make([]friendResponse, 0, len(friends))
	for _, friend := range friends {
		friendScore := scoresByUserID[friend.ID]
		result = append(result, friendResponse{
			ID:       friend.ID,
			Username: friend.Username,
			Score:    friendScore.Score,
			Online:   onlineByUserID[friend.ID],
		})
	}
	c.JSON(http.StatusOK, response.Success(result))
}

// RemoveFriend 删除 JWT 当前用户与路径参数指定用户之间的好友关系。
func (h *SocialHandler) RemoveFriend(c *gin.Context) {
	currentUserID, ok := socialCurrentUserID(c)
	if !ok {
		return
	}
	friendUserID, err := strconv.ParseInt(c.Param("friendID"), 10, 64)
	if err != nil || friendUserID <= 0 {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "invalid friend user id"))
		return
	}

	if err := h.service.RemoveFriend(c.Request.Context(), currentUserID, friendUserID); err != nil {
		writeSocialError(c, "remove friend", err)
		return
	}
	h.pushSocialEvent(friendUserID, "friendship_removed", socialChangedEventData{UserID: currentUserID})
	// DELETE 成功后没有额外数据需要返回，仍保持项目统一响应结构。
	c.JSON(http.StatusOK, response.Success[any](nil))
}

// ListIncomingFriendRequests 返回 JWT 当前用户收到的申请。
func (h *SocialHandler) ListIncomingFriendRequests(c *gin.Context) {
	h.listFriendRequests(c, true)
}

// ListOutgoingFriendRequests 返回 JWT 当前用户发出的申请。
func (h *SocialHandler) ListOutgoingFriendRequests(c *gin.Context) {
	h.listFriendRequests(c, false)
}

// listFriendRequests 复用收件箱和发件箱的鉴权、分页、VO 转换及响应逻辑。
func (h *SocialHandler) listFriendRequests(c *gin.Context, incoming bool) {
	currentUserID, ok := socialCurrentUserID(c)
	if !ok {
		return
	}
	status, limit, offset, ok := socialListQuery(c)
	if !ok {
		return
	}

	var (
		details []social.FriendRequestDetail
		err     error
	)
	if incoming {
		details, err = h.service.ListIncomingFriendRequests(
			c.Request.Context(), currentUserID, status, limit, offset,
		)
	} else {
		details, err = h.service.ListOutgoingFriendRequests(
			c.Request.Context(), currentUserID, status, limit, offset,
		)
	}
	if err != nil {
		writeSocialError(c, "list friend requests", err)
		return
	}

	result := make([]friendRequestResponse, 0, len(details))
	for _, detail := range details {
		result = append(result, toFriendRequestResponse(detail.Request, detail.Requester, detail.Receiver))
	}
	c.JSON(http.StatusOK, response.Success(result))
}

// socialCurrentUserID 只读取 JWT 中间件写入的可信身份；读取失败后立即完成 401 响应。
func socialCurrentUserID(c *gin.Context) (int64, bool) {
	currentUserID, ok := middleware.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, response.Error(
			http.StatusUnauthorized,
			"invalid or missing access token",
		))
		return 0, false
	}
	return currentUserID, true
}

// socialOperationIdentity 同时解析当前用户和好友申请路径参数，供接受、拒绝、撤销复用。
func socialOperationIdentity(c *gin.Context) (currentUserID, requestID int64, ok bool) {
	currentUserID, ok = socialCurrentUserID(c)
	if !ok {
		return 0, 0, false
	}
	requestID, err := strconv.ParseInt(c.Param("requestID"), 10, 64)
	if err != nil || requestID <= 0 {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "invalid friend request id"))
		return 0, 0, false
	}
	return currentUserID, requestID, true
}

// socialListQuery 为申请列表提供统一默认值，并限制最大分页大小，避免一次拉取无限数据。
func socialListQuery(c *gin.Context) (social.FriendRequestStatus, int, int, bool) {
	status := social.FriendRequestStatus(c.DefaultQuery("status", string(social.FriendRequestPending)))
	limit, limitErr := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, offsetErr := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if !status.Valid() || limitErr != nil || offsetErr != nil || limit <= 0 || limit > 100 || offset < 0 {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "invalid list query"))
		return "", 0, 0, false
	}
	return status, limit, offset, true
}

func toFriendRequestResponse(
	request social.FriendRequest,
	requester social.UserSummary,
	receiver social.UserSummary,
) friendRequestResponse {
	return friendRequestResponse{
		ID:          request.ID,
		Requester:   socialUserResponse{ID: requester.ID, Username: requester.Username},
		Receiver:    socialUserResponse{ID: receiver.ID, Username: receiver.Username},
		Status:      request.Status,
		CreatedAt:   request.CreatedAt,
		UpdatedAt:   request.UpdatedAt,
		RespondedAt: request.RespondedAt,
	}
}

func toFriendRequestMutationResponse(request social.FriendRequest) friendRequestMutationResponse {
	return friendRequestMutationResponse{
		ID:          request.ID,
		RequesterID: request.RequesterID,
		ReceiverID:  request.ReceiverID,
		Status:      request.Status,
		CreatedAt:   request.CreatedAt,
		UpdatedAt:   request.UpdatedAt,
		RespondedAt: request.RespondedAt,
	}
}

// pushSocialEvent 在数据库操作成功后通知另一方；离线是正常情况，不影响已经完成的业务操作。
func (h *SocialHandler) pushSocialEvent(userID int64, eventType string, data socialChangedEventData) {
	if h.realtime == nil {
		return
	}
	err := h.realtime.SendToUser(userID, realtime.Event{Type: eventType, Data: data})
	if err != nil && !errors.Is(err, realtime.ErrUserOffline) {
		log.Printf("push %s to user %d: %v", eventType, userID, err)
	}
}

// writeSocialError 把领域错误稳定映射成 HTTP 语义；未知错误只记服务端日志，不泄露内部细节。
func writeSocialError(c *gin.Context, operation string, err error) {
	switch {
	case errors.Is(err, social.ErrInvalidUserPair),
		errors.Is(err, social.ErrInvalidFriendRequestStatus),
		errors.Is(err, social.ErrInvalidFriendRequestPage):
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
	case errors.Is(err, social.ErrSocialUserNotFound),
		errors.Is(err, social.ErrFriendRequestNotFound),
		errors.Is(err, social.ErrFriendshipNotFound):
		c.JSON(http.StatusNotFound, response.Error(http.StatusNotFound, err.Error()))
	case errors.Is(err, social.ErrFriendRequestNotReceiver),
		errors.Is(err, social.ErrFriendRequestNotRequester):
		c.JSON(http.StatusForbidden, response.Error(http.StatusForbidden, err.Error()))
	case errors.Is(err, social.ErrAlreadyFriends),
		errors.Is(err, social.ErrFriendRequestPending),
		errors.Is(err, social.ErrFriendRequestNotPending),
		errors.Is(err, social.ErrFriendRequestStateChanged),
		errors.Is(err, social.ErrFriendRequestAlreadyExists):
		c.JSON(http.StatusConflict, response.Error(http.StatusConflict, err.Error()))
	default:
		log.Printf("%s: %v", operation, err)
		c.JSON(http.StatusInternalServerError, response.Error(
			response.CodeInternalError,
			"internal server error",
		))
	}
}
