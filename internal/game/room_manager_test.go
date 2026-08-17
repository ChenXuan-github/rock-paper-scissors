package game

import (
	"strings"
	"testing"
)

func TestNewRoomManagerInitializesRoomMap(t *testing.T) {
	manager := NewRoomManager()

	// 构造函数必须初始化 map，否则后续创建房间时向 nil map 写入会 panic。
	if manager.rooms == nil {
		t.Fatal("NewRoomManager() left rooms map nil")
	}
}

func TestRoomManagerCreateRoom(t *testing.T) {
	manager := NewRoomManager()

	room, err := manager.CreateRoom()
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	if len(room.ID) != roomIDLength {
		t.Fatalf("CreateRoom() room ID length = %d, want %d", len(room.ID), roomIDLength)
	}
	for _, character := range room.ID {
		if !strings.ContainsRune(roomIDAlphabet, character) {
			t.Fatalf("CreateRoom() room ID %q contains invalid character %q", room.ID, character)
		}
	}
	if len(room.players) != 0 {
		t.Fatalf("CreateRoom() player count = %d, want 0", len(room.players))
	}
	if manager.rooms[room.ID] != room {
		t.Fatal("CreateRoom() did not store the created room in RoomManager")
	}
}

func TestRoomManagerCreateRoomGeneratesUniqueIDs(t *testing.T) {
	manager := NewRoomManager()

	firstRoom, err := manager.CreateRoom()
	if err != nil {
		t.Fatalf("first CreateRoom() error = %v", err)
	}
	secondRoom, err := manager.CreateRoom()
	if err != nil {
		t.Fatalf("second CreateRoom() error = %v", err)
	}
	if firstRoom.ID == secondRoom.ID {
		t.Fatalf("CreateRoom() generated duplicate room ID %q", firstRoom.ID)
	}
}

func TestRoomManagerGetRoom(t *testing.T) {
	manager := NewRoomManager()
	createdRoom, err := manager.CreateRoom()
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}

	// 模拟玩家输入带空格的小写房间号，也应该找到服务器保存的房间。
	inputRoomID := " " + strings.ToLower(createdRoom.ID) + " "
	foundRoom, exists := manager.GetRoom(inputRoomID)
	if !exists {
		t.Fatalf("GetRoom(%q) exists = false, want true", inputRoomID)
	}
	if foundRoom != createdRoom {
		t.Fatal("GetRoom() returned a different Room pointer")
	}
}

func TestRoomManagerGetRoomReturnsFalseWhenMissing(t *testing.T) {
	manager := NewRoomManager()

	room, exists := manager.GetRoom("ABC234")
	if exists {
		t.Fatal("GetRoom() exists = true, want false")
	}
	if room != nil {
		t.Fatalf("GetRoom() room = %#v, want nil", room)
	}
}

func TestRoomManagerDeleteRoom(t *testing.T) {
	manager := NewRoomManager()
	createdRoom, err := manager.CreateRoom()
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}

	// 删除时同样兼容小写和首尾空格。
	deletedRoom, deleted := manager.DeleteRoom(" " + strings.ToLower(createdRoom.ID) + " ")
	if !deleted {
		t.Fatal("DeleteRoom() deleted = false, want true")
	}
	if deletedRoom != createdRoom {
		t.Fatal("DeleteRoom() returned a different Room pointer")
	}
	if _, exists := manager.GetRoom(createdRoom.ID); exists {
		t.Fatal("GetRoom() found room after DeleteRoom()")
	}
}

func TestRoomManagerDeleteRoomReturnsFalseWhenMissing(t *testing.T) {
	manager := NewRoomManager()

	room, deleted := manager.DeleteRoom("ABC234")
	if deleted {
		t.Fatal("DeleteRoom() deleted = true, want false")
	}
	if room != nil {
		t.Fatalf("DeleteRoom() room = %#v, want nil", room)
	}
}

func TestRoomManagerListRooms(t *testing.T) {
	manager := NewRoomManager()
	firstRoom, err := manager.CreateRoom()
	if err != nil {
		t.Fatalf("first CreateRoom() error = %v", err)
	}
	secondRoom, err := manager.CreateRoom()
	if err != nil {
		t.Fatalf("second CreateRoom() error = %v", err)
	}

	rooms := manager.ListRooms()
	if len(rooms) != 2 {
		t.Fatalf("ListRooms() count = %d, want 2", len(rooms))
	}

	found := map[*Room]bool{}
	for _, room := range rooms {
		found[room] = true
	}
	if !found[firstRoom] || !found[secondRoom] {
		t.Fatal("ListRooms() did not return all created rooms")
	}
}
