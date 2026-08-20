package settlement

import (
	"testing"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/game"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/score"
)

func TestRandomPointGeneratorAlwaysUsesConfiguredInclusiveRange(t *testing.T) {
	generator := randomPointGenerator{}

	for range 1_000 {
		points := generator.Next()
		if points < score.MinWinPoints || points > score.MaxWinPoints {
			t.Fatalf("generated points = %d, want [%d, %d]", points, score.MinWinPoints, score.MaxWinPoints)
		}
	}
}

func TestScoreChangeUsesOppositeDeltas(t *testing.T) {
	const points = 16

	if got := scoreChange(game.Win, points); got != 16 {
		t.Fatalf("win change = %d, want 16", got)
	}
	if got := scoreChange(game.Lose, points); got != -16 {
		t.Fatalf("lose change = %d, want -16", got)
	}
	if got := scoreChange(game.Draw, 0); got != 0 {
		t.Fatalf("draw change = %d, want 0", got)
	}
}

func TestValidCommandRejectsInvalidPlayersAndMoves(t *testing.T) {
	valid := Command{
		RoomID:      "3A4JWL",
		Player1ID:   1,
		Player1Move: game.Rock,
		Player2ID:   2,
		Player2Move: game.Scissors,
	}
	if !validCommand(valid) {
		t.Fatal("valid command was rejected")
	}

	samePlayer := valid
	samePlayer.Player2ID = samePlayer.Player1ID
	if validCommand(samePlayer) {
		t.Fatal("same player command was accepted")
	}

	invalidMove := valid
	invalidMove.Player1Move = game.Move(99)
	if validCommand(invalidMove) {
		t.Fatal("invalid move command was accepted")
	}
}
