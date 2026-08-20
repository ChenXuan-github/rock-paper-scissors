package score

import "context"

// Repository 定义玩家积分的持久化能力。
// 后续 SettlementService 仍然只依赖接口，不直接依赖 MySQL 实现。
type Repository interface {
	Create(ctx context.Context, playerScore PlayerScore) (PlayerScore, error)
	FindByUserID(ctx context.Context, userID int64) (PlayerScore, error)
	FindByUserIDs(ctx context.Context, userIDs []int64) ([]PlayerScore, error)
	ListAll(ctx context.Context) ([]PlayerScore, error)
	ApplyChange(ctx context.Context, userID int64, change Change) (PlayerScore, error)
}
