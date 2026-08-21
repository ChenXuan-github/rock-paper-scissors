package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/api/handler"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/api/router"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/auth"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/config"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/database"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/game"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/leaderboard"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/matchmaking"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/realtime"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/record"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/score"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/settlement"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/social"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/user"
)

func main() {
	// 1. 读取端口、MySQL 和 JWT 配置；启动阶段失败直接终止程序。
	cfg, err := config.Load("configs/application.yml")
	if err != nil {
		log.Fatal(err)
	}

	// 2. 创建并验证数据库连接池。
	db, err := database.Open(cfg.Database)
	if err != nil {
		log.Fatal(err)
	}
	// main 退出时关闭连接池；正常运行期间连接池会被各 Repository 复用。
	defer db.Close()

	// 3. 创建一个全局复用的 Redis 客户端；OpenRedis 内部会先执行 PING 验证连接。
	redisClient, err := database.OpenRedis(cfg.Redis)
	if err != nil {
		log.Fatal(err)
	}
	defer redisClient.Close()
	// 当前房间和 WebSocket 都是进程内状态；重启后清除 Redis 中无法恢复的旧匹配队列。
	matchmakingQueue := matchmaking.NewRedisQueue(redisClient)
	queueCleanupContext, cancelQueueCleanup := context.WithTimeout(context.Background(), 5*time.Second)
	err = matchmakingQueue.Clear(queueCleanupContext)
	cancelQueueCleanup()
	if err != nil {
		log.Fatal(fmt.Errorf("clear stale matchmaking queue: %w", err))
	}

	// 4. 从底层到上层手动组装依赖：Repository → Service → Handler。
	userRepository := user.NewMySQLRepository(db)
	userService := user.NewService(userRepository)
	// 根据 JWT 配置创建同一个 TokenService，统一负责签发和校验令牌。
	tokenService := auth.NewTokenService(cfg.JWT)
	// 注入 Handler：用户登录成功后由 TokenService 签发 JWT。
	authHandler := handler.NewAuthHandler(userService, tokenService)
	// 当前用户接口通过同一个 UserService 查询数据库中的最新用户资料。
	userHandler := handler.NewUserHandler(userService)
	// 房间只保存在当前进程内存中；整个服务共享同一个 RoomManager。
	roomManager := game.NewRoomManager()
	roomService := game.NewRoomService(roomManager)
	// Hub 在独立 goroutine 中串行管理全部在线 WebSocket 连接。
	realtimeHub := realtime.NewHub()
	go realtimeHub.Run(context.Background())
	// 查询接口复用普通连接池；结算事务内部仍会创建持有同一个 *sql.Tx 的 Repository。
	scoreService := score.NewService(score.NewMySQLRepository(db))
	recordService := record.NewService(record.NewMySQLRepository(db))

	// Redis ZSet 是排行榜排序索引；启动时从 MySQL 真相源全量重建一次。
	leaderboardService := leaderboard.NewService(redisClient)
	bootstrapContext, cancelBootstrap := context.WithTimeout(context.Background(), 5*time.Second)
	allScores, err := scoreService.ListAll(bootstrapContext)
	if err == nil {
		err = leaderboardService.ReplaceAll(bootstrapContext, allScores)
	}
	cancelBootstrap()
	if err != nil {
		log.Fatal(fmt.Errorf("bootstrap leaderboard: %w", err))
	}

	// 每局 MySQL 事务提交成功后，把双方最新总分同步到同一个排行榜 Service。
	settlementService := settlement.NewService(db, leaderboardService)
	competitionHandler := handler.NewCompetitionHandler(
		scoreService,
		recordService,
		leaderboardService,
		userService,
	)
	// 房间 Handler 先调用结算服务持久化，再通过 Hub 给在线玩家推送正式结果。
	roomHandler := handler.NewRoomHandler(roomService, realtimeHub, settlementService)
	// WebSocket 握手时复用同一个 TokenService 校验登录身份。
	webSocketHandler := handler.NewWebSocketHandler(tokenService, realtimeHub)
	// 自动匹配复用同一个 RoomService 创建房间，并通过现有 Hub 通知双方。
	matchmakingService := matchmaking.NewService(matchmakingQueue, roomService, userService, realtimeHub)
	matchmakingHandler := handler.NewMatchmakingHandler(matchmakingService)
	// 好友申请和无向好友关系共用 MySQL，并复用 UserService 批量查询安全用户摘要。
	socialService := social.NewService(db, userService)
	socialHandler := handler.NewSocialHandler(socialService, scoreService, realtimeHub)
	// 对战邀请只保存 60 秒内的临时状态，接受后复用现有 RoomService 自动建立 1v1 房间。
	gameInvitationService := social.NewGameInvitationService(
		social.NewGameInvitationManager(),
		social.NewMySQLFriendshipRepository(db),
		userService,
		roomService,
		realtimeHub,
	)
	gameInvitationHandler := handler.NewGameInvitationHandler(gameInvitationService)

	// 5. 注入 Router：受保护路由通过同一个 TokenService 校验 JWT。
	r := router.New(
		authHandler,
		userHandler,
		roomHandler,
		competitionHandler,
		matchmakingHandler,
		socialHandler,
		gameInvitationHandler,
		webSocketHandler,
		tokenService,
	)
	// Sprintf 把整数端口拼成 Gin Run 所需的 ":8080" 监听地址。
	address := fmt.Sprintf(":%d", cfg.Server.Port)

	// 6. 阻塞启动 HTTP 服务；监听失败时记录错误并终止程序。
	if err := r.Run(address); err != nil {
		log.Fatal(err)
	}
}
