package game

// Result 是从玩家视角描述的一局结果。
// 它的底层类型是 int，但业务代码应使用命名常量而不是魔法数字。
type Result int

const (
	// ResultPending 使用零值，表示 Round 已创建但还没有执行 Evaluate。
	ResultPending Result = iota
	Draw                 // 1：平局
	Win                  // 2：玩家获胜
	Lose                 // 3：玩家失败
)

// String 把内部 Result 转成稳定的 API 字符串。
func (r Result) String() string {
	switch r {
	case ResultPending:
		return "pending"
	case Draw:
		return "draw"
	case Win:
		return "win"
	case Lose:
		return "lose"
	default:
		// 防御非法 Result 值，避免向外返回没有定义的数字。
		return "unknown"
	}
}
