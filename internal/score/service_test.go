package score

import (
	"context"
	"errors"
	"testing"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/game"
)

type memoryRepository struct {
	scores map[int64]PlayerScore
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{scores: make(map[int64]PlayerScore)}
}

func (r *memoryRepository) Create(_ context.Context, playerScore PlayerScore) (PlayerScore, error) {
	if _, exists := r.scores[playerScore.UserID]; exists {
		return PlayerScore{}, ErrScoreAlreadyExist
	}
	r.scores[playerScore.UserID] = playerScore
	return playerScore, nil
}

func (r *memoryRepository) FindByUserID(_ context.Context, userID int64) (PlayerScore, error) {
	playerScore, exists := r.scores[userID]
	if !exists {
		return PlayerScore{}, ErrScoreNotFound
	}
	return playerScore, nil
}

func (r *memoryRepository) FindByUserIDs(_ context.Context, userIDs []int64) ([]PlayerScore, error) {
	result := make([]PlayerScore, 0, len(userIDs))
	for _, userID := range userIDs {
		if playerScore, exists := r.scores[userID]; exists {
			result = append(result, playerScore)
		}
	}
	return result, nil
}

func (r *memoryRepository) ListAll(_ context.Context) ([]PlayerScore, error) {
	result := make([]PlayerScore, 0, len(r.scores))
	for _, playerScore := range r.scores {
		result = append(result, playerScore)
	}
	return result, nil
}

func (r *memoryRepository) ApplyChange(_ context.Context, userID int64, change Change) (PlayerScore, error) {
	playerScore, exists := r.scores[userID]
	if !exists {
		return PlayerScore{}, ErrScoreNotFound
	}
	playerScore.Score += change.ScoreDelta
	playerScore.Wins += change.Wins
	playerScore.Losses += change.Losses
	playerScore.Draws += change.Draws
	r.scores[userID] = playerScore
	return playerScore, nil
}

func TestCreateForUserCreatesZeroScore(t *testing.T) {
	service := NewService(newMemoryRepository())

	created, err := service.CreateForUser(context.Background(), 7)
	if err != nil {
		t.Fatalf("CreateForUser() error = %v", err)
	}
	if created.UserID != 7 || created.Score != 0 || created.Wins != 0 || created.Losses != 0 || created.Draws != 0 {
		t.Fatalf("created score = %+v", created)
	}
}

func TestCreateForUserRejectsInvalidUserID(t *testing.T) {
	service := NewService(newMemoryRepository())

	_, err := service.CreateForUser(context.Background(), 0)
	if !errors.Is(err, ErrInvalidUserID) {
		t.Fatalf("error = %v, want ErrInvalidUserID", err)
	}
}

func TestGetByUserIDReturnsStoredScore(t *testing.T) {
	repository := newMemoryRepository()
	repository.scores[9] = PlayerScore{UserID: 9, Score: 12, Wins: 14, Losses: 2, Draws: 1}
	service := NewService(repository)

	found, err := service.GetByUserID(context.Background(), 9)
	if err != nil {
		t.Fatalf("GetByUserID() error = %v", err)
	}
	if found.Score != 12 || found.Wins != 14 || found.Losses != 2 || found.Draws != 1 {
		t.Fatalf("found score = %+v", found)
	}
}

func TestGetByUserIDsReturnsScoresIndexedByUserID(t *testing.T) {
	repository := newMemoryRepository()
	repository.scores[3] = PlayerScore{UserID: 3, Score: 18}
	repository.scores[8] = PlayerScore{UserID: 8, Score: -4}
	service := NewService(repository)

	scoresByID, err := service.GetByUserIDs(context.Background(), []int64{8, 3})
	if err != nil {
		t.Fatal(err)
	}
	if scoresByID[3].Score != 18 || scoresByID[8].Score != -4 {
		t.Fatalf("scoresByID = %#v", scoresByID)
	}
}

func TestApplyResultAddsPointsForWinner(t *testing.T) {
	repository := newMemoryRepository()
	repository.scores[1] = PlayerScore{UserID: 1}
	service := NewService(repository)

	updated, err := service.ApplyResult(context.Background(), 1, game.Win, 17)
	if err != nil {
		t.Fatalf("ApplyResult() error = %v", err)
	}
	if updated.Score != 17 || updated.Wins != 1 || updated.Losses != 0 || updated.Draws != 0 {
		t.Fatalf("winner score = %+v", updated)
	}
}

func TestApplyResultAllowsNegativeLoserScore(t *testing.T) {
	repository := newMemoryRepository()
	repository.scores[2] = PlayerScore{UserID: 2, Score: 3}
	service := NewService(repository)

	updated, err := service.ApplyResult(context.Background(), 2, game.Lose, 19)
	if err != nil {
		t.Fatalf("ApplyResult() error = %v", err)
	}
	if updated.Score != -16 || updated.Losses != 1 {
		t.Fatalf("loser score = %+v, want score -16 and one loss", updated)
	}
}

func TestApplyResultDrawDoesNotChangeScore(t *testing.T) {
	repository := newMemoryRepository()
	repository.scores[3] = PlayerScore{UserID: 3, Score: -8}
	service := NewService(repository)

	updated, err := service.ApplyResult(context.Background(), 3, game.Draw, 0)
	if err != nil {
		t.Fatalf("ApplyResult() error = %v", err)
	}
	if updated.Score != -8 || updated.Draws != 1 {
		t.Fatalf("draw score = %+v", updated)
	}
}

func TestApplyResultRejectsPointsOutsideConfiguredRange(t *testing.T) {
	repository := newMemoryRepository()
	repository.scores[4] = PlayerScore{UserID: 4}
	service := NewService(repository)

	for _, points := range []int{10, 20} {
		_, err := service.ApplyResult(context.Background(), 4, game.Win, points)
		if !errors.Is(err, ErrInvalidPointValue) {
			t.Fatalf("points %d error = %v, want ErrInvalidPointValue", points, err)
		}
	}
}

func TestApplyResultRejectsPendingResult(t *testing.T) {
	repository := newMemoryRepository()
	repository.scores[5] = PlayerScore{UserID: 5}
	service := NewService(repository)

	_, err := service.ApplyResult(context.Background(), 5, game.ResultPending, 0)
	if !errors.Is(err, ErrInvalidResult) {
		t.Fatalf("error = %v, want ErrInvalidResult", err)
	}
}
