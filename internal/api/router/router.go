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
	roomHandler *handler.RoomHandler,
	competitionHandler *handler.CompetitionHandler,
	matchmakingHandler *handler.MatchmakingHandler,
	socialHandler *handler.SocialHandler,
	gameInvitationHandler *handler.GameInvitationHandler,
	webSocketHandler *handler.WebSocketHandler,
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

	// 好友对战邀请只允许已登录用户创建和处理。
	if gameInvitationHandler != nil && tokenVerifier != nil {
		gameInvitations := api.Group("/game-invitations")
		gameInvitations.Use(middleware.Authenticate(tokenVerifier))
		gameInvitations.POST("", gameInvitationHandler.Invite)
		gameInvitations.POST("/:invitationID/accept", gameInvitationHandler.Accept)
		gameInvitations.POST("/:invitationID/reject", gameInvitationHandler.Reject)
		gameInvitations.DELETE("/:invitationID", gameInvitationHandler.Cancel)
	}

	// 好友关系和好友申请的操作者都取自 JWT，不接受客户端自行指定当前用户 ID。
	if socialHandler != nil && tokenVerifier != nil {
		friends := api.Group("/friends")
		friends.Use(middleware.Authenticate(tokenVerifier))
		friends.GET("", socialHandler.ListFriends)
		friends.DELETE("/:friendID", socialHandler.RemoveFriend)

		friendRequests := api.Group("/friend-requests")
		friendRequests.Use(middleware.Authenticate(tokenVerifier))
		friendRequests.POST("", socialHandler.SendFriendRequest)
		friendRequests.GET("/incoming", socialHandler.ListIncomingFriendRequests)
		friendRequests.GET("/outgoing", socialHandler.ListOutgoingFriendRequests)
		friendRequests.POST("/:requestID/accept", socialHandler.AcceptFriendRequest)
		friendRequests.POST("/:requestID/reject", socialHandler.RejectFriendRequest)
		friendRequests.DELETE("/:requestID", socialHandler.CancelFriendRequest)
	}

	// 积分与战绩同样依赖 JWT，客户端不能通过参数查询并冒充其他用户。
	if competitionHandler != nil && tokenVerifier != nil {
		competition := api.Group("")
		competition.Use(middleware.Authenticate(tokenVerifier))
		competition.GET("/scores/me", competitionHandler.MyScore)
		competition.GET("/records/me", competitionHandler.MyRecords)
		competition.GET("/rankings", competitionHandler.Ranking)
	}

	// 自动匹配接口只接受当前 JWT 用户，不允许客户端指定任意 userID。
	if matchmakingHandler != nil && tokenVerifier != nil {
		matchmakingRoutes := api.Group("/matchmaking")
		matchmakingRoutes.Use(middleware.Authenticate(tokenVerifier))
		// 同一个资源使用不同 HTTP 方法表达加入、查询和取消。
		matchmakingRoutes.POST("/me", matchmakingHandler.Join)
		matchmakingRoutes.GET("/me", matchmakingHandler.Current)
		matchmakingRoutes.DELETE("/me", matchmakingHandler.Cancel)
	}

	// 用户资料接口必须同时具备 Handler 和 JWT 校验器才注册。
	if userHandler != nil && tokenVerifier != nil {
		users := api.Group("/users")
		// 组内请求先校验 JWT，再进入具体 Handler。
		users.Use(middleware.Authenticate(tokenVerifier))
		// 最终地址：GET /api/v1/users/me。
		users.GET("/me", userHandler.Me)
	}

	// 房间接口必须登录后才能调用，创建者身份来自校验通过的 JWT。
	if roomHandler != nil && tokenVerifier != nil {
		rooms := api.Group("/rooms")
		rooms.Use(middleware.Authenticate(tokenVerifier))
		// 最终地址：GET /api/v1/rooms，读取当前全部房间。
		rooms.GET("", roomHandler.List)
		// 最终地址：GET /api/v1/rooms/me，恢复当前玩家自己的房间与回合状态。
		rooms.GET("/me", roomHandler.Current)
		// 最终地址：POST /api/v1/rooms。
		rooms.POST("", roomHandler.Create)
		// 最终地址：POST /api/v1/rooms/:roomID/join。
		rooms.POST("/:roomID/join", roomHandler.Join)
		// 最终地址：POST /api/v1/rooms/me/start，只有当前房主可以调用。
		rooms.POST("/me/start", roomHandler.Start)
		// 最终地址：POST /api/v1/rooms/me/move，当前玩家只提交自己的拳。
		rooms.POST("/me/move", roomHandler.SubmitMove)
		// 最终地址：DELETE /api/v1/rooms/me，表示当前用户退出所在房间。
		rooms.DELETE("/me", roomHandler.LeaveCurrent)
	}

	// WebSocket 在握手 Handler 内校验查询参数中的 JWT，因此不复用只读取 Authorization Header 的中间件。
	if webSocketHandler != nil {
		// 最终地址：GET /api/v1/ws?token=<JWT>。
		api.GET("/ws", webSocketHandler.Connect)
	}

	// main 会对配置完成的 Engine 调用 Run，真正启动 HTTP 服务。
	return r
}
