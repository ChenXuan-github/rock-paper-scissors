package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/user"
	"github.com/gin-gonic/gin"
)

// stubUserQueryService 隔离 Handler 与真实 Service/MySQL，只提供按 ID 查询能力。
type stubUserQueryService struct {
	getByID func(ctx context.Context, id int64) (user.User, error)
}

func (s stubUserQueryService) GetByID(ctx context.Context, id int64) (user.User, error) {
	return s.getByID(ctx, id)
}

func TestMeReturnsPublicUserFieldsWithoutPasswordHash(t *testing.T) {
	// 固定时间让 createdAt 响应断言稳定，不依赖测试运行时刻。
	gin.SetMode(gin.TestMode)
	createdAt := time.Date(2026, time.August, 15, 10, 30, 0, 0, time.UTC)
	userHandler := NewUserHandler(stubUserQueryService{
		getByID: func(_ context.Context, id int64) (user.User, error) {
			return user.User{
				ID:           id,
				Username:     "chenxuan",
				PasswordHash: "must-not-be-returned",
				CreatedAt:    createdAt,
			}, nil
		},
	})

	// 用前置测试 Handler 模拟真实 JWT 中间件写入可信 currentUserID。
	r := gin.New()
	r.GET("/me", func(c *gin.Context) {
		// 模拟 JWT 鉴权中间件已经把当前用户 ID 写入请求上下文。
		c.Set("currentUserID", int64(1))
		c.Next()
	}, userHandler.Me)

	request := httptest.NewRequest(http.MethodGet, "/me", nil)
	result := httptest.NewRecorder()
	r.ServeHTTP(result, request)

	if result.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, want %d; body = %s", result.Code, http.StatusOK, result.Body.String())
	}
	// 即使领域对象含有 PasswordHash，公开响应也必须彻底过滤该字段和值。
	if strings.Contains(result.Body.String(), "must-not-be-returned") || strings.Contains(result.Body.String(), "password") {
		t.Fatalf("response leaked password hash: %s", result.Body.String())
	}
	if !strings.Contains(result.Body.String(), `"createdAt":"2026-08-15T10:30:00Z"`) {
		t.Errorf("response does not contain createdAt: %s", result.Body.String())
	}
}
