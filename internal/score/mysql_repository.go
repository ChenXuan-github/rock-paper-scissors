package score

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/database"
	mysqlDriver "github.com/go-sql-driver/mysql"
)

// MySQLRepository 使用 player_scores 表持久化积分汇总。
type MySQLRepository struct {
	executor database.Executor
}

// NewMySQLRepository 注入 SQL 执行器。
// 普通调用传 *sql.DB；事务调用传同一个 *sql.Tx。
func NewMySQLRepository(executor database.Executor) *MySQLRepository {
	return &MySQLRepository{executor: executor}
}

// Create 为一名用户创建零积分记录。
func (r *MySQLRepository) Create(ctx context.Context, playerScore PlayerScore) (PlayerScore, error) {
	const query = `
		INSERT INTO player_scores (user_id, score, wins, losses, draws)
		VALUES (?, ?, ?, ?, ?)
	`

	_, err := r.executor.ExecContext(
		ctx,
		query,
		playerScore.UserID,
		playerScore.Score,
		playerScore.Wins,
		playerScore.Losses,
		playerScore.Draws,
	)
	if err != nil {
		var mysqlErr *mysqlDriver.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return PlayerScore{}, ErrScoreAlreadyExist
		}
		return PlayerScore{}, fmt.Errorf("create player score: %w", err)
	}

	// 重新查询一次，取得由数据库生成的 created_at 和 updated_at。
	return r.FindByUserID(ctx, playerScore.UserID)
}

// FindByUserID 查询指定用户最新的积分汇总。
func (r *MySQLRepository) FindByUserID(ctx context.Context, userID int64) (PlayerScore, error) {
	const query = `
		SELECT user_id, score, wins, losses, draws, created_at, updated_at
		FROM player_scores
		WHERE user_id = ?
	`

	var playerScore PlayerScore
	err := r.executor.QueryRowContext(ctx, query, userID).Scan(
		&playerScore.UserID,
		&playerScore.Score,
		&playerScore.Wins,
		&playerScore.Losses,
		&playerScore.Draws,
		&playerScore.CreatedAt,
		&playerScore.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PlayerScore{}, ErrScoreNotFound
		}
		return PlayerScore{}, fmt.Errorf("find player score by user id: %w", err)
	}

	return playerScore, nil
}

// FindByUserIDs 使用一条 IN 查询批量读取积分，避免排行榜逐人访问 player_scores。
func (r *MySQLRepository) FindByUserIDs(ctx context.Context, userIDs []int64) ([]PlayerScore, error) {
	if len(userIDs) == 0 {
		return []PlayerScore{}, nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(userIDs)), ",")
	query := fmt.Sprintf(`
		SELECT user_id, score, wins, losses, draws, created_at, updated_at
		FROM player_scores
		WHERE user_id IN (%s)
	`, placeholders)
	args := make([]any, len(userIDs))
	for index, userID := range userIDs {
		args[index] = userID
	}

	rows, err := r.executor.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("find player scores by user ids: %w", err)
	}
	defer rows.Close()

	playerScores := make([]PlayerScore, 0, len(userIDs))
	for rows.Next() {
		var playerScore PlayerScore
		if err := rows.Scan(
			&playerScore.UserID,
			&playerScore.Score,
			&playerScore.Wins,
			&playerScore.Losses,
			&playerScore.Draws,
			&playerScore.CreatedAt,
			&playerScore.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan player scores by user ids: %w", err)
		}
		playerScores = append(playerScores, playerScore)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate player scores by user ids: %w", err)
	}
	return playerScores, nil
}

// ListAll 读取 MySQL 中全部积分，用于服务启动时重建 Redis 排行榜索引。
func (r *MySQLRepository) ListAll(ctx context.Context) ([]PlayerScore, error) {
	const query = `
		SELECT user_id, score, wins, losses, draws, created_at, updated_at
		FROM player_scores
		ORDER BY user_id
	`

	rows, err := r.executor.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list all player scores: %w", err)
	}
	defer rows.Close()

	result := make([]PlayerScore, 0)
	for rows.Next() {
		var playerScore PlayerScore
		if err := rows.Scan(
			&playerScore.UserID,
			&playerScore.Score,
			&playerScore.Wins,
			&playerScore.Losses,
			&playerScore.Draws,
			&playerScore.CreatedAt,
			&playerScore.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan player score list: %w", err)
		}
		result = append(result, playerScore)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate player score list: %w", err)
	}
	return result, nil
}

// ApplyChange 使用一条 UPDATE 同时修改积分和胜负统计。
// score = score + ? 直接在数据库中做原子增量，允许最终积分小于零。
func (r *MySQLRepository) ApplyChange(ctx context.Context, userID int64, change Change) (PlayerScore, error) {
	const query = `
		UPDATE player_scores
		SET
			score = score + ?,
			wins = wins + ?,
			losses = losses + ?,
			draws = draws + ?
		WHERE user_id = ?
	`

	result, err := r.executor.ExecContext(
		ctx,
		query,
		change.ScoreDelta,
		change.Wins,
		change.Losses,
		change.Draws,
		userID,
	)
	if err != nil {
		return PlayerScore{}, fmt.Errorf("apply player score change: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return PlayerScore{}, fmt.Errorf("get affected player score rows: %w", err)
	}
	if affected == 0 {
		return PlayerScore{}, ErrScoreNotFound
	}

	// 若 executor 是 *sql.Tx，这次查询仍在同一事务中，能读到尚未提交的更新结果。
	return r.FindByUserID(ctx, userID)
}
