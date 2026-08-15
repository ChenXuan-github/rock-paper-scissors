package user

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	mysqlDriver "github.com/go-sql-driver/mysql"
)

// MySQLRepository 使用 MySQL 持久化用户数据。
type MySQLRepository struct {
	// *sql.DB 是并发安全的连接池句柄，不代表某一条固定连接。
	db *sql.DB
}

// NewMySQLRepository 创建用户数据仓库。
func NewMySQLRepository(db *sql.DB) *MySQLRepository {
	// 数据库依赖由 main 创建后注入，Repository 自己不读取配置或建立连接池。
	return &MySQLRepository{db: db}
}

// Create 将新用户写入数据库，并返回数据库生成的用户 ID。
func (r *MySQLRepository) Create(ctx context.Context, user User) (User, error) {
	// SQL 结构固定，用户输入通过 ? 占位符传递，避免字符串拼接导致 SQL 注入。
	const query = `
		INSERT INTO users (username, password_hash)
		VALUES (?, ?)
	`

	// ExecContext 适合 INSERT/UPDATE/DELETE，并能响应上游 Context 的取消信号。
	result, err := r.db.ExecContext(
		ctx,
		query,
		user.Username,
		user.PasswordHash,
	)
	if err != nil {
		// errors.As 尝试从错误链中取得 MySQL 驱动的具体错误。
		var mysqlErr *mysqlDriver.MySQLError
		// 1062 表示唯一索引冲突，这里转换成上层认识的领域错误。
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return User{}, ErrUsernameExists
		}
		return User{}, fmt.Errorf("create user: %w", err)
	}

	// INSERT 成功后读取 MySQL 为 AUTO_INCREMENT 主键生成的 ID。
	id, err := result.LastInsertId()
	if err != nil {
		return User{}, fmt.Errorf("get created user id: %w", err)
	}

	// 把数据库生成的 ID 写回 User，调用方无需再次查询即可获得新用户主键。
	user.ID = id
	return user, nil
}

// FindByID 根据用户 ID 查询用户。
func (r *MySQLRepository) FindByID(ctx context.Context, id int64) (User, error) {
	// SELECT 明确列出字段，避免表结构变化时 SELECT * 悄悄改变 Scan 顺序。
	const query = `
		SELECT id, username, password_hash, created_at
		FROM users
		WHERE id = ?
	`

	// 零值 User 用作当前这一行查询结果的接收对象。
	var user User
	// QueryRowContext 查询单行；Scan 的目标顺序必须与 SELECT 字段顺序一致。
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&user.ID,
		&user.Username,
		&user.PasswordHash,
		&user.CreatedAt,
	)
	if err != nil {
		// database/sql 使用 sql.ErrNoRows 表示查询成功执行但没有匹配记录。
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrUserNotFound
		}
		return User{}, fmt.Errorf("find user by id: %w", err)
	}

	return user, nil
}

// FindByUsername 根据用户名查询用户。
func (r *MySQLRepository) FindByUsername(ctx context.Context, username string) (User, error) {
	// 登录和注册查重都通过唯一用户名查询完整用户记录。
	const query = `
		SELECT id, username, password_hash, created_at
		FROM users
		WHERE username = ?
	`

	var user User
	// username 仍通过占位符绑定，而不是拼入 SQL 文本。
	err := r.db.QueryRowContext(ctx, query, username).Scan(
		&user.ID,
		&user.Username,
		&user.PasswordHash,
		&user.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrUserNotFound
		}
		return User{}, fmt.Errorf("find user by username: %w", err)
	}

	return user, nil
}
