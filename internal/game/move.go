package game

import "fmt"

// Move 是对“石头、剪刀、布”的领域建模。
// Go 没有 enum 关键字，这里使用自定义整数类型配合常量模拟枚举。
type Move int

const (
	// iota 在当前 const 块中从 0 开始逐行递增。
	// 这个顺序不能随意修改：rule.go 的取模算法依赖 Rock(0)、Scissors(1)、Paper(2)。
	Rock     Move = iota // 0：石头
	Scissors             // 1：剪刀
	Paper                // 2：布
)

// String 把内部 Move 转成适合 HTTP JSON 展示的字符串。
// 因为接收者是 Move，所以可以直接调用 Rock.String() 或 move.String()。
func (m Move) String() string {
	// 显式列出每个合法值，避免把内部整数直接暴露给客户端。
	switch m {
	case Rock:
		return "rock"
	case Scissors:
		return "scissors"
	case Paper:
		return "paper"
	default:
		// 非法整数仍然可以被强制转换成 Move，因此需要兜底分支。
		return "unknown"
	}
}

// ParseMove 把请求中的字符串转换成游戏内部使用的 Move。
func ParseMove(value string) (Move, error) {
	// String 与 ParseMove 互为正反向转换：一个负责输出，一个负责解析输入。
	switch value {
	case "rock":
		// 第二个返回值为 nil，表示转换成功、没有错误。
		return Rock, nil
	case "scissors":
		return Scissors, nil
	case "paper":
		return Paper, nil
	default:
		// Move 的零值恰好是 Rock，所以调用方必须检查 error，不能只看第一个返回值。
		// %q 会给非法输入加引号，便于日志或错误响应识别空字符串等情况。
		return 0, fmt.Errorf("invalid move: %q", value)
	}
}
