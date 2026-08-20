package record

import (
	"time"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/game"
)

// GameRecord 表示一小局 1v1 对战的永久记录。
// 一局只保存一条，Player1 和 Player2 是记录中的两个参与方，不代表固定房主或提交顺序。
type GameRecord struct {
	ID                 int64
	RoomID             string
	Player1ID          int64
	Player1Move        game.Move
	Player2ID          int64
	Player2Move        game.Move
	WinnerID           *int64
	Player1ScoreChange int
	Player1ScoreAfter  int
	Player2ScoreChange int
	Player2ScoreAfter  int
	// Username 来自查询时关联 users 表，不重复存入 game_records。
	Player1Username string
	Player2Username string
	CreatedAt       time.Time
}
