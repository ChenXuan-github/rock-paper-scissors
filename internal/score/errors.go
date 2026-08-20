package score

import "errors"

var (
	// ErrInvalidUserID 表示积分操作收到的用户 ID 不是有效数据库主键。
	ErrInvalidUserID = errors.New("invalid score user id")
	// ErrInvalidResult 表示待入库结果不是胜、负、平中的任何一种。
	ErrInvalidResult = errors.New("invalid score result")
	// ErrInvalidPointValue 表示胜负积分不在 [11, 19]，或平局错误地携带了积分变化。
	ErrInvalidPointValue = errors.New("invalid score point value")
	// ErrScoreNotFound 表示指定用户尚未建立 player_scores 汇总行。
	ErrScoreNotFound = errors.New("player score not found")
	// ErrScoreAlreadyExist 表示该用户的积分汇总已经存在，通常由唯一索引冲突产生。
	ErrScoreAlreadyExist = errors.New("player score already exists")
)
