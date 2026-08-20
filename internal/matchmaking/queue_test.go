package matchmaking

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// TestRedisQueueLifecycle 使用独立临时 Key 验证真实 Redis ZSet 与 Lua 取对逻辑。
// 没有启动 Redis 的通用测试环境会跳过；本项目本机开发环境会实际执行。
func TestRedisQueueLifecycle(t *testing.T) {
	ctx := context.Background()
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"})
	defer client.Close()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("local Redis is unavailable: %v", err)
	}

	key := fmt.Sprintf("rps:test:matchmaking:%d", time.Now().UnixNano())
	queue := newRedisQueue(client, key)
	t.Cleanup(func() { _ = queue.Clear(ctx) })

	baseTime := time.UnixMilli(1_700_000_000_000)
	for index, userID := range []int64{11, 22, 33} {
		err := queue.Enqueue(ctx, QueueEntry{
			UserID:     userID,
			EnqueuedAt: baseTime.Add(time.Duration(index) * time.Millisecond),
		})
		if err != nil {
			t.Fatalf("Enqueue(%d) error = %v", userID, err)
		}
	}
	if err := queue.Enqueue(ctx, QueueEntry{UserID: 11, EnqueuedAt: time.Now()}); !errors.Is(err, ErrAlreadyQueued) {
		t.Fatalf("duplicate Enqueue() error = %v, want %v", err, ErrAlreadyQueued)
	}

	position, exists, err := queue.Position(ctx, 22)
	if err != nil || !exists || position != 2 {
		t.Fatalf("Position(22) = (%d, %t, %v), want (2, true, nil)", position, exists, err)
	}

	pair, err := queue.PopPair(ctx)
	if err != nil {
		t.Fatalf("PopPair() error = %v", err)
	}
	if len(pair) != 2 || pair[0].UserID != 11 || pair[1].UserID != 22 {
		t.Fatalf("PopPair() = %#v, want users 11 and 22", pair)
	}
	position, exists, err = queue.Position(ctx, 33)
	if err != nil || !exists || position != 1 {
		t.Fatalf("remaining Position(33) = (%d, %t, %v), want (1, true, nil)", position, exists, err)
	}

	removed, err := queue.Remove(ctx, 33)
	if err != nil || !removed {
		t.Fatalf("Remove(33) = (%t, %v), want (true, nil)", removed, err)
	}
	if pair, err := queue.PopPair(ctx); err != nil || len(pair) != 0 {
		t.Fatalf("empty PopPair() = (%#v, %v), want empty", pair, err)
	}
}
