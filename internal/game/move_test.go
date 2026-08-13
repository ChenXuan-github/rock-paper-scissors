package game

import "testing"

func TestMoveString(t *testing.T) {
	tests := []struct {
		move Move
		want string
	}{
		{move: Rock, want: "rock"},
		{move: Scissors, want: "scissors"},
		{move: Paper, want: "paper"},
		{move: Move(-1), want: "unknown"},
	}

	for _, test := range tests {
		if got := test.move.String(); got != test.want {
			t.Errorf("Move(%d).String() = %q, want %q", test.move, got, test.want)
		}
	}
}

func TestParseMoveRock(t *testing.T) {
	move, err := ParseMove("rock")

	if err != nil {
		t.Fatalf("ParseMove(%q) returned error: %v", "rock", err)
	}

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
	_, err := ParseMove("invalid")

	if err == nil {
		t.Fatal("ParseMove(\"invalid\") returned nil error, want an error")
	}
}
