package game

import "testing"

func TestRoundStartsPending(t *testing.T) {
	round := Round{}

	if round.Result != ResultPending {
		t.Errorf("new round result = %v, want %v", round.Result, ResultPending)
	}
}

func TestRoundEvaluate(t *testing.T) {
	round := Round{
		PlayerMove:   Rock,
		OpponentMove: Scissors,
	}

	round.Evaluate()

	if round.Result != Win {
		t.Errorf("round.Result = %v, want %v", round.Result, Win)
	}
}
