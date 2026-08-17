package game

import (
	"errors"
	"testing"
)

func TestRoomServiceCreateRoomAddsHost(t *testing.T) {
	manager := NewRoomManager()
	service := NewRoomService(manager)
	host := &Player{UserID: 1, Username: "chenxuan"}

	room, err := service.CreateRoom(host)
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}

	storedRoom, exists := manager.GetRoom(room.ID)
	if !exists || storedRoom != room {
		t.Fatal("CreateRoom() did not store room in RoomManager")
	}
	players := room.GetPlayers()
	if len(players) != 1 || players[0] != host {
		t.Fatalf("CreateRoom() players = %#v, want only host", players)
	}
}

func TestRoomServiceCreateRoomRejectsNilHost(t *testing.T) {
	manager := NewRoomManager()
	service := NewRoomService(manager)

	if _, err := service.CreateRoom(nil); err != ErrPlayerRequired {
		t.Fatalf("CreateRoom(nil) error = %v, want %v", err, ErrPlayerRequired)
	}
	if rooms := manager.ListRooms(); len(rooms) != 0 {
		t.Fatalf("CreateRoom(nil) left %d rooms, want 0", len(rooms))
	}
}

func TestRoomServiceListRooms(t *testing.T) {
	manager := NewRoomManager()
	service := NewRoomService(manager)

	if _, err := service.CreateRoom(&Player{UserID: 1, Username: "player-1"}); err != nil {
		t.Fatalf("first CreateRoom() error = %v", err)
	}
	if _, err := service.CreateRoom(&Player{UserID: 2, Username: "player-2"}); err != nil {
		t.Fatalf("second CreateRoom() error = %v", err)
	}

	if rooms := service.ListRooms(); len(rooms) != 2 {
		t.Fatalf("ListRooms() count = %d, want 2", len(rooms))
	}
}

func TestRoomServiceRejectsPlayerCreatingMultipleRooms(t *testing.T) {
	manager := NewRoomManager()
	service := NewRoomService(manager)
	host := &Player{UserID: 1, Username: "chenxuan"}

	firstRoom, err := service.CreateRoom(host)
	if err != nil {
		t.Fatalf("first CreateRoom() error = %v", err)
	}

	secondRoom, err := service.CreateRoom(host)
	if !errors.Is(err, ErrPlayerAlreadyInAnotherRoom) {
		t.Fatalf("second CreateRoom() error = %v, want %v", err, ErrPlayerAlreadyInAnotherRoom)
	}
	if secondRoom != nil {
		t.Fatalf("second CreateRoom() room = %#v, want nil", secondRoom)
	}
	if rooms := manager.ListRooms(); len(rooms) != 1 || rooms[0] != firstRoom {
		t.Fatalf("rooms after duplicate create = %#v, want only first room", rooms)
	}
}

func TestRoomServiceLeaveCurrentRoomDeletesEmptyRoom(t *testing.T) {
	manager := NewRoomManager()
	service := NewRoomService(manager)
	host := &Player{UserID: 1, Username: "chenxuan"}

	room, err := service.CreateRoom(host)
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}

	leftRoomID, roomDeleted, err := service.LeaveCurrentRoom(host.UserID)
	if err != nil {
		t.Fatalf("LeaveCurrentRoom() error = %v", err)
	}
	if leftRoomID != room.ID {
		t.Fatalf("LeaveCurrentRoom() room ID = %q, want %q", leftRoomID, room.ID)
	}
	if !roomDeleted {
		t.Fatal("LeaveCurrentRoom() roomDeleted = false, want true")
	}
	if _, exists := manager.GetRoom(room.ID); exists {
		t.Fatal("empty room still exists after LeaveCurrentRoom()")
	}
	if _, exists := service.playerRoomIDs[host.UserID]; exists {
		t.Fatal("player room index still exists after LeaveCurrentRoom()")
	}

	// 索引清理后，该玩家应该可以再次创建房间。
	if _, err := service.CreateRoom(host); err != nil {
		t.Fatalf("CreateRoom() after leaving error = %v", err)
	}
}

func TestRoomServiceLeaveCurrentRoomRejectsPlayerWithoutRoom(t *testing.T) {
	service := NewRoomService(NewRoomManager())

	_, _, err := service.LeaveCurrentRoom(1)
	if !errors.Is(err, ErrPlayerNotInRoom) {
		t.Fatalf("LeaveCurrentRoom() error = %v, want %v", err, ErrPlayerNotInRoom)
	}
}

func TestRoomServiceJoinRoom(t *testing.T) {
	manager := NewRoomManager()
	service := NewRoomService(manager)
	host := &Player{UserID: 1, Username: "host"}
	guest := &Player{UserID: 2, Username: "guest"}

	room, err := service.CreateRoom(host)
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}

	joinedRoom, err := service.JoinRoom(room.ID, guest)
	if err != nil {
		t.Fatalf("JoinRoom() error = %v", err)
	}
	if joinedRoom != room {
		t.Fatal("JoinRoom() returned a different Room pointer")
	}
	if players := room.GetPlayers(); len(players) != 2 {
		t.Fatalf("player count = %d, want 2", len(players))
	}
	if indexedRoomID := service.playerRoomIDs[guest.UserID]; indexedRoomID != room.ID {
		t.Fatalf("guest room index = %q, want %q", indexedRoomID, room.ID)
	}
}

func TestRoomServiceJoinRoomRejectsMissingRoom(t *testing.T) {
	service := NewRoomService(NewRoomManager())

	_, err := service.JoinRoom("ABC234", &Player{UserID: 1, Username: "player"})
	if !errors.Is(err, ErrRoomNotFound) {
		t.Fatalf("JoinRoom() error = %v, want %v", err, ErrRoomNotFound)
	}
}

func TestRoomServiceJoinRoomRejectsFullRoom(t *testing.T) {
	manager := NewRoomManager()
	service := NewRoomService(manager)
	room, err := service.CreateRoom(&Player{UserID: 1, Username: "host"})
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	if _, err := service.JoinRoom(room.ID, &Player{UserID: 2, Username: "guest"}); err != nil {
		t.Fatalf("first JoinRoom() error = %v", err)
	}

	thirdPlayer := &Player{UserID: 3, Username: "third"}
	_, err = service.JoinRoom(room.ID, thirdPlayer)
	if !errors.Is(err, ErrRoomFull) {
		t.Fatalf("full JoinRoom() error = %v, want %v", err, ErrRoomFull)
	}
	if _, exists := service.playerRoomIDs[thirdPlayer.UserID]; exists {
		t.Fatal("full JoinRoom() registered rejected player")
	}
}

func TestRoomServiceJoinRoomRejectsPlayerAlreadyInRoom(t *testing.T) {
	manager := NewRoomManager()
	service := NewRoomService(manager)
	player := &Player{UserID: 1, Username: "player"}
	firstRoom, err := service.CreateRoom(player)
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	secondRoom, err := manager.CreateRoom()
	if err != nil {
		t.Fatalf("manager CreateRoom() error = %v", err)
	}

	_, err = service.JoinRoom(secondRoom.ID, player)
	if !errors.Is(err, ErrPlayerAlreadyInAnotherRoom) {
		t.Fatalf("JoinRoom() error = %v, want %v", err, ErrPlayerAlreadyInAnotherRoom)
	}
	if players := firstRoom.GetPlayers(); len(players) != 1 {
		t.Fatalf("first room player count = %d, want 1", len(players))
	}
	if players := secondRoom.GetPlayers(); len(players) != 0 {
		t.Fatalf("second room player count = %d, want 0", len(players))
	}
}

func TestRoomServiceStartCurrentRoom(t *testing.T) {
	manager := NewRoomManager()
	service := NewRoomService(manager)
	host := &Player{UserID: 1, Username: "host"}
	guest := &Player{UserID: 2, Username: "guest"}
	room, err := service.CreateRoom(host)
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	if _, err := service.JoinRoom(room.ID, guest); err != nil {
		t.Fatalf("JoinRoom() error = %v", err)
	}

	startedRoom, err := service.StartCurrentRoom(host.UserID)
	if err != nil {
		t.Fatalf("StartCurrentRoom() error = %v", err)
	}
	if startedRoom != room {
		t.Fatal("StartCurrentRoom() returned a different Room pointer")
	}
	if status := room.Status(); status != RoomStatusPlaying {
		t.Fatalf("started room status = %q, want %q", status, RoomStatusPlaying)
	}
}

func TestRoomServiceStartCurrentRoomRejectsGuest(t *testing.T) {
	manager := NewRoomManager()
	service := NewRoomService(manager)
	host := &Player{UserID: 1, Username: "host"}
	guest := &Player{UserID: 2, Username: "guest"}
	room, err := service.CreateRoom(host)
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	if _, err := service.JoinRoom(room.ID, guest); err != nil {
		t.Fatalf("JoinRoom() error = %v", err)
	}

	if _, err := service.StartCurrentRoom(guest.UserID); !errors.Is(err, ErrOnlyHostCanStart) {
		t.Fatalf("guest StartCurrentRoom() error = %v, want %v", err, ErrOnlyHostCanStart)
	}
}

func TestRoomServiceSubmitMoveSettlesCurrentRound(t *testing.T) {
	manager := NewRoomManager()
	service := NewRoomService(manager)
	host := &Player{UserID: 1, Username: "host"}
	guest := &Player{UserID: 2, Username: "guest"}
	room, err := service.CreateRoom(host)
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	if _, err := service.JoinRoom(room.ID, guest); err != nil {
		t.Fatalf("JoinRoom() error = %v", err)
	}
	if _, err := service.StartCurrentRoom(host.UserID); err != nil {
		t.Fatalf("StartCurrentRoom() error = %v", err)
	}

	firstState, err := service.SubmitMove(host.UserID, Rock)
	if err != nil {
		t.Fatalf("host SubmitMove() error = %v", err)
	}
	if firstState.Settled {
		t.Fatal("host SubmitMove() settled = true, want false")
	}

	settledState, err := service.SubmitMove(guest.UserID, Scissors)
	if err != nil {
		t.Fatalf("guest SubmitMove() error = %v", err)
	}
	if !settledState.Settled {
		t.Fatal("guest SubmitMove() settled = false, want true")
	}
	if settledState.Results[host.UserID] != Win || settledState.Results[guest.UserID] != Lose {
		t.Fatalf("results = %#v, want host win and guest lose", settledState.Results)
	}
}

func TestRoomServiceGetCurrentRoom(t *testing.T) {
	manager := NewRoomManager()
	service := NewRoomService(manager)
	host := &Player{UserID: 1, Username: "host"}
	room, err := service.CreateRoom(host)
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}

	snapshot, err := service.GetCurrentRoom(host.UserID)
	if err != nil {
		t.Fatalf("GetCurrentRoom() error = %v", err)
	}
	if snapshot.Room.ID != room.ID || snapshot.Room.HostUserID != host.UserID {
		t.Fatalf("GetCurrentRoom() snapshot = %#v, want room %s and host %d", snapshot, room.ID, host.UserID)
	}
}

func TestRoomServiceGetCurrentRoomRejectsPlayerWithoutRoom(t *testing.T) {
	service := NewRoomService(NewRoomManager())

	if _, err := service.GetCurrentRoom(1); !errors.Is(err, ErrPlayerNotInRoom) {
		t.Fatalf("GetCurrentRoom() error = %v, want %v", err, ErrPlayerNotInRoom)
	}
}
