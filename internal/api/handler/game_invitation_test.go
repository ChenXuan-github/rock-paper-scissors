package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/api/middleware"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/social"
	"github.com/gin-gonic/gin"
)

type gameInvitationHandlerService struct {
	invitation social.GameInvitation
}

func (s *gameInvitationHandlerService) Invite(
	_ context.Context,
	inviterID, inviteeID int64,
) (social.GameInvitation, error) {
	s.invitation = social.GameInvitation{
		ID:        "invitation-id",
		InviterID: inviterID,
		InviteeID: inviteeID,
		Status:    social.GameInvitationPending,
	}
	return s.invitation, nil
}
func (s *gameInvitationHandlerService) Accept(
	context.Context,
	string,
	int64,
) (social.GameInvitation, error) {
	return social.GameInvitation{}, nil
}
func (s *gameInvitationHandlerService) Reject(string, int64) (social.GameInvitation, error) {
	return social.GameInvitation{}, nil
}
func (s *gameInvitationHandlerService) Cancel(string, int64) (social.GameInvitation, error) {
	return social.GameInvitation{}, nil
}

func TestGameInvitationHandlerInviteUsesJWTInviter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &gameInvitationHandlerService{}
	handler := NewGameInvitationHandler(service)
	router := gin.New()
	router.POST(
		"/game-invitations",
		middleware.Authenticate(roomTestTokenVerifier{userID: 5, username: "inviter"}),
		handler.Invite,
	)

	request := httptest.NewRequest(
		http.MethodPost,
		"/game-invitations",
		strings.NewReader(`{"inviteeId":8}`),
	)
	request.Header.Set("Authorization", "Bearer test-token")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("HTTP status = %d, want %d; body = %s", recorder.Code, http.StatusCreated, recorder.Body.String())
	}
	if service.invitation.InviterID != 5 || service.invitation.InviteeID != 8 {
		t.Fatalf("invitation = %+v, want JWT user 5 -> user 8", service.invitation)
	}
}
