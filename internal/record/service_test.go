package record

import (
	"context"
	"errors"
	"testing"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/game"
)

type memoryRepository struct {
	records []GameRecord
}

func (r *memoryRepository) Create(_ context.Context, gameRecord GameRecord) (GameRecord, error) {
	gameRecord.ID = int64(len(r.records) + 1)
	r.records = append(r.records, gameRecord)
	return gameRecord, nil
}

func (r *memoryRepository) FindByID(_ context.Context, id int64) (GameRecord, error) {
	for _, gameRecord := range r.records {
		if gameRecord.ID == id {
			return gameRecord, nil
		}
	}
	return GameRecord{}, ErrRecordNotFound
}

func (r *memoryRepository) ListByUserID(_ context.Context, userID int64, limit, offset int) ([]GameRecord, error) {
	matched := make([]GameRecord, 0)
	for _, gameRecord := range r.records {
		if gameRecord.Player1ID == userID || gameRecord.Player2ID == userID {
			matched = append(matched, gameRecord)
		}
	}
	if offset >= len(matched) {
		return []GameRecord{}, nil
	}
	end := offset + limit
	if end > len(matched) {
		end = len(matched)
	}
	return matched[offset:end], nil
}

func TestCreateValidRecord(t *testing.T) {
	repository := &memoryRepository{}
	service := NewService(repository)
	winnerID := int64(1)

	created, err := service.Create(context.Background(), GameRecord{
		RoomID:             " 3A4JWL ",
		Player1ID:          1,
		Player1Move:        game.Rock,
		Player2ID:          2,
		Player2Move:        game.Scissors,
		WinnerID:           &winnerID,
		Player1ScoreChange: 1,
		Player2ScoreChange: -1,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.ID != 1 || created.RoomID != "3A4JWL" {
		t.Fatalf("created record = %+v", created)
	}
}

func TestCreateRejectsSamePlayer(t *testing.T) {
	service := NewService(&memoryRepository{})

	_, err := service.Create(context.Background(), GameRecord{
		RoomID:      "3A4JWL",
		Player1ID:   1,
		Player1Move: game.Rock,
		Player2ID:   1,
		Player2Move: game.Paper,
	})
	if !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("error = %v, want ErrInvalidRecord", err)
	}
}

func TestCreateRejectsWinnerOutsidePlayers(t *testing.T) {
	service := NewService(&memoryRepository{})
	winnerID := int64(99)

	_, err := service.Create(context.Background(), GameRecord{
		RoomID:      "3A4JWL",
		Player1ID:   1,
		Player1Move: game.Rock,
		Player2ID:   2,
		Player2Move: game.Paper,
		WinnerID:    &winnerID,
	})
	if !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("error = %v, want ErrInvalidRecord", err)
	}
}

func TestListByUserIDRejectsInvalidPage(t *testing.T) {
	service := NewService(&memoryRepository{})

	_, err := service.ListByUserID(context.Background(), 1, 0, 0)
	if !errors.Is(err, ErrInvalidPage) {
		t.Fatalf("error = %v, want ErrInvalidPage", err)
	}
}
