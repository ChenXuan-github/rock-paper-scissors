package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/auth"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/config"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/user"
	"github.com/gin-gonic/gin"
)

func TestAuthenticateStoresCurrentUserInContext(t *testing.T) {
	// 使用真实 TokenService 签发测试 JWT，覆盖“生成→请求携带→中间件解析”的正向链路。
	tokenService := auth.NewTokenService(config.JWTConfig{
		Issuer:           "rock-paper-scissors",
		Secret:           "test-signing-secret",
		ExpiresInMinutes: 60,
	})
	signedToken, err := tokenService.Generate(user.User{ID: 1, Username: "chenxuan"})
	if err != nil {
		t.Fatal(err)
	}

	// 受保护 Handler 从 Context 读取身份并原样响应，证明中间件确实完成了 c.Set。
	r := gin.New()
	r.Use(Authenticate(tokenService))
	r.GET("/protected", func(c *gin.Context) {
		userID, _ := CurrentUserID(c)
		username, _ := CurrentUsername(c)
		c.JSON(http.StatusOK, gin.H{"userId": userID, "username": username})
	})

	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("Authorization", "Bearer "+signedToken)
	result := httptest.NewRecorder()
	r.ServeHTTP(result, request)

	if result.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, want %d; body = %s", result.Code, http.StatusOK, result.Body.String())
	}
	if result.Body.String() != `{"userId":1,"username":"chenxuan"}` {
		t.Errorf("body = %s", result.Body.String())
	}
}

func TestAuthenticateRejectsMissingToken(t *testing.T) {
	// 没有 Authorization Header 时，中间件应返回 401 并阻止业务 Handler 执行。
	tokenService := auth.NewTokenService(config.JWTConfig{
		Issuer:           "rock-paper-scissors",
		Secret:           "test-signing-secret",
		ExpiresInMinutes: 60,
	})

	r := gin.New()
	r.Use(Authenticate(tokenService))
	r.GET("/protected", func(c *gin.Context) {
		t.Fatal("protected handler must not run without a token")
	})

	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	result := httptest.NewRecorder()
	r.ServeHTTP(result, request)

	if result.Code != http.StatusUnauthorized {
		t.Fatalf("HTTP status = %d, want %d", result.Code, http.StatusUnauthorized)
	}
}
