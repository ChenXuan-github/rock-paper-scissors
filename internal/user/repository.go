package user

import "context"

// Repository 定义用户数据的持久化能力。
// Service 只依赖该接口，不关心生产环境使用 MySQL，还是测试使用内存 map。
type Repository interface {
	// Context 作为第一个参数，把请求取消和超时信号传递给数据库操作。
	Create(ctx context.Context, user User) (User, error)
	FindByID(ctx context.Context, id int64) (User, error)
	FindByIDs(ctx context.Context, ids []int64) ([]User, error)
	FindByUsername(ctx context.Context, username string) (User, error)
}
