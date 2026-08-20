package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/api/middleware"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/api/response"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/matchmaking"
	"github.com/gin-gonic/gin"
)

type matchmakingHandlerService struct {
	state      matchmaking.State
	joinErr    error
	cancelErr  error
	currentErr error
}

func (s *matchmakingHandlerService) Join(context.Context, int64) (matchmaking.State, error) {
	return s.state, s.joinErr
}

func (s *matchmakingHandlerService) Cancel(context.Context, int64) error {
	return s.cancelErr
}

func (s *matchmakingHandlerService) Current(context.Context, int64) (matchmaking.State, error) {
	return s.state, s.currentErr
}

func TestMatchmakingHandlerJoinReturnsWaitingPosition(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &matchmakingHandlerService{state: matchmaking.State{
		Status:   matchmaking.StateWaiting,
		Position: 1,
	}}
	handler := NewMatchmakingHandler(service)
	router := gin.New()
	router.POST(
		"/matchmaking/me",
		middleware.Authenticate(roomTestTokenVerifier{userID: 7, username: "player"}),
		handler.Join,
	)

	request := httptest.NewRequest(http.MethodPost, "/matchmaking/me", nil)
	request.Header.Set("Authorization", "Bearer test-token")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var body response.Response[matchmakingStateResponse]
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Status != matchmaking.StateWaiting || body.Data.Position == nil || *body.Data.Position != 1 {
		t.Fatalf("response = %#v, want waiting position 1", body.Data)
	}
}

func TestMatchmakingHandlerJoinRequiresWebSocket(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewMatchmakingHandler(&matchmakingHandlerService{joinErr: matchmaking.ErrPlayerOffline})
	router := gin.New()
	router.POST(
		"/matchmaking/me",
		middleware.Authenticate(roomTestTokenVerifier{}),
		handler.Join,
	)

	request := httptest.NewRequest(http.MethodPost, "/matchmaking/me", nil)
	request.Header.Set("Authorization", "Bearer test-token")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("HTTP status = %d, want %d; body = %s", recorder.Code, http.StatusConflict, recorder.Body.String())
	}
}

func TestMatchmakingHandlerCancelRejectsPlayerNotQueued(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewMatchmakingHandler(&matchmakingHandlerService{cancelErr: matchmaking.ErrNotQueued})
	router := gin.New()
	router.DELETE(
		"/matchmaking/me",
		middleware.Authenticate(roomTestTokenVerifier{}),
		handler.Cancel,
	)

	request := httptest.NewRequest(http.MethodDelete, "/matchmaking/me", nil)
	request.Header.Set("Authorization", "Bearer test-token")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("HTTP status = %d, want %d; body = %s", recorder.Code, http.StatusConflict, recorder.Body.String())
	}
}

func TestMatchmakingHandlerCurrentInternalError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewMatchmakingHandler(&matchmakingHandlerService{currentErr: errors.New("redis unavailable")})
	router := gin.New()
	router.GET(
		"/matchmaking/me",
		middleware.Authenticate(roomTestTokenVerifier{}),
		handler.Current,
	)

	request := httptest.NewRequest(http.MethodGet, "/matchmaking/me", nil)
	request.Header.Set("Authorization", "Bearer test-token")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("HTTP status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}
