package game

import (
	"errors"
	"testing"
)

func TestRoomAddPlayer(t *testing.T) {
	room := newRoom("ABC234")
	player := &Player{UserID: 1, Username: "chenxuan"}

	if err := room.AddPlayer(player); err != nil {
		t.Fatalf("AddPlayer() error = %v", err)
	}

	players := room.GetPlayers()
	if len(players) != 1 {
		t.Fatalf("GetPlayers() count = %d, want 1", len(players))
	}
	if players[0] != player {
		t.Fatal("GetPlayers() returned a different Player pointer")
	}
}

func TestRoomAddPlayerRejectsInvalidPlayer(t *testing.T) {
	room := newRoom("ABC234")

	if err := room.AddPlayer(nil); !errors.Is(err, ErrPlayerRequired) {
		t.Fatalf("AddPlayer(nil) error = %v, want %v", err, ErrPlayerRequired)
	}
}

func TestRoomAddPlayerRejectsDuplicatePlayer(t *testing.T) {
	room := newRoom("ABC234")
	if err := room.AddPlayer(&Player{UserID: 1, Username: "chenxuan"}); err != nil {
		t.Fatalf("first AddPlayer() error = %v", err)
	}

	err := room.AddPlayer(&Player{UserID: 1, Username: "another-name"})
	if !errors.Is(err, ErrPlayerAlreadyInRoom) {
		t.Fatalf("duplicate AddPlayer() error = %v, want %v", err, ErrPlayerAlreadyInRoom)
	}
}

func TestRoomRejectsThirdPlayerWhenFull(t *testing.T) {
	room := newRoom("ABC234")
	if err := room.AddPlayer(&Player{UserID: 1, Username: "player-1"}); err != nil {
		t.Fatalf("first AddPlayer() error = %v", err)
	}
	if err := room.AddPlayer(&Player{UserID: 2, Username: "player-2"}); err != nil {
		t.Fatalf("second AddPlayer() error = %v", err)
	}
	if !room.IsFull() {
		t.Fatal("IsFull() = false, want true")
	}

	err := room.AddPlayer(&Player{UserID: 3, Username: "player-3"})
	if !errors.Is(err, ErrRoomFull) {
		t.Fatalf("third AddPlayer() error = %v, want %v", err, ErrRoomFull)
	}
}

func TestRoomRemovePlayer(t *testing.T) {
	room := newRoom("ABC234")
	firstPlayer := &Player{UserID: 1, Username: "player-1"}
	secondPlayer := &Player{UserID: 2, Username: "player-2"}
	if err := room.AddPlayer(firstPlayer); err != nil {
		t.Fatalf("first AddPlayer() error = %v", err)
	}
	if err := room.AddPlayer(secondPlayer); err != nil {
		t.Fatalf("second AddPlayer() error = %v", err)
	}

	if removed := room.RemovePlayer(firstPlayer.UserID); !removed {
		t.Fatal("RemovePlayer() = false, want true")
	}
	if room.IsFull() {
		t.Fatal("IsFull() = true after removing one player")
	}
	players := room.GetPlayers()
	if len(players) != 1 || players[0] != secondPlayer {
		t.Fatalf("GetPlayers() = %#v, want only second player", players)
	}
	if removed := room.RemovePlayer(999); removed {
		t.Fatal("RemovePlayer() missing user = true, want false")
	}
}

func TestRoomGetPlayersReturnsIndependentSlice(t *testing.T) {
	room := newRoom("ABC234")
	player := &Player{UserID: 1, Username: "chenxuan"}
	if err := room.AddPlayer(player); err != nil {
		t.Fatalf("AddPlayer() error = %v", err)
	}

	players := room.GetPlayers()
	players[0] = nil

	roomPlayers := room.GetPlayers()
	if roomPlayers[0] != player {
		t.Fatal("modifying GetPlayers() result changed Room internal slice")
	}
}

func TestRoomStoresPlayersByUserID(t *testing.T) {
	room := newRoom("ABC234")
	player := &Player{UserID: 7, Username: "player-7"}
	if err := room.AddPlayer(player); err != nil {
		t.Fatalf("AddPlayer() error = %v", err)
	}

	storedPlayer, exists := room.players[player.UserID]
	if !exists {
		t.Fatalf("players[%d] does not exist", player.UserID)
	}
	if storedPlayer != player {
		t.Fatal("players map stored a different Player pointer")
	}
}

func TestRoomStatusLifecycle(t *testing.T) {
	room := newRoom("ABC234")
	if status := room.Status(); status != RoomStatusWaiting {
		t.Fatalf("new room status = %q, want %q", status, RoomStatusWaiting)
	}

	firstPlayer := &Player{UserID: 1, Username: "player-1"}
	secondPlayer := &Player{UserID: 2, Username: "player-2"}
	if err := room.AddPlayer(firstPlayer); err != nil {
		t.Fatalf("first AddPlayer() error = %v", err)
	}
	if status := room.Status(); status != RoomStatusWaiting {
		t.Fatalf("one-player status = %q, want %q", status, RoomStatusWaiting)
	}
	if err := room.AddPlayer(secondPlayer); err != nil {
		t.Fatalf("second AddPlayer() error = %v", err)
	}
	if status := room.Status(); status != RoomStatusReady {
		t.Fatalf("two-player status = %q, want %q", status, RoomStatusReady)
	}

	if err := room.Start(firstPlayer.UserID); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if status := room.Status(); status != RoomStatusPlaying {
		t.Fatalf("started status = %q, want %q", status, RoomStatusPlaying)
	}

	if removed := room.RemovePlayer(secondPlayer.UserID); !removed {
		t.Fatal("RemovePlayer() = false, want true")
	}
	if status := room.Status(); status != RoomStatusWaiting {
		t.Fatalf("one-player status after leave = %q, want %q", status, RoomStatusWaiting)
	}

	if removed := room.RemovePlayer(firstPlayer.UserID); !removed {
		t.Fatal("RemovePlayer() = false, want true")
	}
	if status := room.Status(); status != RoomStatusClosed {
		t.Fatalf("empty room status = %q, want %q", status, RoomStatusClosed)
	}
}

func TestRoomStartRequiresReadyStatus(t *testing.T) {
	room := newRoom("ABC234")

	if err := room.Start(999); !errors.Is(err, ErrPlayerNotInRoom) {
		t.Fatalf("Start() outsider error = %v, want %v", err, ErrPlayerNotInRoom)
	}

	player := &Player{UserID: 1, Username: "player"}
	if err := room.AddPlayer(player); err != nil {
		t.Fatalf("AddPlayer() error = %v", err)
	}
	if err := room.Start(player.UserID); !errors.Is(err, ErrRoomNotReady) {
		t.Fatalf("Start() error = %v, want %v", err, ErrRoomNotReady)
	}
}

func TestRoomClosePreventsJoining(t *testing.T) {
	room := newRoom("ABC234")
	room.Close()

	if status := room.Status(); status != RoomStatusClosed {
		t.Fatalf("Close() status = %q, want %q", status, RoomStatusClosed)
	}
	err := room.AddPlayer(&Player{UserID: 1, Username: "player"})
	if !errors.Is(err, ErrRoomNotJoinable) {
		t.Fatalf("AddPlayer() closed room error = %v, want %v", err, ErrRoomNotJoinable)
	}
}

func TestRoomSnapshotContainsMatchingStatusAndPlayers(t *testing.T) {
	room := newRoom("ABC234")
	if err := room.AddPlayer(&Player{UserID: 1, Username: "player-1"}); err != nil {
		t.Fatalf("first AddPlayer() error = %v", err)
	}
	if err := room.AddPlayer(&Player{UserID: 2, Username: "player-2"}); err != nil {
		t.Fatalf("second AddPlayer() error = %v", err)
	}

	snapshot := room.Snapshot()
	if snapshot.ID != room.ID {
		t.Fatalf("Snapshot() ID = %q, want %q", snapshot.ID, room.ID)
	}
	if snapshot.Status != RoomStatusReady {
		t.Fatalf("Snapshot() status = %q, want %q", snapshot.Status, RoomStatusReady)
	}
	if len(snapshot.Players) != 2 {
		t.Fatalf("Snapshot() player count = %d, want 2", len(snapshot.Players))
	}
	if snapshot.HostUserID != 1 {
		t.Fatalf("Snapshot() host user ID = %d, want 1", snapshot.HostUserID)
	}
}

func TestRoomTransfersHostWhenHostLeaves(t *testing.T) {
	room := newRoom("ABC234")
	host := &Player{UserID: 1, Username: "host"}
	guest := &Player{UserID: 2, Username: "guest"}
	if err := room.AddPlayer(host); err != nil {
		t.Fatalf("host AddPlayer() error = %v", err)
	}
	if err := room.AddPlayer(guest); err != nil {
		t.Fatalf("guest AddPlayer() error = %v", err)
	}
	if hostUserID := room.HostUserID(); hostUserID != host.UserID {
		t.Fatalf("initial host user ID = %d, want %d", hostUserID, host.UserID)
	}

	if removed := room.RemovePlayer(host.UserID); !removed {
		t.Fatal("RemovePlayer(host) = false, want true")
	}
	if hostUserID := room.HostUserID(); hostUserID != guest.UserID {
		t.Fatalf("transferred host user ID = %d, want %d", hostUserID, guest.UserID)
	}
	if status := room.Status(); status != RoomStatusWaiting {
		t.Fatalf("status after host leaves = %q, want %q", status, RoomStatusWaiting)
	}
}

func TestRoomOnlyHostCanStart(t *testing.T) {
	room := newRoom("ABC234")
	host := &Player{UserID: 1, Username: "host"}
	guest := &Player{UserID: 2, Username: "guest"}
	if err := room.AddPlayer(host); err != nil {
		t.Fatalf("host AddPlayer() error = %v", err)
	}
	if err := room.AddPlayer(guest); err != nil {
		t.Fatalf("guest AddPlayer() error = %v", err)
	}

	if err := room.Start(guest.UserID); !errors.Is(err, ErrOnlyHostCanStart) {
		t.Fatalf("guest Start() error = %v, want %v", err, ErrOnlyHostCanStart)
	}
	if err := room.Start(host.UserID); err != nil {
		t.Fatalf("host Start() error = %v", err)
	}
}

func TestRoomSubmitMovesSettlesUsingExistingRoundRule(t *testing.T) {
	room := newRoom("ABC234")
	host := &Player{UserID: 1, Username: "host"}
	guest := &Player{UserID: 2, Username: "guest"}
	if err := room.AddPlayer(host); err != nil {
		t.Fatalf("host AddPlayer() error = %v", err)
	}
	if err := room.AddPlayer(guest); err != nil {
		t.Fatalf("guest AddPlayer() error = %v", err)
	}
	if err := room.Start(host.UserID); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	firstSnapshot, err := room.SubmitMove(host.UserID, Rock)
	if err != nil {
		t.Fatalf("first SubmitMove() error = %v", err)
	}
	if firstSnapshot.Settled {
		t.Fatal("first submission settled = true, want false")
	}
	if firstSnapshot.SubmittedCount != 1 {
		t.Fatalf("first submitted count = %d, want 1", firstSnapshot.SubmittedCount)
	}
	if status := room.Status(); status != RoomStatusPlaying {
		t.Fatalf("status after first move = %q, want %q", status, RoomStatusPlaying)
	}

	settledSnapshot, err := room.SubmitMove(guest.UserID, Scissors)
	if err != nil {
		t.Fatalf("second SubmitMove() error = %v", err)
	}
	if !settledSnapshot.Settled {
		t.Fatal("second submission settled = false, want true")
	}
	if settledSnapshot.Results[host.UserID] != Win {
		t.Fatalf("host result = %v, want %v", settledSnapshot.Results[host.UserID], Win)
	}
	if settledSnapshot.Results[guest.UserID] != Lose {
		t.Fatalf("guest result = %v, want %v", settledSnapshot.Results[guest.UserID], Lose)
	}
	if status := room.Status(); status != RoomStatusReady {
		t.Fatalf("status after settlement = %q, want %q", status, RoomStatusReady)
	}
}

func TestRoomRejectsDuplicateMove(t *testing.T) {
	room := newRoom("ABC234")
	host := &Player{UserID: 1, Username: "host"}
	guest := &Player{UserID: 2, Username: "guest"}
	if err := room.AddPlayer(host); err != nil {
		t.Fatalf("host AddPlayer() error = %v", err)
	}
	if err := room.AddPlayer(guest); err != nil {
		t.Fatalf("guest AddPlayer() error = %v", err)
	}
	if err := room.Start(host.UserID); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if _, err := room.SubmitMove(host.UserID, Paper); err != nil {
		t.Fatalf("first SubmitMove() error = %v", err)
	}
	if _, err := room.SubmitMove(host.UserID, Rock); !errors.Is(err, ErrMoveAlreadySubmitted) {
		t.Fatalf("duplicate SubmitMove() error = %v, want %v", err, ErrMoveAlreadySubmitted)
	}
}

func TestRoomRejectsMoveBeforeGameStarts(t *testing.T) {
	room := newRoom("ABC234")
	player := &Player{UserID: 1, Username: "player"}
	if err := room.AddPlayer(player); err != nil {
		t.Fatalf("AddPlayer() error = %v", err)
	}

	if _, err := room.SubmitMove(player.UserID, Rock); !errors.Is(err, ErrRoomNotPlaying) {
		t.Fatalf("SubmitMove() error = %v, want %v", err, ErrRoomNotPlaying)
	}
}

func TestRoomSupportsConsecutiveRoundsWithoutReusingMoves(t *testing.T) {
	room := newRoom("ABC234")
	host := &Player{UserID: 1, Username: "host"}
	guest := &Player{UserID: 2, Username: "guest"}
	if err := room.AddPlayer(host); err != nil {
		t.Fatalf("host AddPlayer() error = %v", err)
	}
	if err := room.AddPlayer(guest); err != nil {
		t.Fatalf("guest AddPlayer() error = %v", err)
	}

	// 第一局：石头战胜剪刀。
	if err := room.Start(host.UserID); err != nil {
		t.Fatalf("first Start() error = %v", err)
	}
	if _, err := room.SubmitMove(host.UserID, Rock); err != nil {
		t.Fatalf("first round host SubmitMove() error = %v", err)
	}
	if _, err := room.SubmitMove(guest.UserID, Scissors); err != nil {
		t.Fatalf("first round guest SubmitMove() error = %v", err)
	}

	// 第二次 Start 必须创建全新的 RoundState，不能继承第一局的 moves 和 results。
	if err := room.Start(host.UserID); err != nil {
		t.Fatalf("second Start() error = %v", err)
	}
	freshSnapshot, err := room.SnapshotForPlayer(host.UserID)
	if err != nil {
		t.Fatalf("SnapshotForPlayer() error = %v", err)
	}
	if freshSnapshot.Round == nil {
		t.Fatal("second round snapshot is nil")
	}
	if freshSnapshot.Round.SubmittedCount != 0 || freshSnapshot.Round.Settled {
		t.Fatalf("fresh round = %#v, want zero submissions and unsettled", freshSnapshot.Round)
	}

	// 第二局改为布对石头，证明结果完全来自本局的新选择。
	if _, err := room.SubmitMove(host.UserID, Paper); err != nil {
		t.Fatalf("second round host SubmitMove() error = %v", err)
	}
	secondResult, err := room.SubmitMove(guest.UserID, Rock)
	if err != nil {
		t.Fatalf("second round guest SubmitMove() error = %v", err)
	}
	if secondResult.Results[host.UserID] != Win || secondResult.Results[guest.UserID] != Lose {
		t.Fatalf("second round results = %#v, want host win and guest lose", secondResult.Results)
	}
}

func TestRoomSnapshotForPlayerHidesUnsettledOpponentMove(t *testing.T) {
	room := newRoom("ABC234")
	host := &Player{UserID: 1, Username: "host"}
	guest := &Player{UserID: 2, Username: "guest"}
	if err := room.AddPlayer(host); err != nil {
		t.Fatalf("host AddPlayer() error = %v", err)
	}
	if err := room.AddPlayer(guest); err != nil {
		t.Fatalf("guest AddPlayer() error = %v", err)
	}
	if err := room.Start(host.UserID); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if _, err := room.SubmitMove(guest.UserID, Scissors); err != nil {
		t.Fatalf("guest SubmitMove() error = %v", err)
	}

	hostSnapshot, err := room.SnapshotForPlayer(host.UserID)
	if err != nil {
		t.Fatalf("SnapshotForPlayer() error = %v", err)
	}
	if hostSnapshot.Round == nil || hostSnapshot.Round.SubmittedCount != 1 {
		t.Fatalf("host round snapshot = %#v, want one submission", hostSnapshot.Round)
	}
	if hostSnapshot.Round.Submitted || hostSnapshot.Round.Settled {
		t.Fatalf("host round snapshot = %#v, want host not submitted and unsettled", hostSnapshot.Round)
	}
}
