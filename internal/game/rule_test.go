package game

import "testing"

func TestEvaluateRockBeatsScissors(t *testing.T) {
	result := Evaluate(Rock, Scissors)

	if result != Win {
		t.Errorf("Evaluate(Rock, Scissors) = %v, want %v", result, Win)
	}
}

func TestEvaluateRockDrawsRock(t *testing.T) {
	result := Evaluate(Rock, Rock)

	if result != Draw {
		t.Errorf("Evaluate(Rock, Rock) = %v, want %v", result, Draw)
	}
}

func TestEvaluateRockLosesToPaper(t *testing.T) {
	result := Evaluate(Rock, Paper)

	if result != Lose {
		t.Errorf("Evaluate(Rock, Paper) = %v, want %v", result, Lose)
	}
}

func TestEvaluateScissorsLosesToRock(t *testing.T) {
	result := Evaluate(Scissors, Rock)

	if result != Lose {
		t.Errorf("Evaluate(Scissors, Rock) = %v, want %v", result, Lose)
	}
}

func TestEvaluateScissorsDrawsScissors(t *testing.T) {
	result := Evaluate(Scissors, Scissors)

	if result != Draw {
		t.Errorf("Evaluate(Scissors, Scissors) = %v, want %v", result, Draw)
	}
}

func TestEvaluateScissorsBeatsPaper(t *testing.T) {
	result := Evaluate(Scissors, Paper)

	if result != Win {
		t.Errorf("Evaluate(Scissors, Paper) = %v, want %v", result, Win)
	}
}

func TestEvaluatePaperBeatsRock(t *testing.T) {
	result := Evaluate(Paper, Rock)

	if result != Win {
		t.Errorf("Evaluate(Paper, Rock) = %v, want %v", result, Win)
	}
}

func TestEvaluatePaperLosesToScissors(t *testing.T) {
	result := Evaluate(Paper, Scissors)

	if result != Lose {
		t.Errorf("Evaluate(Paper, Scissors) = %v, want %v", result, Lose)
	}
}

func TestEvaluatePaperDrawsPaper(t *testing.T) {
	result := Evaluate(Paper, Paper)

	if result != Draw {
		t.Errorf("Evaluate(Paper, Paper) = %v, want %v", result, Draw)
	}
}
