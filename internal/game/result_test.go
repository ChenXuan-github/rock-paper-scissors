package game

import "testing"

func TestResultString(t *testing.T) {
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

	for _, test := range tests {
		if got := test.result.String(); got != test.want {
			t.Errorf("Result(%d).String() = %q, want %q", test.result, got, test.want)
		}
	}
}
