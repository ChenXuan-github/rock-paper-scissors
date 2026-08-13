package game

type Result int

const (
	ResultPending Result = iota
	Draw
	Win
	Lose
)

// String 返回 Result 对应的字符串。
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
		return "unknown"
	}
}
