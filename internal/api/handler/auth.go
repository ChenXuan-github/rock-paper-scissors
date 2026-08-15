package handler

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/api/response"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/user"
	"github.com/gin-gonic/gin"
)

// userService 是认证 Handler 真正依赖的最小业务能力。
// *user.Service 方法集合匹配后会隐式实现该接口。
type userService interface {
	Register(ctx context.Context, username, password string) (user.User, error)
	Login(ctx context.Context, username, password string) (user.User, error)
}

// tokenIssuer 是 Handler 签发登录令牌所依赖的最小能力。
// AuthHandler 不需要知道底层使用哪个 JWT 库或签名算法。
type tokenIssuer interface {
	// Generate 根据已经完成密码验证的用户生成 JWT。
	Generate(loginUser user.User) (string, error)
}

// AuthHandler 处理注册、登录等身份认证接口。
type AuthHandler struct {
	// userService 负责注册、查重、密码哈希和登录验证等业务规则。
	userService userService
	// tokenIssuer 在登录成功后签发 JWT。
	tokenIssuer tokenIssuer
}

// NewAuthHandler 创建身份认证接口处理器。
func NewAuthHandler(userService userService, tokenIssuer tokenIssuer) *AuthHandler {
	return &AuthHandler{
		userService: userService,
		// 由 main 注入 TokenService，实现 Handler 与 JWT 具体实现解耦。
		tokenIssuer: tokenIssuer,
	}
}

type registerRequest struct {
	// binding 标签属于 HTTP 入口的第一层格式校验，Service 仍会执行最终业务校验。
	Username string `json:"username" binding:"required,max=64"`
	Password string `json:"password" binding:"required,min=6,max=72"`
}

// registerResponse 是注册成功后允许公开的用户信息，不包含 PasswordHash。
type registerResponse struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

// loginRequest 是登录接口接收的账号密码 DTO。
type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// loginResponse 同时返回访问令牌、令牌使用方式和公开用户资料。
type loginResponse struct {
	// Token 是签名完成的 Header.Payload.Signature 字符串。
	Token string `json:"token"`
	// TokenType 告诉客户端后续应使用 Authorization: Bearer <token>。
	TokenType string `json:"tokenType"`
	// User 返回可公开的用户信息，绝不包含密码哈希。
	User registerResponse `json:"user"`
}

// Register 接收注册信息并创建用户。
func (h *AuthHandler) Register(c *gin.Context) {
	// 先把 JSON 请求体绑定到 DTO，并执行 required/max/min 标签校验。
	var request registerRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(
			response.CodeInvalidRequest,
			"invalid request body",
		))
		return
	}

	// 把 HTTP Request 的标准 Context 向业务层传递，以保留取消信号。
	createdUser, err := h.userService.Register(
		c.Request.Context(),
		request.Username,
		request.Password,
	)
	if err != nil {
		// Handler 把领域错误翻译成对应 HTTP 状态码和统一响应结构。
		switch {
		case errors.Is(err, user.ErrInvalidUsername),
			errors.Is(err, user.ErrInvalidPassword):
			c.JSON(http.StatusBadRequest, response.Error(
				response.CodeInvalidRequest,
				err.Error(),
			))
		case errors.Is(err, user.ErrUsernameExists):
			c.JSON(http.StatusConflict, response.Error(
				http.StatusConflict,
				err.Error(),
			))
		default:
			// 未预期错误只写服务器日志，不把数据库或内部堆栈细节暴露给客户端。
			log.Printf("register user: %v", err)
			c.JSON(http.StatusInternalServerError, response.Error(
				response.CodeInternalError,
				"internal server error",
			))
		}
		return
	}

	// 显式组装响应 DTO，从结构上阻止 PasswordHash 被 JSON 序列化。
	c.JSON(http.StatusCreated, response.Success(registerResponse{
		ID:       createdUser.ID,
		Username: createdUser.Username,
	}))
}

// Login 验证账号密码，成功后签发 JWT。
func (h *AuthHandler) Login(c *gin.Context) {
	// 登录与注册使用独立 DTO，便于以后分别调整接口校验规则。
	var request loginRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(
			response.CodeInvalidRequest,
			"invalid request body",
		))
		return
	}

	// Service 查询用户并使用 BCrypt 验证密码；Handler 不直接接触哈希算法。
	loginUser, err := h.userService.Login(
		c.Request.Context(),
		request.Username,
		request.Password,
	)
	if err != nil {
		// 账号不存在和密码错误都映射为相同的 401 响应。
		if errors.Is(err, user.ErrInvalidCredentials) {
			c.JSON(http.StatusUnauthorized, response.Error(
				http.StatusUnauthorized,
				"invalid username or password",
			))
			return
		}

		log.Printf("login user: %v", err)
		c.JSON(http.StatusInternalServerError, response.Error(
			response.CodeInternalError,
			"internal server error",
		))
		return
	}

	// 账号密码已经验证通过，现在为该用户签发 JWT。
	signedToken, err := h.tokenIssuer.Generate(loginUser)
	if err != nil {
		// 签发失败属于服务器内部错误，具体原因只写日志、不返回客户端。
		log.Printf("generate login token: %v", err)
		c.JSON(http.StatusInternalServerError, response.Error(
			response.CodeInternalError,
			"internal server error",
		))
		return
	}

	// 把 JWT 及其使用方式交给客户端，供后续受保护请求携带。
	c.JSON(http.StatusOK, response.Success(loginResponse{
		// 客户端后续把该字符串放进 Authorization 请求头。
		Token: signedToken,
		// 最终请求头格式为：Authorization: Bearer <signedToken>。
		TokenType: "Bearer",
		User: registerResponse{
			ID:       loginUser.ID,
			Username: loginUser.Username,
		},
	}))
}
