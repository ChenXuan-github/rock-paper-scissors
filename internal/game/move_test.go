package game

import "testing"

func TestMoveString(t *testing.T) {
	// 表驱动测试把相同验证逻辑应用到所有合法枚举值和一个非法兜底值。
	tests := []struct {
		move Move
		want string
	}{
		{move: Rock, want: "rock"},
		{move: Scissors, want: "scissors"},
		{move: Paper, want: "paper"},
		{move: Move(-1), want: "unknown"},
	}

	// 每个用例都调用同一个 String 方法，并与期望字符串比较。
	for _, test := range tests {
		if got := test.move.String(); got != test.want {
			t.Errorf("Move(%d).String() = %q, want %q", test.move, got, test.want)
		}
	}
}

func TestParseMoveRock(t *testing.T) {
	// Go 函数可以返回多个值，这里同时接收转换结果和错误。
	move, err := ParseMove("rock")

	// Fatalf 表示前置条件失败，立即停止当前测试，避免继续使用无效 move。
	if err != nil {
		t.Fatalf("ParseMove(%q) returned error: %v", "rock", err)
	}

	// Errorf 记录断言失败，但允许当前测试函数继续执行后续检查。
	if move != Rock {
		t.Errorf("ParseMove(%q) = %v, want %v", "rock", move, Rock)
	}
}

func TestParseMoveScissors(t *testing.T) {
	move, err := ParseMove("scissors")

	if err != nil {
		t.Fatalf("ParseMove(%q) returned error: %v", "scissors", err)
	}

	if move != Scissors {
		t.Errorf("ParseMove(%q) = %v, want %v", "scissors", move, Scissors)
	}
}

func TestParseMovePaper(t *testing.T) {
	move, err := ParseMove("paper")

	if err != nil {
		t.Fatalf("ParseMove(%q) returned error: %v", "paper", err)
	}

	if move != Paper {
		t.Errorf("ParseMove(%q) = %v, want %v", "paper", move, Paper)
	}
}

func TestParseMoveRejectsInvalidValue(t *testing.T) {
	// 下划线忽略不需要验证的 Move，只关心非法输入是否返回 error。
	_, err := ParseMove("invalid")

	if err == nil {
		t.Fatal("ParseMove(\"invalid\") returned nil error, want an error")
	}
}
