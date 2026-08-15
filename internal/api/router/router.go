package router

import (
	"github.com/ChenXuan-github/rock-paper-scissors/internal/api/handler"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/api/middleware"
	"github.com/gin-gonic/gin"
)

// New 创建并配置 HTTP 路由器。
// tokenVerifier 用于给需要登录的路由组挂载 JWT 鉴权中间件。
func New(
	authHandler *handler.AuthHandler,
	userHandler *handler.UserHandler,
	tokenVerifier middleware.TokenVerifier,
) *gin.Engine {
	// Default 创建 Gin 引擎，并自带 Logger 与 Recovery 中间件。
	r := gin.Default()
	// 项目自定义的耗时中间件作用于全部路由。
	r.Use(middleware.RequestTimer())

	// 统一声明 API 版本；组内路径会自动与 /api/v1 拼接。
	api := r.Group("/api/v1")

	// 注册、登录在取得 JWT 前必须可访问，因此不挂鉴权中间件。
	if authHandler != nil {
		auth := api.Group("/auth")
		// 最终地址：/api/v1/auth/register 与 /api/v1/auth/login。
		auth.POST("/register", authHandler.Register)
		auth.POST("/login", authHandler.Login)
	}

	// 用户资料接口必须同时具备 Handler 和 JWT 校验器才注册。
	if userHandler != nil && tokenVerifier != nil {
		users := api.Group("/users")
		// 组内请求先校验 JWT，再进入具体 Handler。
		users.Use(middleware.Authenticate(tokenVerifier))
		// 最终地址：GET /api/v1/users/me。
		users.GET("/me", userHandler.Me)
	}

	// rounds 归组管理所有回合相关接口。
	rounds := api.Group("/rounds")
	// 正常启动时 main 会传入 TokenService，因此整个 rounds 路由组都需要 JWT。
	// 测试可以传 nil，只测试路由自身而不引入真实鉴权依赖。
	if tokenVerifier != nil {
		// 请求进入具体 Handler 前，先提取并校验 Authorization Bearer Token。
		rounds.Use(middleware.Authenticate(tokenVerifier))
	}
	// 最终地址：POST /api/v1/rounds/evaluate。
	rounds.POST("/evaluate", handler.EvaluateRound)

	// main 会对配置完成的 Engine 调用 Run，真正启动 HTTP 服务。
	return r
}
