package game

import "fmt"

type Move int

const (
	Rock     Move = iota // 石头
	Scissors             // 剪刀
	Paper                // 布
)

// String 返回 Move 对应的字符串。
func (m Move) String() string {
	switch m {
	case Rock:
		return "rock"
	case Scissors:
		return "scissors"
	case Paper:
		return "paper"
	default:
		return "unknown"
	}
}

// ParseMove 把请求中的字符串转换成游戏内部使用的 Move。
func ParseMove(value string) (Move, error) {
	switch value {
	case "rock":
		return Rock, nil
	case "scissors":
		return Scissors, nil
	case "paper":
		return Paper, nil
	default:
		return 0, fmt.Errorf("invalid move: %q", value)
	}
}
