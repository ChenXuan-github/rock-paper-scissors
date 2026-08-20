package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/api/response"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/leaderboard"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/record"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/score"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/user"
	"github.com/gin-gonic/gin"
)

type competitionScoreService struct {
	scores     map[int64]score.PlayerScore
	batchCalls int
}

func (s *competitionScoreService) GetOrCreate(context.Context, int64) (score.PlayerScore, error) {
	return score.PlayerScore{}, nil
}

func (s *competitionScoreService) GetByUserIDs(
	_ context.Context,
	_ []int64,
) (map[int64]score.PlayerScore, error) {
	s.batchCalls++
	return s.scores, nil
}

type competitionRecordService struct{}

func (competitionRecordService) ListByUserID(
	context.Context,
	int64,
	int,
	int,
) ([]record.GameRecord, error) {
	return nil, nil
}

type competitionLeaderboardService struct {
	entries []leaderboard.Entry
}

func (s competitionLeaderboardService) UpdateScores(context.Context, ...score.PlayerScore) error {
	return nil
}

func (s competitionLeaderboardService) Top(context.Context, int) ([]leaderboard.Entry, error) {
	return s.entries, nil
}

func (competitionLeaderboardService) Rank(context.Context, int64) (*int, error) {
	return nil, nil
}

type competitionUserService struct {
	users      map[int64]user.User
	batchCalls int
}

func (s *competitionUserService) GetByIDs(
	_ context.Context,
	_ []int64,
) (map[int64]user.User, error) {
	s.batchCalls++
	return s.users, nil
}

func TestCompetitionHandlerRankingUsesTwoBatchQueries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	scoreService := &competitionScoreService{scores: map[int64]score.PlayerScore{
		1: {UserID: 1, Score: 20, Wins: 2},
		2: {UserID: 2, Score: 30, Wins: 3},
	}}
	userService := &competitionUserService{users: map[int64]user.User{
		1: {ID: 1, Username: "first"},
		2: {ID: 2, Username: "second"},
	}}
	handler := NewCompetitionHandler(
		scoreService,
		competitionRecordService{},
		competitionLeaderboardService{entries: []leaderboard.Entry{
			{Rank: 1, UserID: 2, Score: 30},
			{Rank: 2, UserID: 1, Score: 20},
		}},
		userService,
	)
	router := gin.New()
	router.GET("/rankings", handler.Ranking)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/rankings", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if userService.batchCalls != 1 || scoreService.batchCalls != 1 {
		t.Fatalf(
			"batch calls: users=%d scores=%d, want one query for each table",
			userService.batchCalls,
			scoreService.batchCalls,
		)
	}

	var body response.Response[[]rankingEntryResponse]
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 2 || body.Data[0].UserID != 2 || body.Data[1].UserID != 1 {
		t.Fatalf("ranking response = %#v, want Redis rank order [2, 1]", body.Data)
	}
}
