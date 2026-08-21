package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/api/middleware"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/realtime"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/score"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/social"
	"github.com/gin-gonic/gin"
)

type socialHandlerService struct {
	sent    social.FriendRequest
	friends []social.UserSummary
}

func (s *socialHandlerService) SendFriendRequest(
	_ context.Context,
	requesterID, receiverID int64,
) (social.FriendRequest, error) {
	s.sent = social.FriendRequest{
		ID:          15,
		RequesterID: requesterID,
		ReceiverID:  receiverID,
		Status:      social.FriendRequestPending,
	}
	return s.sent, nil
}
func (s *socialHandlerService) AcceptFriendRequest(
	context.Context,
	int64,
	int64,
) (social.AcceptFriendRequestResult, error) {
	return social.AcceptFriendRequestResult{}, nil
}
func (s *socialHandlerService) RejectFriendRequest(
	context.Context,
	int64,
	int64,
) (social.FriendRequest, error) {
	return social.FriendRequest{}, nil
}
func (s *socialHandlerService) CancelFriendRequest(
	context.Context,
	int64,
	int64,
) (social.FriendRequest, error) {
	return social.FriendRequest{}, nil
}
func (s *socialHandlerService) RemoveFriend(context.Context, int64, int64) error {
	return nil
}
func (s *socialHandlerService) ListFriends(context.Context, int64) ([]social.UserSummary, error) {
	return s.friends, nil
}

type socialHandlerScores struct {
	scores map[int64]score.PlayerScore
	calls  int
}

func (s *socialHandlerScores) GetByUserIDs(
	_ context.Context,
	_ []int64,
) (map[int64]score.PlayerScore, error) {
	s.calls++
	return s.scores, nil
}

type socialHandlerRealtime struct {
	events map[int64][]realtime.Event
	online map[int64]bool
}

func (r *socialHandlerRealtime) SendToUser(userID int64, event realtime.Event) error {
	r.events[userID] = append(r.events[userID], event)
	return nil
}

func (r *socialHandlerRealtime) OnlineUsers(userIDs []int64) map[int64]bool {
	result := make(map[int64]bool, len(userIDs))
	for _, userID := range userIDs {
		result[userID] = r.online[userID]
	}
	return result
}
func (s *socialHandlerService) ListIncomingFriendRequests(
	context.Context,
	int64,
	social.FriendRequestStatus,
	int,
	int,
) ([]social.FriendRequestDetail, error) {
	return nil, nil
}
func (s *socialHandlerService) ListOutgoingFriendRequests(
	context.Context,
	int64,
	social.FriendRequestStatus,
	int,
	int,
) ([]social.FriendRequestDetail, error) {
	return nil, nil
}

func TestSocialHandlerSendUsesJWTUserAndPushesReceiver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &socialHandlerService{}
	realtimeGateway := &socialHandlerRealtime{events: make(map[int64][]realtime.Event)}
	socialHandler := NewSocialHandler(service, nil, realtimeGateway)
	router := gin.New()
	router.POST(
		"/friend-requests",
		middleware.Authenticate(roomTestTokenVerifier{userID: 7, username: "sender"}),
		socialHandler.SendFriendRequest,
	)

	request := httptest.NewRequest(
		http.MethodPost,
		"/friend-requests",
		strings.NewReader(`{"receiverId":9}`),
	)
	request.Header.Set("Authorization", "Bearer test-token")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("HTTP status = %d, want %d; body = %s", recorder.Code, http.StatusCreated, recorder.Body.String())
	}
	if service.sent.RequesterID != 7 || service.sent.ReceiverID != 9 {
		t.Fatalf("sent request = %+v, want JWT user 7 -> user 9", service.sent)
	}
	events := realtimeGateway.events[9]
	if len(events) != 1 || events[0].Type != "friend_request_received" {
		t.Fatalf("receiver events = %+v", events)
	}
}

func TestSocialHandlerListFriendsCombinesScoreAndOnlineState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &socialHandlerService{friends: []social.UserSummary{
		{ID: 2, Username: "online-friend"},
		{ID: 3, Username: "offline-friend"},
	}}
	scores := &socialHandlerScores{scores: map[int64]score.PlayerScore{
		2: {UserID: 2, Score: 120},
		3: {UserID: 3, Score: -5},
	}}
	realtimeGateway := &socialHandlerRealtime{
		events: make(map[int64][]realtime.Event),
		online: map[int64]bool{2: true},
	}
	socialHandler := NewSocialHandler(service, scores, realtimeGateway)
	router := gin.New()
	router.GET(
		"/friends",
		middleware.Authenticate(roomTestTokenVerifier{userID: 1, username: "current"}),
		socialHandler.ListFriends,
	)

	request := httptest.NewRequest(http.MethodGet, "/friends", nil)
	request.Header.Set("Authorization", "Bearer test-token")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if scores.calls != 1 {
		t.Fatalf("score batch query calls = %d, want 1", scores.calls)
	}
	body := recorder.Body.String()
	for _, expected := range []string{
		`"username":"online-friend"`,
		`"score":120`,
		`"online":true`,
		`"username":"offline-friend"`,
		`"score":-5`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("response %s does not contain %s", body, expected)
		}
	}
}
