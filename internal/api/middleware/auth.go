package middleware

import (
	"net/http"
	"strings"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/api/response"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/auth"
	"github.com/gin-gonic/gin"
)

const (
	// userIDContextKey 是当前请求中保存用户 ID 的键。
	userIDContextKey = "currentUserID"
	// usernameContextKey 是当前请求中保存用户名的键。
	usernameContextKey = "currentUsername"
)

// TokenVerifier 表示鉴权中间件需要的 JWT 校验能力。
type TokenVerifier interface {
	// Parse 校验 JWT，成功时返回已经可信的 Claims。
	Parse(signedToken string) (auth.Claims, error)
}

// Authenticate 校验 Authorization Bearer Token，并把用户身份写入当前请求上下文。
func Authenticate(tokenVerifier TokenVerifier) gin.HandlerFunc {
	// Gin 中间件本质上也是一个接收 *gin.Context 的处理函数。
	return func(c *gin.Context) {
		// 从 Authorization 请求头中提取 Bearer 后面的 JWT 字符串。
		signedToken, ok := bearerToken(c.GetHeader("Authorization"))
		if !ok {
			// 请求头缺失或格式错误时返回 401，并阻止业务 Handler 执行。
			abortUnauthorized(c)
			return
		}

		// 校验 JWT 的算法、签名、签发者、签发时间和过期时间。
		claims, err := tokenVerifier.Parse(signedToken)
		if err != nil {
			// 伪造、过期或声明不合法的 JWT 都统一返回 401。
			abortUnauthorized(c)
			return
		}

		// JWT 校验通过后，其中的 UserID 才可以被服务器信任。
		// 身份只写入本次请求的 gin.Context，不使用会串请求的全局变量。
		c.Set(userIDContextKey, claims.UserID)
		// 同时保存用户名快照，后续 Handler 可以按需读取。
		c.Set(usernameContextKey, claims.Username)
		// 放行请求，继续执行后面的中间件和真正的业务 Handler。
		c.Next()
	}
}

// CurrentUserID 读取鉴权中间件写入的当前用户 ID。
func CurrentUserID(c *gin.Context) (int64, bool) {
	// 根据约定的键读取中间件保存的原始值。
	value, exists := c.Get(userIDContextKey)
	if !exists {
		// 不存在通常说明请求没有经过 JWT 鉴权中间件。
		return 0, false
	}

	// gin.Context 中保存的是 any，需要类型断言还原成 int64。
	userID, ok := value.(int64)
	// ok 表示类型断言是否成功。
	return userID, ok
}

// CurrentUsername 读取鉴权中间件写入的当前用户名。
func CurrentUsername(c *gin.Context) (string, bool) {
	// 读取中间件写入当前请求上下文的用户名。
	value, exists := c.Get(usernameContextKey)
	if !exists {
		return "", false
	}

	// 把 any 安全地断言成 string。
	username, ok := value.(string)
	return username, ok
}

// bearerToken 只解析 Authorization Header 的格式，不校验 JWT 内容。
func bearerToken(authorization string) (string, bool) {
	// Fields 按空白字符切分，还会自动忽略首尾和重复空格。
	parts := strings.Fields(authorization)
	// 标准格式必须正好两段，第一段必须是 Bearer（忽略大小写）。
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}

	// 第二段才是 Header.Payload.Signature 形式的 JWT。
	return parts[1], true
}

// abortUnauthorized 终止请求链，并返回统一的未认证响应。
func abortUnauthorized(c *gin.Context) {
	// AbortWithStatusJSON 同时中断后续 Handler、设置 401 并写入 JSON。
	c.AbortWithStatusJSON(http.StatusUnauthorized, response.Error(
		http.StatusUnauthorized,
		// 不向客户端透露 JWT 具体在哪一步校验失败。
		"invalid or missing access token",
	))
}
