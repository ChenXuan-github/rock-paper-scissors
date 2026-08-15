package game

import "testing"

func TestResultString(t *testing.T) {
	// 同时覆盖四个领域常量和未知整数的兜底输出。
	tests := []struct {
		result Result
		want   string
	}{
		{result: ResultPending, want: "pending"},
		{result: Draw, want: "draw"},
		{result: Win, want: "win"},
		{result: Lose, want: "lose"},
		{result: Result(-1), want: "unknown"},
	}

	// 表驱动方式避免为每个 Result 重复编写一个测试函数。
	for _, test := range tests {
		if got := test.result.String(); got != test.want {
			t.Errorf("Result(%d).String() = %q, want %q", test.result, got, test.want)
		}
	}
}
