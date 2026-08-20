package record

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/database"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/game"
)

// MySQLRepository 使用 game_records 表持久化一局一条的对战记录。
type MySQLRepository struct {
	executor database.Executor
}

// NewMySQLRepository 接收普通连接池或事务，两者都实现 database.Executor。
func NewMySQLRepository(executor database.Executor) *MySQLRepository {
	return &MySQLRepository{executor: executor}
}

// Create 写入双方、拳型、胜者和积分变化。
func (r *MySQLRepository) Create(ctx context.Context, gameRecord GameRecord) (GameRecord, error) {
	const query = `
		INSERT INTO game_records (
			room_id,
			player1_id,
			player1_move,
			player2_id,
			player2_move,
			winner_id,
			player1_score_change, player1_score_after,
			player2_score_change, player2_score_after
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := r.executor.ExecContext(
		ctx,
		query,
		gameRecord.RoomID,
		gameRecord.Player1ID,
		gameRecord.Player1Move.String(),
		gameRecord.Player2ID,
		gameRecord.Player2Move.String(),
		gameRecord.WinnerID,
		gameRecord.Player1ScoreChange,
		gameRecord.Player1ScoreAfter,
		gameRecord.Player2ScoreChange,
		gameRecord.Player2ScoreAfter,
	)
	if err != nil {
		return GameRecord{}, fmt.Errorf("create game record: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return GameRecord{}, fmt.Errorf("get created game record id: %w", err)
	}
	return r.FindByID(ctx, id)
}

// FindByID 根据主键查询一局记录。
func (r *MySQLRepository) FindByID(ctx context.Context, id int64) (GameRecord, error) {
	const query = `
		SELECT
			r.id, r.room_id,
			r.player1_id, p1.username, r.player1_move,
			r.player2_id, p2.username, r.player2_move,
			r.winner_id,
			r.player1_score_change, r.player1_score_after,
			r.player2_score_change, r.player2_score_after,
			r.created_at
		FROM game_records r
		JOIN users p1 ON p1.id = r.player1_id
		JOIN users p2 ON p2.id = r.player2_id
		WHERE r.id = ?
	`

	gameRecord, err := scanGameRecord(r.executor.QueryRowContext(ctx, query, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return GameRecord{}, ErrRecordNotFound
		}
		return GameRecord{}, fmt.Errorf("find game record by id: %w", err)
	}
	return gameRecord, nil
}

// ListByUserID 查询某名玩家参与的历史对局，按时间从新到旧分页。
func (r *MySQLRepository) ListByUserID(ctx context.Context, userID int64, limit, offset int) ([]GameRecord, error) {
	const query = `
		SELECT
			r.id, r.room_id,
			r.player1_id, p1.username, r.player1_move,
			r.player2_id, p2.username, r.player2_move,
			r.winner_id,
			r.player1_score_change, r.player1_score_after,
			r.player2_score_change, r.player2_score_after,
			r.created_at
		FROM game_records r
		JOIN users p1 ON p1.id = r.player1_id
		JOIN users p2 ON p2.id = r.player2_id
		WHERE r.player1_id = ? OR r.player2_id = ?
		ORDER BY r.created_at DESC, r.id DESC
		LIMIT ? OFFSET ?
	`

	rows, err := r.executor.QueryContext(ctx, query, userID, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list game records by user id: %w", err)
	}
	defer rows.Close()

	records := make([]GameRecord, 0, limit)
	for rows.Next() {
		gameRecord, err := scanGameRecord(rows)
		if err != nil {
			return nil, fmt.Errorf("scan game record list: %w", err)
		}
		records = append(records, gameRecord)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate game record list: %w", err)
	}
	return records, nil
}

// rowScanner 同时兼容 *sql.Row 和 *sql.Rows，避免两种查询重复编写 Scan 逻辑。
type rowScanner interface {
	Scan(dest ...any) error
}

func scanGameRecord(row rowScanner) (GameRecord, error) {
	var (
		gameRecord  GameRecord
		player1Move string
		player2Move string
		winnerID    sql.NullInt64
	)

	err := row.Scan(
		&gameRecord.ID,
		&gameRecord.RoomID,
		&gameRecord.Player1ID,
		&gameRecord.Player1Username,
		&player1Move,
		&gameRecord.Player2ID,
		&gameRecord.Player2Username,
		&player2Move,
		&winnerID,
		&gameRecord.Player1ScoreChange,
		&gameRecord.Player1ScoreAfter,
		&gameRecord.Player2ScoreChange,
		&gameRecord.Player2ScoreAfter,
		&gameRecord.CreatedAt,
	)
	if err != nil {
		return GameRecord{}, err
	}

	parsedPlayer1Move, err := game.ParseMove(player1Move)
	if err != nil {
		return GameRecord{}, fmt.Errorf("parse stored player1 move: %w", err)
	}
	parsedPlayer2Move, err := game.ParseMove(player2Move)
	if err != nil {
		return GameRecord{}, fmt.Errorf("parse stored player2 move: %w", err)
	}
	gameRecord.Player1Move = parsedPlayer1Move
	gameRecord.Player2Move = parsedPlayer2Move

	if winnerID.Valid {
		value := winnerID.Int64
		gameRecord.WinnerID = &value
	}
	return gameRecord, nil
}
