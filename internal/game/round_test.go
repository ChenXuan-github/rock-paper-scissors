package game

import "testing"

func TestRoundStartsPending(t *testing.T) {
	// 空结构体使用所有字段的零值，Result 的零值被设计为 ResultPending。
	round := Round{}

	if round.Result != ResultPending {
		t.Errorf("new round result = %v, want %v", round.Result, ResultPending)
	}
}

func TestRoundEvaluate(t *testing.T) {
	// struct literal 只给判定依据赋值，Result 初始保持 Pending。
	round := Round{
		PlayerMove:   Rock,
		OpponentMove: Scissors,
	}

	// 指针接收者方法会直接更新同一个 round 的 Result 字段。
	round.Evaluate()

	if round.Result != Win {
		t.Errorf("round.Result = %v, want %v", round.Result, Win)
	}
}
