package record

import "errors"

var (
	// ErrRecordNotFound 表示按主键查询时没有找到对应对局记录。
	ErrRecordNotFound = errors.New("game record not found")
	// ErrInvalidRecord 表示房间、玩家、拳型或胜者之间存在不合法关系。
	ErrInvalidRecord = errors.New("invalid game record")
	// ErrInvalidPage 表示历史记录分页参数超出业务允许范围。
	ErrInvalidPage = errors.New("invalid game record page")
)
