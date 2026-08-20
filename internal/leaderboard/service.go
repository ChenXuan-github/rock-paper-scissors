package leaderboard

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/score"
	"github.com/redis/go-redis/v9"
)

const scoresKey = "rps:leaderboard:scores"

// Entry 是 Redis ZSet 返回的一条排序结果；Rank 从 1 开始，便于直接展示。
type Entry struct {
	Rank   int
	UserID int64
	Score  int
}

// Service 使用 Redis ZSet 维护按积分从高到低排列的索引。
// MySQL player_scores 仍是真相源，Redis 只负责快速排序和名次查询。
type Service struct {
	client *redis.Client
}

// NewService 注入进程内共享的 Redis 客户端；Service 本身不重复创建连接池。
func NewService(client *redis.Client) *Service {
	return &Service{client: client}
}

// ReplaceAll 使用 MySQL 的完整积分数据重建排行榜，修复 Redis 重启或短暂失败造成的偏差。
func (s *Service) ReplaceAll(ctx context.Context, playerScores []score.PlayerScore) error {
	if s == nil || s.client == nil {
		return errors.New("leaderboard redis client is nil")
	}

	members := make([]redis.Z, 0, len(playerScores))
	for _, playerScore := range playerScores {
		if playerScore.UserID <= 0 {
			return errors.New("invalid leaderboard user id")
		}
		// ZSet member 唯一标识用户，score 决定排序；重复用户会被覆盖为最新总分。
		members = append(members, redis.Z{
			Score:  float64(playerScore.Score),
			Member: strconv.FormatInt(playerScore.UserID, 10),
		})
	}

	// 删除旧榜和写入新榜通过 MULTI/EXEC 一起提交，避免客户端看到只重建了一半的榜单。
	_, err := s.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Del(ctx, scoresKey)
		if len(members) > 0 {
			pipe.ZAdd(ctx, scoresKey, members...)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("replace leaderboard: %w", err)
	}
	return nil
}

// UpdateScores 在结算事务提交后，把双方最新总分覆盖写入 ZSet。
func (s *Service) UpdateScores(ctx context.Context, playerScores ...score.PlayerScore) error {
	if s == nil || s.client == nil {
		return errors.New("leaderboard redis client is nil")
	}
	members := make([]redis.Z, 0, len(playerScores))
	for _, playerScore := range playerScores {
		if playerScore.UserID <= 0 {
			return errors.New("invalid leaderboard user id")
		}
		members = append(members, redis.Z{
			Score:  float64(playerScore.Score),
			Member: strconv.FormatInt(playerScore.UserID, 10),
		})
	}
	if len(members) == 0 {
		return nil
	}
	// ZADD 对已有 member 是覆盖总分而不是累加，数据来源始终是 MySQL 提交后的最新值。
	if err := s.client.ZAdd(ctx, scoresKey, members...).Err(); err != nil {
		return fmt.Errorf("update leaderboard scores: %w", err)
	}
	return nil
}

// Top 返回排行榜前 limit 名。ZRevRange 表示按 score 从高到低读取。
func (s *Service) Top(ctx context.Context, limit int) ([]Entry, error) {
	if s == nil || s.client == nil || limit <= 0 || limit > 100 {
		return nil, errors.New("invalid leaderboard query")
	}

	// Redis 区间下标左右都包含，因此前 limit 名的结束下标是 limit-1。
	values, err := s.client.ZRevRangeWithScores(ctx, scoresKey, 0, int64(limit-1)).Result()
	if err != nil {
		return nil, fmt.Errorf("read leaderboard: %w", err)
	}
	entries := make([]Entry, 0, len(values))
	for index, value := range values {
		// 写入时 UserID 被转换为字符串，读取后再恢复为业务层使用的 int64。
		member, ok := value.Member.(string)
		if !ok {
			return nil, errors.New("invalid leaderboard member")
		}
		userID, err := strconv.ParseInt(member, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse leaderboard user id: %w", err)
		}
		entries = append(entries, Entry{
			Rank:   index + 1,
			UserID: userID,
			Score:  int(value.Score),
		})
	}
	return entries, nil
}

// Rank 查询指定用户的名次；Redis 内部排名从 0 开始，因此返回时加 1。
func (s *Service) Rank(ctx context.Context, userID int64) (*int, error) {
	if s == nil || s.client == nil || userID <= 0 {
		return nil, errors.New("invalid leaderboard user id")
	}
	rank, err := s.client.ZRevRank(ctx, scoresKey, strconv.FormatInt(userID, 10)).Result()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read leaderboard rank: %w", err)
	}
	value := int(rank) + 1
	return &value, nil
}
