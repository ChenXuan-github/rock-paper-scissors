package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/user"
	"github.com/gin-gonic/gin"
)

// stubUserService 通过函数字段按用例定制行为，并隐式实现 Handler 需要的 userService。
type stubUserService struct {
	register func(ctx context.Context, username, password string) (user.User, error)
	login    func(ctx context.Context, username, password string) (user.User, error)
}

func (s stubUserService) Register(ctx context.Context, username, password string) (user.User, error) {
	return s.register(ctx, username, password)
}

func (s stubUserService) Login(ctx context.Context, username, password string) (user.User, error) {
	return s.login(ctx, username, password)
}

// stubTokenIssuer 让 Handler 测试不依赖真实 JWT 算法，只验证是否正确调用签发能力。
type stubTokenIssuer struct {
	generate func(loginUser user.User) (string, error)
}

func (s stubTokenIssuer) Generate(loginUser user.User) (string, error) {
	return s.generate(loginUser)
}

func TestRegisterReturnsCreatedUserWithoutPasswordHash(t *testing.T) {
	// Stub 故意返回带敏感哈希的领域对象，以验证 Handler 的响应 DTO 会过滤它。
	authHandler := NewAuthHandler(stubUserService{
		register: func(_ context.Context, username, _ string) (user.User, error) {
			return user.User{
				ID:           1,
				Username:     username,
				PasswordHash: "must-not-be-returned",
			}, nil
		},
	}, nil)

	response := performAuthRequest(
		authHandler.Register,
		`{"username":"chenxuan","password":"password123"}`,
	)

	if response.Code != http.StatusCreated {
		t.Fatalf("HTTP status = %d, want %d", response.Code, http.StatusCreated)
	}

	if strings.Contains(response.Body.String(), "must-not-be-returned") ||
		strings.Contains(response.Body.String(), "passwordHash") {
		t.Fatalf("response exposed password hash: %s", response.Body.String())
	}

	var body struct {
		Data registerResponse `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.ID != 1 || body.Data.Username != "chenxuan" {
		t.Errorf("response data = %+v", body.Data)
	}
}

func TestRegisterRejectsExistingUsername(t *testing.T) {
	authHandler := NewAuthHandler(stubUserService{
		register: func(_ context.Context, _, _ string) (user.User, error) {
			return user.User{}, user.ErrUsernameExists
		},
	}, nil)

	response := performAuthRequest(
		authHandler.Register,
		`{"username":"chenxuan","password":"password123"}`,
	)

	if response.Code != http.StatusConflict {
		t.Fatalf("HTTP status = %d, want %d", response.Code, http.StatusConflict)
	}
}

func TestRegisterHidesInternalError(t *testing.T) {
	// 底层错误包含模拟敏感信息，HTTP 响应只能返回统一 500 文案。
	authHandler := NewAuthHandler(stubUserService{
		register: func(_ context.Context, _, _ string) (user.User, error) {
			return user.User{}, errors.New("database password leaked here")
		},
	}, nil)

	response := performAuthRequest(
		authHandler.Register,
		`{"username":"chenxuan","password":"password123"}`,
	)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("HTTP status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	if strings.Contains(response.Body.String(), "database password") {
		t.Fatalf("response exposed internal error: %s", response.Body.String())
	}
}

func TestLoginReturnsBearerTokenAndUser(t *testing.T) {
	// 分别替换登录业务和 Token 签发器，测试 Handler 对两者的编排。
	authHandler := NewAuthHandler(
		stubUserService{
			login: func(_ context.Context, username, _ string) (user.User, error) {
				return user.User{
					ID:           1,
					Username:     username,
					PasswordHash: "must-not-be-returned",
				}, nil
			},
		},
		stubTokenIssuer{
			generate: func(loginUser user.User) (string, error) {
				if loginUser.ID != 1 {
					t.Fatalf("login user = %+v", loginUser)
				}
				return "header.payload.signature", nil
			},
		},
	)

	result := performAuthRequest(
		authHandler.Login,
		`{"username":"chenxuan","password":"123123"}`,
	)

	if result.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, want %d; body = %s", result.Code, http.StatusOK, result.Body.String())
	}
	if strings.Contains(result.Body.String(), "must-not-be-returned") {
		t.Fatalf("response exposed password hash: %s", result.Body.String())
	}

	var body struct {
		Data loginResponse `json:"data"`
	}
	if err := json.Unmarshal(result.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Token != "header.payload.signature" || body.Data.TokenType != "Bearer" {
		t.Errorf("login response = %+v", body.Data)
	}
	if body.Data.User.ID != 1 || body.Data.User.Username != "chenxuan" {
		t.Errorf("login user = %+v", body.Data.User)
	}
}

func TestLoginRejectsInvalidCredentials(t *testing.T) {
	// 凭证无效时不但要返回 401，还必须保证签发器完全不会被调用。
	authHandler := NewAuthHandler(
		stubUserService{
			login: func(_ context.Context, _, _ string) (user.User, error) {
				return user.User{}, user.ErrInvalidCredentials
			},
		},
		stubTokenIssuer{
			generate: func(user.User) (string, error) {
				t.Fatal("token must not be generated for invalid credentials")
				return "", nil
			},
		},
	)

	result := performAuthRequest(
		authHandler.Login,
		`{"username":"chenxuan","password":"wrong-password"}`,
	)

	if result.Code != http.StatusUnauthorized {
		t.Fatalf("HTTP status = %d, want %d", result.Code, http.StatusUnauthorized)
	}
}

func performAuthRequest(handler gin.HandlerFunc, body string) *httptest.ResponseRecorder {
	// 每个用例创建隔离的最小 Gin 引擎，只注册当前待测 Handler。
	r := gin.New()
	r.POST("/register", handler)

	request := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")

	// Recorder 捕获 Handler 写出的 HTTP 状态和 JSON 响应。
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	return response
}
