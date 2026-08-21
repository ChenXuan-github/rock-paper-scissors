package social

import (
	"context"
	"time"
)

// FriendshipRepository 定义无向好友边的持久化能力。
// Service 只依赖该接口；生产环境使用 MySQL，测试可以使用内存 Map 模拟图的邻接关系。
type FriendshipRepository interface {
	Create(ctx context.Context, friendship Friendship) (Friendship, error)
	Delete(ctx context.Context, firstUserID, secondUserID int64) (bool, error)
	Exists(ctx context.Context, firstUserID, secondUserID int64) (bool, error)
	ListFriendIDs(ctx context.Context, userID int64) ([]int64, error)
}

// FriendRequestRepository 定义有向好友申请的持久化和状态变更能力。
type FriendRequestRepository interface {
	Create(ctx context.Context, request FriendRequest) (FriendRequest, error)
	// Reopen 复用已经结束的用户对记录，重新设置申请方向并恢复为 pending。
	Reopen(ctx context.Context, requestID, requesterID, receiverID int64) (FriendRequest, error)
	FindByID(ctx context.Context, requestID int64) (FriendRequest, error)
	FindByPair(ctx context.Context, firstUserID, secondUserID int64) (FriendRequest, error)
	ListIncoming(
		ctx context.Context,
		receiverID int64,
		status FriendRequestStatus,
		limit, offset int,
	) ([]FriendRequest, error)
	ListOutgoing(
		ctx context.Context,
		requesterID int64,
		status FriendRequestStatus,
		limit, offset int,
	) ([]FriendRequest, error)
	// UpdateStatus 使用 expectedStatus 作为乐观并发条件，避免同一申请被接受或拒绝两次。
	UpdateStatus(
		ctx context.Context,
		requestID int64,
		expectedStatus FriendRequestStatus,
		nextStatus FriendRequestStatus,
		respondedAt *time.Time,
	) (FriendRequest, error)
}
