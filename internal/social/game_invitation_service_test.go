package social

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/game"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/realtime"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/user"
)

type gameInvitationTestRealtime struct {
	online map[int64]bool
	events map[int64][]realtime.Event
}

func (r *gameInvitationTestRealtime) IsOnline(userID int64) bool {
	return r.online[userID]
}

func (r *gameInvitationTestRealtime) SendToUser(userID int64, event realtime.Event) error {
	if !r.online[userID] {
		return realtime.ErrUserOffline
	}
	r.events[userID] = append(r.events[userID], event)
	return nil
}

func newGameInvitationTestService() (
	*GameInvitationService,
	*game.RoomService,
	*gameInvitationTestRealtime,
) {
	users := &socialTestUsers{users: map[int64]user.User{
		1: {ID: 1, Username: "inviter"},
		2: {ID: 2, Username: "invitee"},
	}}
	rooms := game.NewRoomService(game.NewRoomManager())
	realtimeGateway := &gameInvitationTestRealtime{
		online: map[int64]bool{1: true, 2: true},
		events: make(map[int64][]realtime.Event),
	}
	service := NewGameInvitationService(
		NewGameInvitationManager(),
		&socialTestFriendships{exists: true},
		users,
		rooms,
		realtimeGateway,
	)
	return service, rooms, realtimeGateway
}

func TestGameInvitationServiceInvitePushesOnlineFriend(t *testing.T) {
	service, _, realtimeGateway := newGameInvitationTestService()

	invitation, err := service.Invite(context.Background(), 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if invitation.Status != GameInvitationPending {
		t.Fatalf("status = %q, want pending", invitation.Status)
	}
	events := realtimeGateway.events[2]
	if len(events) != 1 || events[0].Type != "game_invitation_received" {
		t.Fatalf("invitee events = %+v", events)
	}

	// WebSocket 事件直接由 encoding/json 序列化；字段名必须与前端的
	// invitation.inviter.username 保持一致，不能退化成 Go 默认的 Username。
	payload, err := json.Marshal(events[0])
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(payload)
	if !strings.Contains(serialized, `"inviter":{"id":1,"username":"inviter"}`) {
		t.Fatalf("serialized event = %s, want lower-camel inviter fields", serialized)
	}
}

func TestGameInvitationServiceAcceptCreatesRoomForBothPlayers(t *testing.T) {
	service, rooms, realtimeGateway := newGameInvitationTestService()
	invitation, err := service.Invite(context.Background(), 1, 2)
	if err != nil {
		t.Fatal(err)
	}

	accepted, err := service.Accept(context.Background(), invitation.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Status != GameInvitationAccepted || accepted.RoomID == "" {
		t.Fatalf("accepted invitation = %+v", accepted)
	}
	inviterRoom, err := rooms.GetCurrentRoom(1)
	if err != nil {
		t.Fatal(err)
	}
	inviteeRoom, err := rooms.GetCurrentRoom(2)
	if err != nil {
		t.Fatal(err)
	}
	if inviterRoom.Room.ID != accepted.RoomID || inviteeRoom.Room.ID != accepted.RoomID {
		t.Fatalf("room IDs = %q and %q, want %q", inviterRoom.Room.ID, inviteeRoom.Room.ID, accepted.RoomID)
	}
	events := realtimeGateway.events[1]
	if len(events) != 1 || events[0].Type != "game_invitation_accepted" {
		t.Fatalf("inviter events = %+v", events)
	}
}
