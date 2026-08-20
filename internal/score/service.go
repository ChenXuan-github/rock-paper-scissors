package score

import (
	"context"
	"errors"
	"fmt"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/game"
)

// Service 负责玩家积分的基础业务。
// 这一阶段只提供初始化和查询；胜负后的积分变化留给 SettlementService 统一处理。
type Service struct {
	repository Repository
}

// NewService 注入积分 Repository；传入基于 *sql.Tx 的实现时，所有操作会加入同一事务。
func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

// CreateForUser 为新玩家建立初始值全部为零的积分记录。
func (s *Service) CreateForUser(ctx context.Context, userID int64) (PlayerScore, error) {
	if userID <= 0 {
		return PlayerScore{}, ErrInvalidUserID
	}

	created, err := s.repository.Create(ctx, PlayerScore{UserID: userID})
	if err != nil {
		return PlayerScore{}, fmt.Errorf("create score for user: %w", err)
	}
	return created, nil
}

// GetByUserID 获取一名玩家当前的永久积分汇总。
func (s *Service) GetByUserID(ctx context.Context, userID int64) (PlayerScore, error) {
	if userID <= 0 {
		return PlayerScore{}, ErrInvalidUserID
	}

	found, err := s.repository.FindByUserID(ctx, userID)
	if err != nil {
		return PlayerScore{}, fmt.Errorf("get score by user id: %w", err)
	}
	return found, nil
}

// GetByUserIDs 批量查询积分并按 UserID 建立 Map，供排行榜按 Redis 顺序快速组装结果。
func (s *Service) GetByUserIDs(ctx context.Context, userIDs []int64) (map[int64]PlayerScore, error) {
	for _, userID := range userIDs {
		if userID <= 0 {
			return nil, ErrInvalidUserID
		}
	}

	playerScores, err := s.repository.FindByUserIDs(ctx, userIDs)
	if err != nil {
		return nil, fmt.Errorf("get scores by user ids: %w", err)
	}
	scoresByUserID := make(map[int64]PlayerScore, len(playerScores))
	for _, playerScore := range playerScores {
		scoresByUserID[playerScore.UserID] = playerScore
	}
	return scoresByUserID, nil
}

// GetOrCreate 获取玩家积分；迁移后新注册且尚未产生积分行的用户会自动初始化为零。
func (s *Service) GetOrCreate(ctx context.Context, userID int64) (PlayerScore, error) {
	found, err := s.GetByUserID(ctx, userID)
	if err == nil {
		return found, nil
	}
	if !errors.Is(err, ErrScoreNotFound) {
		return PlayerScore{}, err
	}

	created, err := s.CreateForUser(ctx, userID)
	if err == nil {
		return created, nil
	}
	// 并发请求可能同时发现记录不存在；另一个请求先插入时，重新读取即可。
	if errors.Is(err, ErrScoreAlreadyExist) {
		return s.GetByUserID(ctx, userID)
	}
	return PlayerScore{}, err
}

// ListAll 返回 MySQL 中的全部积分汇总，供 Redis 排行榜在启动时重建。
func (s *Service) ListAll(ctx context.Context) ([]PlayerScore, error) {
	playerScores, err := s.repository.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all player scores: %w", err)
	}
	return playerScores, nil
}

// ApplyResult 把一局结果转换成玩家积分和胜负次数的增量。
// 非平局的 points 必须在 [11, 19]；赢家加 points，输家减 points，积分允许为负数。
func (s *Service) ApplyResult(
	ctx context.Context,
	userID int64,
	result game.Result,
	points int,
) (PlayerScore, error) {
	if userID <= 0 {
		return PlayerScore{}, ErrInvalidUserID
	}

	var change Change
	switch result {
	case game.Win:
		if !validWinPoints(points) {
			return PlayerScore{}, ErrInvalidPointValue
		}
		change.ScoreDelta = points
		change.Wins = 1
	case game.Lose:
		if !validWinPoints(points) {
			return PlayerScore{}, ErrInvalidPointValue
		}
		change.ScoreDelta = -points
		change.Losses = 1
	case game.Draw:
		// 平局固定不改变积分，要求调用方显式传 0，避免错误随机数被静默忽略。
		if points != 0 {
			return PlayerScore{}, ErrInvalidPointValue
		}
		change.Draws = 1
	default:
		// ResultPending 或任何强制转换出的非法值都不能计入永久积分。
		return PlayerScore{}, ErrInvalidResult
	}

	updated, err := s.repository.ApplyChange(ctx, userID, change)
	if err != nil {
		return PlayerScore{}, fmt.Errorf("apply score result: %w", err)
	}
	return updated, nil
}

// validWinPoints 集中维护胜负积分边界，避免 Win 和 Lose 分支各写一份判断。
func validWinPoints(points int) bool {
	return points >= MinWinPoints && points <= MaxWinPoints
}
