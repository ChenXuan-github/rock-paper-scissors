package handler

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/api/middleware"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/api/response"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/user"
	"github.com/gin-gonic/gin"
)

// userQueryService 是用户信息 Handler 查询用户时需要的最小能力。
type userQueryService interface {
	GetByID(ctx context.Context, id int64) (user.User, error)
}

// UserHandler 处理当前用户资料等用户接口。
type UserHandler struct {
	userService userQueryService
}

// NewUserHandler 创建用户接口处理器。
func NewUserHandler(userService userQueryService) *UserHandler {
	return &UserHandler{userService: userService}
}

// currentUserResponse 只定义允许返回给客户端的非敏感用户字段。
type currentUserResponse struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"createdAt"`
}

// Me 返回当前登录用户在数据库中的最新信息。
func (h *UserHandler) Me(c *gin.Context) {
	// 用户 ID 已经由 JWT 鉴权中间件校验并写入本次请求的 Context。
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, response.Error(
			http.StatusUnauthorized,
			"invalid or missing access token",
		))
		return
	}

	currentUser, err := h.userService.GetByID(c.Request.Context(), userID)
	if err != nil {
		// JWT 对应的用户已经不存在时，不再把该身份视为有效登录状态。
		if errors.Is(err, user.ErrUserNotFound) {
			c.JSON(http.StatusUnauthorized, response.Error(
				http.StatusUnauthorized,
				"current user no longer exists",
			))
			return
		}

		log.Printf("get current user: %v", err)
		c.JSON(http.StatusInternalServerError, response.Error(
			response.CodeInternalError,
			"internal server error",
		))
		return
	}

	// 显式组装响应，避免把 User 中的 PasswordHash 意外序列化出去。
	c.JSON(http.StatusOK, response.Success(currentUserResponse{
		ID:        currentUser.ID,
		Username:  currentUser.Username,
		CreatedAt: currentUser.CreatedAt,
	}))
}
