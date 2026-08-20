package settlement

import "errors"

var (
	// ErrInvalidCommand 表示结算命令缺少房间、双方玩家或合法拳型。
	ErrInvalidCommand = errors.New("invalid settlement command")
	// ErrInvalidGeneratedPoints 表示积分生成器返回值不在业务约定的 [11, 19]。
	ErrInvalidGeneratedPoints = errors.New("generated settlement points are outside configured range")
)
