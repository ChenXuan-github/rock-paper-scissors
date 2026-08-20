package record

import "context"

// Repository 定义对战记录的持久化和历史查询能力。
type Repository interface {
	Create(ctx context.Context, gameRecord GameRecord) (GameRecord, error)
	FindByID(ctx context.Context, id int64) (GameRecord, error)
	ListByUserID(ctx context.Context, userID int64, limit, offset int) ([]GameRecord, error)
}
