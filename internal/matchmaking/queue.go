package matchmaking

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const defaultQueueKey = "rps:matchmaking:queue"

// popPairScript 把“检查至少两人”和“移除队首两人”放在同一次 Redis 执行中。
// Redis 会原子执行 Lua 脚本，因此多个 Gin 请求并发尝试匹配时不会取到同一名玩家。
var popPairScript = redis.NewScript(`
if redis.call("ZCARD", KEYS[1]) < 2 then
    return {}
end

local pair = redis.call("ZRANGE", KEYS[1], 0, 1, "WITHSCORES")
redis.call("ZREM", KEYS[1], pair[1], pair[3])
return pair
`)

// QueueEntry 保存玩家身份和原始入队顺序。
// EnqueuedAt 在重新入队时复用，避免内部错误让等待已久的玩家跑到队尾。
type QueueEntry struct {
	UserID     int64
	EnqueuedAt time.Time
}

// Queue 描述 MatchmakingService 依赖的队列能力。
// RedisQueue 是生产实现，测试可以使用内存假实现而不依赖真实 Redis。
type Queue interface {
	Enqueue(ctx context.Context, entry QueueEntry) error
	Requeue(ctx context.Context, entries ...QueueEntry) error
	Remove(ctx context.Context, userID int64) (bool, error)
	Position(ctx context.Context, userID int64) (position int, exists bool, err error)
	PopPair(ctx context.Context) ([]QueueEntry, error)
	Clear(ctx context.Context) error
}

// RedisQueue 使用 ZSet 保存匹配队列：member 是 UserID，score 是入队毫秒时间戳。
type RedisQueue struct {
	client *redis.Client
	key    string
}

// NewRedisQueue 创建使用项目固定 Key 的匹配队列。
func NewRedisQueue(client *redis.Client) *RedisQueue {
	return newRedisQueue(client, defaultQueueKey)
}

func newRedisQueue(client *redis.Client, key string) *RedisQueue {
	return &RedisQueue{client: client, key: key}
}

// Enqueue 使用 NX 写入，保证同一个 UserID 不能重复进入队列。
func (q *RedisQueue) Enqueue(ctx context.Context, entry QueueEntry) error {
	if err := q.validate(entry.UserID); err != nil {
		return err
	}
	if entry.EnqueuedAt.IsZero() {
		entry.EnqueuedAt = time.Now()
	}

	added, err := q.client.ZAddArgs(ctx, q.key, redis.ZAddArgs{
		NX: true,
		Members: []redis.Z{{
			Score:  float64(entry.EnqueuedAt.UnixMilli()),
			Member: strconv.FormatInt(entry.UserID, 10),
		}},
	}).Result()
	if err != nil {
		return fmt.Errorf("enqueue matchmaking player: %w", err)
	}
	if added == 0 {
		return ErrAlreadyQueued
	}
	return nil
}

// Requeue 把创建房间失败但仍有效的玩家按原始时间重新放回队列。
func (q *RedisQueue) Requeue(ctx context.Context, entries ...QueueEntry) error {
	if q == nil || q.client == nil {
		return errors.New("matchmaking redis client is nil")
	}
	if len(entries) == 0 {
		return nil
	}

	members := make([]redis.Z, 0, len(entries))
	for _, entry := range entries {
		if entry.UserID <= 0 {
			return ErrInvalidUserID
		}
		if entry.EnqueuedAt.IsZero() {
			entry.EnqueuedAt = time.Now()
		}
		members = append(members, redis.Z{
			Score:  float64(entry.EnqueuedAt.UnixMilli()),
			Member: strconv.FormatInt(entry.UserID, 10),
		})
	}
	if err := q.client.ZAddArgs(ctx, q.key, redis.ZAddArgs{NX: true, Members: members}).Err(); err != nil {
		return fmt.Errorf("requeue matchmaking players: %w", err)
	}
	return nil
}

// Remove 删除指定玩家；bool=false 表示队列中原本不存在该玩家。
func (q *RedisQueue) Remove(ctx context.Context, userID int64) (bool, error) {
	if err := q.validate(userID); err != nil {
		return false, err
	}
	removed, err := q.client.ZRem(ctx, q.key, strconv.FormatInt(userID, 10)).Result()
	if err != nil {
		return false, fmt.Errorf("remove matchmaking player: %w", err)
	}
	return removed > 0, nil
}

// Position 返回从 1 开始的排队位置；ZRank 的原始结果从 0 开始。
func (q *RedisQueue) Position(ctx context.Context, userID int64) (int, bool, error) {
	if err := q.validate(userID); err != nil {
		return 0, false, err
	}
	rank, err := q.client.ZRank(ctx, q.key, strconv.FormatInt(userID, 10)).Result()
	if errors.Is(err, redis.Nil) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("read matchmaking position: %w", err)
	}
	return int(rank) + 1, true, nil
}

// PopPair 原子取出等待时间最长的两名玩家；不足两人时返回空切片。
func (q *RedisQueue) PopPair(ctx context.Context) ([]QueueEntry, error) {
	if q == nil || q.client == nil {
		return nil, errors.New("matchmaking redis client is nil")
	}
	values, err := popPairScript.Run(ctx, q.client, []string{q.key}).StringSlice()
	if err != nil {
		return nil, fmt.Errorf("pop matchmaking pair: %w", err)
	}
	if len(values) == 0 {
		return []QueueEntry{}, nil
	}
	if len(values) != 4 {
		return nil, fmt.Errorf("invalid matchmaking pair response length: %d", len(values))
	}

	entries := make([]QueueEntry, 0, 2)
	for index := 0; index < len(values); index += 2 {
		userID, err := strconv.ParseInt(values[index], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse matchmaking user id: %w", err)
		}
		timestamp, err := strconv.ParseInt(values[index+1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse matchmaking timestamp: %w", err)
		}
		entries = append(entries, QueueEntry{
			UserID:     userID,
			EnqueuedAt: time.UnixMilli(timestamp),
		})
	}
	return entries, nil
}

// Clear 清除进程重启前遗留的队列。当前房间和 WebSocket 都是本机内存状态，不能复用旧队列。
func (q *RedisQueue) Clear(ctx context.Context) error {
	if q == nil || q.client == nil {
		return errors.New("matchmaking redis client is nil")
	}
	if err := q.client.Del(ctx, q.key).Err(); err != nil {
		return fmt.Errorf("clear matchmaking queue: %w", err)
	}
	return nil
}

func (q *RedisQueue) validate(userID int64) error {
	if q == nil || q.client == nil {
		return errors.New("matchmaking redis client is nil")
	}
	if userID <= 0 {
		return ErrInvalidUserID
	}
	return nil
}
