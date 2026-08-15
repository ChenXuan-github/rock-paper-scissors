package database

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/config"
	mysqlDriver "github.com/go-sql-driver/mysql"
)

// Open 创建数据库连接池，并验证数据库当前可以访问。
func Open(cfg config.DatabaseConfig) (*sql.DB, error) {
	// 使用驱动提供的 Config 组装 DSN，避免手写连接字符串时漏转义参数。
	mysqlConfig := mysqlDriver.Config{
		User:      cfg.Username,
		Passwd:    cfg.Password,
		Net:       "tcp",
		Addr:      net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)),
		DBName:    cfg.Name,
		ParseTime: true,       // 把 DATETIME/TIMESTAMP 解析为 time.Time，而不是 []byte。
		Loc:       time.Local, // 使用本机时区解释数据库时间。
		Params: map[string]string{
			"charset": "utf8mb4",
		},
	}
	// FormatDSN 生成 database/sql 和 MySQL 驱动认识的连接字符串。
	dsn := mysqlConfig.FormatDSN()

	// sql.Open 创建并返回连接池句柄，通常不会在此刻真正建立网络连接。
	db, err := sql.Open(cfg.Driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// 限制连接池规模，避免并发请求无限创建数据库连接。
	db.SetMaxOpenConns(10)
	// 保留少量空闲连接，后续请求可以直接复用。
	db.SetMaxIdleConns(5)
	// 定期淘汰长期存活的连接，降低服务端断开旧连接带来的问题。
	db.SetConnMaxLifetime(30 * time.Minute)

	// 启动阶段最多等待 5 秒验证数据库，防止服务长期卡在不可用连接上。
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	// 无论 Ping 成功还是失败，函数返回前都释放 Context 的定时器资源。
	defer cancel()

	// PingContext 才会实际验证地址、账号密码和目标数据库是否可访问。
	if err := db.PingContext(ctx); err != nil {
		// 验证失败时关闭刚创建的连接池，避免资源泄漏。
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	// 调用方获得的是并发安全的连接池，并负责在程序退出时 Close。
	return db, nil
}
