package main

import (
	"fmt"
	"log"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/api/handler"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/api/router"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/auth"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/config"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/database"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/game"
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

	// 3. 从底层到上层手动组装依赖：Repository → Service → Handler。
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
	roomHandler := handler.NewRoomHandler(roomService)

	// 4. 注入 Router：受保护路由通过同一个 TokenService 校验 JWT。
	r := router.New(authHandler, userHandler, roomHandler, tokenService)
	// Sprintf 把整数端口拼成 Gin Run 所需的 ":8080" 监听地址。
	address := fmt.Sprintf(":%d", cfg.Server.Port)

	// 5. 阻塞启动 HTTP 服务；监听失败时记录错误并终止程序。
	if err := r.Run(address); err != nil {
		log.Fatal(err)
	}
}
