package score

import "time"

const (
	// MinWinPoints 和 MaxWinPoints 定义非平局对战的随机积分绝对值范围，左右边界都包含。
	MinWinPoints = 11
	MaxWinPoints = 19
)

// PlayerScore 是一名用户的永久积分汇总。
// 它与房间中的临时 Player 不同：PlayerScore 会持久化到 MySQL。
type PlayerScore struct {
	UserID    int64
	Score     int
	Wins      uint
	Losses    uint
	Draws     uint
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Change 是一次对局对单个玩家积分汇总造成的增量。
// 它只在 Repository 层表示 SQL 增量，不是数据库中的独立实体。
type Change struct {
	ScoreDelta int
	Wins       uint
	Losses     uint
	Draws      uint
}
