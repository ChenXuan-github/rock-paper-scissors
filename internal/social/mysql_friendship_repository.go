package social

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/database"
	mysqlDriver "github.com/go-sql-driver/mysql"
)

// MySQLFriendshipRepository 使用 friendships 表保存好友图中的无向边。
type MySQLFriendshipRepository struct {
	executor database.Executor
}

// NewMySQLFriendshipRepository 注入普通连接池或事务执行器。
func NewMySQLFriendshipRepository(executor database.Executor) *MySQLFriendshipRepository {
	return &MySQLFriendshipRepository{executor: executor}
}

// Create 规范化用户顺序后插入一条无向边。
func (r *MySQLFriendshipRepository) Create(
	ctx context.Context,
	friendship Friendship,
) (Friendship, error) {
	// Repository 再做一次规范化，不能假设所有调用方都已经保证 low < high。
	normalized, err := NewFriendship(friendship.UserIDLow, friendship.UserIDHigh)
	if err != nil {
		return Friendship{}, err
	}

	const query = `
		INSERT INTO friendships (user_id_low, user_id_high)
		VALUES (?, ?)
	`
	// 使用占位符绑定参数，避免把用户输入拼进 SQL。
	if _, err := r.executor.ExecContext(
		ctx,
		query,
		normalized.UserIDLow,
		normalized.UserIDHigh,
	); err != nil {
		var mysqlErr *mysqlDriver.MySQLError
		// 联合主键冲突说明同一条无向边已经存在，把驱动错误翻译成领域错误。
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return Friendship{}, ErrFriendshipAlreadyExists
		}
		return Friendship{}, fmt.Errorf("create friendship: %w", err)
	}

	// 创建后重新读取数据库生成的 created_at。
	return r.find(ctx, normalized.UserIDLow, normalized.UserIDHigh)
}

// Delete 删除两个用户之间唯一的规范化无向边。
func (r *MySQLFriendshipRepository) Delete(
	ctx context.Context,
	firstUserID, secondUserID int64,
) (bool, error) {
	low, high, err := canonicalUserPair(firstUserID, secondUserID)
	if err != nil {
		return false, err
	}

	const query = `
		DELETE FROM friendships
		WHERE user_id_low = ? AND user_id_high = ?
	`
	result, err := r.executor.ExecContext(ctx, query, low, high)
	if err != nil {
		return false, fmt.Errorf("delete friendship: %w", err)
	}
	// DELETE 不报错不等于真的删除了记录，RowsAffected 用于区分“不存在”和“删除成功”。
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read deleted friendship rows: %w", err)
	}
	return affected > 0, nil
}

// Exists 判断两个节点之间是否存在直接边；它不会沿图继续搜索间接关系。
func (r *MySQLFriendshipRepository) Exists(
	ctx context.Context,
	firstUserID, secondUserID int64,
) (bool, error) {
	low, high, err := canonicalUserPair(firstUserID, secondUserID)
	if err != nil {
		return false, err
	}

	const query = `
		SELECT EXISTS(
			SELECT 1
			FROM friendships
			WHERE user_id_low = ? AND user_id_high = ?
		)
	`
	// SELECT EXISTS 只返回布尔值，不需要把整行好友数据传回应用层。
	var exists bool
	if err := r.executor.QueryRowContext(ctx, query, low, high).Scan(&exists); err != nil {
		return false, fmt.Errorf("check friendship existence: %w", err)
	}
	return exists, nil
}

// ListFriendIDs 查询当前用户在无向图中的全部直接邻接节点。
func (r *MySQLFriendshipRepository) ListFriendIDs(ctx context.Context, userID int64) ([]int64, error) {
	if userID <= 0 {
		return nil, ErrInvalidUserPair
	}

	// 拆成 UNION ALL 后，两部分分别利用主键前缀和反向索引，避免 OR 查询退化。
	const query = `
		SELECT user_id_high AS friend_id
		FROM friendships
		WHERE user_id_low = ?
		UNION ALL
		SELECT user_id_low AS friend_id
		FROM friendships
		WHERE user_id_high = ?
		ORDER BY friend_id
	`
	rows, err := r.executor.QueryContext(ctx, query, userID, userID)
	if err != nil {
		return nil, fmt.Errorf("list friend ids: %w", err)
	}
	// QueryContext 返回的结果集占用数据库连接，必须在方法结束前关闭。
	defer rows.Close()

	friendIDs := make([]int64, 0)
	for rows.Next() {
		var friendID int64
		if err := rows.Scan(&friendID); err != nil {
			return nil, fmt.Errorf("scan friend id: %w", err)
		}
		friendIDs = append(friendIDs, friendID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate friend ids: %w", err)
	}
	return friendIDs, nil
}

// find 读取一条规范化好友边，供 Create 获取数据库生成字段。
func (r *MySQLFriendshipRepository) find(
	ctx context.Context,
	userIDLow, userIDHigh int64,
) (Friendship, error) {
	const query = `
		SELECT user_id_low, user_id_high, created_at
		FROM friendships
		WHERE user_id_low = ? AND user_id_high = ?
	`
	var friendship Friendship
	if err := r.executor.QueryRowContext(ctx, query, userIDLow, userIDHigh).Scan(
		&friendship.UserIDLow,
		&friendship.UserIDHigh,
		&friendship.CreatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Friendship{}, ErrFriendshipNotFound
		}
		return Friendship{}, fmt.Errorf("find friendship: %w", err)
	}
	return friendship, nil
}
