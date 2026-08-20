package database

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/config"
	"github.com/redis/go-redis/v9"
)

// OpenRedis 创建全局复用的 Redis 客户端，并在启动阶段执行一次 PING。
// redis.Client 内部自带并发安全的连接池，因此不需要为每次请求重新创建客户端。
func OpenRedis(cfg config.RedisConfig) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)),
		Password:     cfg.Password,
		DB:           cfg.Database,
		PoolSize:     10,
		MinIdleConns: 2,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})

	// NewClient 与 sql.Open 类似，只创建客户端和连接池配置；PING 才验证 Redis 真正可用。
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	return client, nil
}
