package social

import (
	"errors"
	"testing"
	"time"
)

func TestGameInvitationManagerRejectsReverseDuplicate(t *testing.T) {
	manager := NewGameInvitationManager()
	if _, err := manager.Create(
		UserSummary{ID: 1, Username: "one"},
		UserSummary{ID: 2, Username: "two"},
	); err != nil {
		t.Fatal(err)
	}

	_, err := manager.Create(
		UserSummary{ID: 2, Username: "two"},
		UserSummary{ID: 1, Username: "one"},
	)
	if !errors.Is(err, ErrGameInvitationPending) {
		t.Fatalf("reverse duplicate error = %v, want ErrGameInvitationPending", err)
	}
}

func TestGameInvitationManagerOnlyInviteeCanAccept(t *testing.T) {
	manager := NewGameInvitationManager()
	created, err := manager.Create(UserSummary{ID: 1}, UserSummary{ID: 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Accept(created.ID, 1, "ABC123"); !errors.Is(err, ErrGameInvitationNotInvitee) {
		t.Fatalf("inviter accept error = %v, want ErrGameInvitationNotInvitee", err)
	}
	accepted, err := manager.Accept(created.ID, 2, "ABC123")
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Status != GameInvitationAccepted || accepted.RoomID != "ABC123" {
		t.Fatalf("accepted invitation = %+v", accepted)
	}
}

func TestGameInvitationManagerExpiresAndAllowsNewInvitation(t *testing.T) {
	manager := newGameInvitationManager(time.Millisecond)
	created, err := manager.Create(UserSummary{ID: 1}, UserSummary{ID: 2})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)

	found, err := manager.FindByID(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if found.Status != GameInvitationExpired {
		t.Fatalf("status = %q, want expired", found.Status)
	}
	if _, err := manager.Create(UserSummary{ID: 2}, UserSummary{ID: 1}); err != nil {
		t.Fatalf("new invitation after expiration: %v", err)
	}
}
