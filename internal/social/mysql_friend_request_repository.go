package social

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/database"
	mysqlDriver "github.com/go-sql-driver/mysql"
)

// MySQLFriendRequestRepository 使用 friend_requests 表保存有方向的申请及其生命周期。
type MySQLFriendRequestRepository struct {
	executor database.Executor
}

// NewMySQLFriendRequestRepository 注入普通连接池或事务执行器。
func NewMySQLFriendRequestRepository(executor database.Executor) *MySQLFriendRequestRepository {
	return &MySQLFriendRequestRepository{executor: executor}
}

// Create 插入一条新的 pending 申请；数据库生成列负责规范化用户对并阻止反向重复申请。
func (r *MySQLFriendRequestRepository) Create(
	ctx context.Context,
	request FriendRequest,
) (FriendRequest, error) {
	if _, _, err := canonicalUserPair(request.RequesterID, request.ReceiverID); err != nil {
		return FriendRequest{}, err
	}
	if request.Status != FriendRequestPending {
		return FriendRequest{}, ErrInvalidFriendRequestStatus
	}

	const query = `
		INSERT INTO friend_requests (requester_id, receiver_id, status)
		VALUES (?, ?, ?)
	`
	// pair_user_id_low/high 由 MySQL 生成列计算，应用层只写有方向的 requester/receiver。
	result, err := r.executor.ExecContext(
		ctx,
		query,
		request.RequesterID,
		request.ReceiverID,
		request.Status,
	)
	if err != nil {
		var mysqlErr *mysqlDriver.MySQLError
		// 用户对唯一索引同时阻止 A→B 和 B→A 的重复生命周期记录。
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return FriendRequest{}, ErrFriendRequestAlreadyExists
		}
		return FriendRequest{}, fmt.Errorf("create friend request: %w", err)
	}

	// 获取自增主键后重新查询，统一取得生成列和数据库时间字段。
	requestID, err := result.LastInsertId()
	if err != nil {
		return FriendRequest{}, fmt.Errorf("read created friend request id: %w", err)
	}
	return r.FindByID(ctx, requestID)
}

// Reopen 把 accepted/rejected/cancelled 的旧记录恢复为新的 pending 申请。
// requester/receiver 可以交换方向，但生成列表示的无方向用户对保持不变。
func (r *MySQLFriendRequestRepository) Reopen(
	ctx context.Context,
	requestID, requesterID, receiverID int64,
) (FriendRequest, error) {
	if requestID <= 0 {
		return FriendRequest{}, ErrFriendRequestNotFound
	}
	if _, _, err := canonicalUserPair(requesterID, receiverID); err != nil {
		return FriendRequest{}, err
	}

	const query = `
		UPDATE friend_requests
		SET requester_id = ?,
			receiver_id = ?,
			status = ?,
			created_at = CURRENT_TIMESTAMP,
			responded_at = NULL
		WHERE id = ? AND status IN (?, ?, ?)
	`
	// WHERE 只允许结束状态重开；若记录又被并发改为 pending，本次更新不会命中。
	result, err := r.executor.ExecContext(
		ctx,
		query,
		requesterID,
		receiverID,
		FriendRequestPending,
		requestID,
		FriendRequestAccepted,
		FriendRequestRejected,
		FriendRequestCancelled,
	)
	if err != nil {
		return FriendRequest{}, fmt.Errorf("reopen friend request: %w", err)
	}
	if err := r.requireChangedRequest(ctx, requestID, result); err != nil {
		return FriendRequest{}, err
	}
	return r.FindByID(ctx, requestID)
}

// FindByID 根据申请主键读取完整申请。
func (r *MySQLFriendRequestRepository) FindByID(
	ctx context.Context,
	requestID int64,
) (FriendRequest, error) {
	if requestID <= 0 {
		return FriendRequest{}, ErrFriendRequestNotFound
	}
	const query = `
		SELECT id, requester_id, receiver_id,
			pair_user_id_low, pair_user_id_high,
			status, created_at, updated_at, responded_at
		FROM friend_requests
		WHERE id = ?
	`
	return scanFriendRequest(r.executor.QueryRowContext(ctx, query, requestID))
}

// FindByPair 忽略申请方向，查询两个用户之间唯一的申请生命周期记录。
func (r *MySQLFriendRequestRepository) FindByPair(
	ctx context.Context,
	firstUserID, secondUserID int64,
) (FriendRequest, error) {
	low, high, err := canonicalUserPair(firstUserID, secondUserID)
	if err != nil {
		return FriendRequest{}, err
	}
	const query = `
		SELECT id, requester_id, receiver_id,
			pair_user_id_low, pair_user_id_high,
			status, created_at, updated_at, responded_at
		FROM friend_requests
		WHERE pair_user_id_low = ? AND pair_user_id_high = ?
	`
	return scanFriendRequest(r.executor.QueryRowContext(ctx, query, low, high))
}

// ListIncoming 分页查询指定用户收到的某一状态申请。
func (r *MySQLFriendRequestRepository) ListIncoming(
	ctx context.Context,
	receiverID int64,
	status FriendRequestStatus,
	limit, offset int,
) ([]FriendRequest, error) {
	return r.list(ctx, "receiver_id", receiverID, status, limit, offset)
}

// ListOutgoing 分页查询指定用户发出的某一状态申请。
func (r *MySQLFriendRequestRepository) ListOutgoing(
	ctx context.Context,
	requesterID int64,
	status FriendRequestStatus,
	limit, offset int,
) ([]FriendRequest, error) {
	return r.list(ctx, "requester_id", requesterID, status, limit, offset)
}

// UpdateStatus 通过 id + expectedStatus 条件原子流转申请状态。
// 若两个请求同时处理同一申请，只有第一个能更新成功，后一个得到 ErrFriendRequestStateChanged。
func (r *MySQLFriendRequestRepository) UpdateStatus(
	ctx context.Context,
	requestID int64,
	expectedStatus FriendRequestStatus,
	nextStatus FriendRequestStatus,
	respondedAt *time.Time,
) (FriendRequest, error) {
	if requestID <= 0 {
		return FriendRequest{}, ErrFriendRequestNotFound
	}
	if !expectedStatus.Valid() || !nextStatus.Valid() || expectedStatus == nextStatus {
		return FriendRequest{}, ErrInvalidFriendRequestStatus
	}

	const query = `
		UPDATE friend_requests
		SET status = ?, responded_at = ?
		WHERE id = ? AND status = ?
	`
	// id + expectedStatus 相当于轻量乐观锁：只有仍处于预期状态的请求能修改成功。
	result, err := r.executor.ExecContext(ctx, query, nextStatus, respondedAt, requestID, expectedStatus)
	if err != nil {
		return FriendRequest{}, fmt.Errorf("update friend request status: %w", err)
	}
	if err := r.requireChangedRequest(ctx, requestID, result); err != nil {
		return FriendRequest{}, err
	}
	return r.FindByID(ctx, requestID)
}

// list 复用收件箱与发件箱查询；column 只由上面两个固定调用传入，不接受用户输入。
func (r *MySQLFriendRequestRepository) list(
	ctx context.Context,
	column string,
	userID int64,
	status FriendRequestStatus,
	limit, offset int,
) ([]FriendRequest, error) {
	if userID <= 0 {
		return nil, ErrInvalidUserPair
	}
	if !status.Valid() {
		return nil, ErrInvalidFriendRequestStatus
	}
	if limit <= 0 || limit > 100 || offset < 0 {
		return nil, ErrInvalidFriendRequestPage
	}

	// 这里只动态替换列名，列名只能由 ListIncoming/ListOutgoing 两个固定入口传入。
	// userID、status、limit、offset 仍全部使用占位符，不能让客户端控制 SQL 结构。
	query := fmt.Sprintf(`
		SELECT id, requester_id, receiver_id,
			pair_user_id_low, pair_user_id_high,
			status, created_at, updated_at, responded_at
		FROM friend_requests
		WHERE %s = ? AND status = ?
		ORDER BY created_at DESC, id DESC
		LIMIT ? OFFSET ?
	`, column)
	rows, err := r.executor.QueryContext(ctx, query, userID, status, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list friend requests: %w", err)
	}
	// 及时关闭结果集，把占用的连接归还给 database/sql 连接池。
	defer rows.Close()

	requests := make([]FriendRequest, 0, limit)
	for rows.Next() {
		request, err := scanFriendRequest(rows)
		if err != nil {
			return nil, err
		}
		requests = append(requests, request)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate friend requests: %w", err)
	}
	return requests, nil
}

// requireChangedRequest 区分“记录不存在”和“记录存在但状态已被其他请求改变”。
func (r *MySQLFriendRequestRepository) requireChangedRequest(
	ctx context.Context,
	requestID int64,
	result sql.Result,
) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read changed friend request rows: %w", err)
	}
	if affected > 0 {
		return nil
	}
	// 0 行可能是主键不存在，也可能是记录存在但状态已经变化；再查一次进行精确区分。
	if _, err := r.FindByID(ctx, requestID); err != nil {
		return err
	}
	return ErrFriendRequestStateChanged
}

// friendRequestRowScanner 让 *sql.Row 与 *sql.Rows 共用同一套申请字段映射逻辑。
type friendRequestRowScanner interface {
	Scan(dest ...any) error
}

func scanFriendRequest(row friendRequestRowScanner) (FriendRequest, error) {
	var (
		request FriendRequest
		status  string
		// SQL NULL 不能直接 Scan 到 time.Time，使用 NullTime 同时保存“是否有值”。
		respondedAt sql.NullTime
	)
	if err := row.Scan(
		&request.ID,
		&request.RequesterID,
		&request.ReceiverID,
		&request.PairUserIDLow,
		&request.PairUserIDHigh,
		&status,
		&request.CreatedAt,
		&request.UpdatedAt,
		&respondedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return FriendRequest{}, ErrFriendRequestNotFound
		}
		return FriendRequest{}, fmt.Errorf("scan friend request: %w", err)
	}

	// 数据库读出的字符串仍要回到领域类型，并防御数据库中的非法历史值。
	request.Status = FriendRequestStatus(status)
	if !request.Status.Valid() {
		return FriendRequest{}, ErrInvalidFriendRequestStatus
	}
	if respondedAt.Valid {
		value := respondedAt.Time
		request.RespondedAt = &value
	}
	return request, nil
}
