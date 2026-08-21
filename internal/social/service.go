package social

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/user"
)

// userQueryService 是社交业务需要的最小用户查询能力。
// 依赖小接口后，测试不需要连接真实 MySQL，也不用依赖 UserService 的其他方法。
type userQueryService interface {
	GetByID(ctx context.Context, id int64) (user.User, error)
	GetByIDs(ctx context.Context, ids []int64) (map[int64]user.User, error)
}

// Service 编排好友关系和好友申请业务。
// db 会在后续“接受好友申请”时开启事务，保证申请状态和好友关系同时成功或同时失败。
type Service struct {
	db                      *sql.DB
	friendshipRepository    FriendshipRepository
	friendRequestRepository FriendRequestRepository
	users                   userQueryService
}

// AcceptFriendRequestResult 是接受申请事务成功后的完整结果。
type AcceptFriendRequestResult struct {
	Request    FriendRequest
	Friendship Friendship
}

// NewService 创建社交业务服务。
func NewService(db *sql.DB, users userQueryService) *Service {
	return &Service{
		db:                      db,
		friendshipRepository:    NewMySQLFriendshipRepository(db),
		friendRequestRepository: NewMySQLFriendRequestRepository(db),
		users:                   users,
	}
}

// SendFriendRequest 发送好友申请。
// 新用户对插入申请；被拒绝、已撤回或解除过好友关系的用户对复用原记录重新申请。
func (s *Service) SendFriendRequest(
	ctx context.Context,
	requesterID, receiverID int64,
) (FriendRequest, error) {
	request, err := NewFriendRequest(requesterID, receiverID)
	if err != nil {
		return FriendRequest{}, err
	}
	if s.users == nil || s.friendshipRepository == nil || s.friendRequestRepository == nil {
		return FriendRequest{}, ErrSocialServiceNotInitialized
	}

	// requesterID 来自已通过 JWT 校验的请求上下文；receiverID 来自客户端，因此需要确认目标用户存在。
	if _, err := s.users.GetByID(ctx, receiverID); err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			return FriendRequest{}, ErrSocialUserNotFound
		}
		return FriendRequest{}, fmt.Errorf("find friend request receiver: %w", err)
	}

	// 好友边已经存在时不能再次发送申请。
	exists, err := s.friendshipRepository.Exists(ctx, requesterID, receiverID)
	if err != nil {
		return FriendRequest{}, fmt.Errorf("check existing friendship: %w", err)
	}
	if exists {
		return FriendRequest{}, ErrAlreadyFriends
	}

	// friend_requests 对同一用户对只保存一条生命周期记录，因此先判断是否已经存在。
	existing, err := s.friendRequestRepository.FindByPair(ctx, requesterID, receiverID)
	if err != nil {
		if errors.Is(err, ErrFriendRequestNotFound) {
			created, createErr := s.friendRequestRepository.Create(ctx, request)
			if createErr != nil {
				return FriendRequest{}, fmt.Errorf("create friend request: %w", createErr)
			}
			return created, nil
		}
		return FriendRequest{}, fmt.Errorf("find existing friend request: %w", err)
	}

	if existing.Status == FriendRequestPending {
		return FriendRequest{}, ErrFriendRequestPending
	}

	// rejected/cancelled 可以重新申请；accepted 但好友边已不存在，说明双方曾经解除好友，也允许重开。
	reopened, err := s.friendRequestRepository.Reopen(
		ctx,
		existing.ID,
		requesterID,
		receiverID,
	)
	if err != nil {
		return FriendRequest{}, fmt.Errorf("reopen friend request: %w", err)
	}
	return reopened, nil
}

// AcceptFriendRequest 接受一条发给当前用户的待处理好友申请。
// 申请状态更新与好友边创建位于同一个 MySQL 事务中，不会出现“申请已接受但好友关系没创建”的半成品数据。
func (s *Service) AcceptFriendRequest(
	ctx context.Context,
	requestID, currentUserID int64,
) (AcceptFriendRequestResult, error) {
	if s.db == nil {
		return AcceptFriendRequestResult{}, ErrSocialServiceNotInitialized
	}
	if requestID <= 0 {
		return AcceptFriendRequestResult{}, ErrFriendRequestNotFound
	}
	if currentUserID <= 0 {
		return AcceptFriendRequestResult{}, ErrInvalidUserPair
	}

	// 事务开始方式与 Day 6 的对局结算一致；nil 使用 MySQL 驱动默认事务隔离级别。
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AcceptFriendRequestResult{}, fmt.Errorf("begin accepting friend request transaction: %w", err)
	}
	// Commit 成功后再次 Rollback 只会得到 sql.ErrTxDone，因此可以统一忽略。
	defer func() {
		_ = tx.Rollback()
	}()

	// 两个 Repository 注入同一个 *sql.Tx，下面的 SQL 才真正属于同一个事务。
	requestRepository := NewMySQLFriendRequestRepository(tx)
	friendshipRepository := NewMySQLFriendshipRepository(tx)

	request, err := requestRepository.FindByID(ctx, requestID)
	if err != nil {
		return AcceptFriendRequestResult{}, fmt.Errorf("find friend request to accept: %w", err)
	}
	// currentUserID 来自 JWT 校验后的 gin.Context，只有申请接收者才能点击同意。
	if request.ReceiverID != currentUserID {
		return AcceptFriendRequestResult{}, ErrFriendRequestNotReceiver
	}
	if request.Status != FriendRequestPending {
		return AcceptFriendRequestResult{}, ErrFriendRequestNotPending
	}

	now := time.Now().UTC()
	acceptedRequest, err := requestRepository.UpdateStatus(
		ctx,
		request.ID,
		FriendRequestPending,
		FriendRequestAccepted,
		&now,
	)
	if err != nil {
		return AcceptFriendRequestResult{}, fmt.Errorf("accept friend request: %w", err)
	}

	// 状态更新使用 id + pending 作为并发条件；抢先处理成功的事务才会继续创建好友边。
	friendship, err := friendshipRepository.Create(ctx, Friendship{
		UserIDLow:  request.RequesterID,
		UserIDHigh: request.ReceiverID,
	})
	if err != nil {
		if errors.Is(err, ErrFriendshipAlreadyExists) {
			return AcceptFriendRequestResult{}, ErrAlreadyFriends
		}
		return AcceptFriendRequestResult{}, fmt.Errorf("create accepted friendship: %w", err)
	}

	// 两次写操作都成功后才提交；此前任何 return 都由 defer 回滚。
	if err := tx.Commit(); err != nil {
		return AcceptFriendRequestResult{}, fmt.Errorf("commit accepting friend request transaction: %w", err)
	}

	return AcceptFriendRequestResult{
		Request:    acceptedRequest,
		Friendship: friendship,
	}, nil
}

// RejectFriendRequest 让申请接收者拒绝一条 pending 申请。
func (s *Service) RejectFriendRequest(
	ctx context.Context,
	requestID, currentUserID int64,
) (FriendRequest, error) {
	if s.friendRequestRepository == nil {
		return FriendRequest{}, ErrSocialServiceNotInitialized
	}
	request, err := s.findPendingFriendRequest(ctx, requestID)
	if err != nil {
		return FriendRequest{}, err
	}
	// JWT 中的当前用户必须是 receiver，发送者不能替接收者拒绝自己的申请。
	if request.ReceiverID != currentUserID {
		return FriendRequest{}, ErrFriendRequestNotReceiver
	}

	now := time.Now().UTC()
	rejected, err := s.friendRequestRepository.UpdateStatus(
		ctx,
		request.ID,
		FriendRequestPending,
		FriendRequestRejected,
		&now,
	)
	if err != nil {
		return FriendRequest{}, fmt.Errorf("reject friend request: %w", err)
	}
	return rejected, nil
}

// CancelFriendRequest 让申请发送者在对方处理前撤销一条 pending 申请。
func (s *Service) CancelFriendRequest(
	ctx context.Context,
	requestID, currentUserID int64,
) (FriendRequest, error) {
	if s.friendRequestRepository == nil {
		return FriendRequest{}, ErrSocialServiceNotInitialized
	}
	request, err := s.findPendingFriendRequest(ctx, requestID)
	if err != nil {
		return FriendRequest{}, err
	}
	// JWT 中的当前用户必须是 requester，接收者应使用接受或拒绝操作。
	if request.RequesterID != currentUserID {
		return FriendRequest{}, ErrFriendRequestNotRequester
	}

	// 撤销不是接收者作出的响应，因此 responded_at 保持 NULL；updated_at 仍会由 MySQL 自动更新。
	cancelled, err := s.friendRequestRepository.UpdateStatus(
		ctx,
		request.ID,
		FriendRequestPending,
		FriendRequestCancelled,
		nil,
	)
	if err != nil {
		return FriendRequest{}, fmt.Errorf("cancel friend request: %w", err)
	}
	return cancelled, nil
}

// RemoveFriend 删除当前用户与目标用户之间的无向好友边。
func (s *Service) RemoveFriend(ctx context.Context, currentUserID, friendUserID int64) error {
	if s.friendshipRepository == nil {
		return ErrSocialServiceNotInitialized
	}
	// NewFriendship 复用相同的用户对规则，提前拒绝无效 ID 和删除自己的请求。
	if _, err := NewFriendship(currentUserID, friendUserID); err != nil {
		return err
	}

	deleted, err := s.friendshipRepository.Delete(ctx, currentUserID, friendUserID)
	if err != nil {
		return fmt.Errorf("remove friendship: %w", err)
	}
	if !deleted {
		return ErrFriendshipNotFound
	}
	return nil
}

// findPendingFriendRequest 统一完成申请主键校验、查询和 pending 状态校验。
func (s *Service) findPendingFriendRequest(ctx context.Context, requestID int64) (FriendRequest, error) {
	if requestID <= 0 {
		return FriendRequest{}, ErrFriendRequestNotFound
	}
	request, err := s.friendRequestRepository.FindByID(ctx, requestID)
	if err != nil {
		return FriendRequest{}, fmt.Errorf("find pending friend request: %w", err)
	}
	if request.Status != FriendRequestPending {
		return FriendRequest{}, ErrFriendRequestNotPending
	}
	return request, nil
}
