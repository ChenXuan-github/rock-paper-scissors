package game

import (
	"crypto/rand"
	"fmt"
	"strings"
	"sync"
)

const (
	// 房间号使用 6 位便于玩家查看、输入和分享的字符。
	roomIDLength = 6
	// 去掉容易混淆的 I、O、0、1；共 32 个字符，方便用随机字节均匀取值。
	roomIDAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
)

// RoomManager 管理服务器当前仍然存在的全部内存房间。
// Handler 的一次请求结束后 Room 仍需继续存在，因此不能把房间只保存在 Handler 局部变量中。
type RoomManager struct {
	// mu 保护 rooms map。Gin 会并发处理请求，而 Go 原生 map 不允许无保护地并发读写。
	mu sync.RWMutex
	// rooms 使用房间 ID 作为键，指针让所有请求操作的是同一个 Room 对象。
	rooms map[string]*Room
}

// NewRoomManager 创建房间管理器，并初始化内部 map。
func NewRoomManager() *RoomManager {
	return &RoomManager{
		// map 的零值是 nil，只能读取、不能写入，所以必须先用 make 初始化。
		rooms: make(map[string]*Room),
	}
}

// CreateRoom 创建一间空的 1v1 房间，由服务器生成唯一房间号并保存到 RoomManager。
func (m *RoomManager) CreateRoom() (*Room, error) {
	// 创建房间会修改 rooms map，因此必须使用写锁。
	m.mu.Lock()
	defer m.mu.Unlock()

	var roomID string
	for {
		generatedID, err := generateRoomID()
		if err != nil {
			return nil, fmt.Errorf("generate room ID: %w", err)
		}

		// 随机房间号理论上可能碰撞；只有 Map 中不存在时才使用。
		if _, exists := m.rooms[generatedID]; !exists {
			roomID = generatedID
			break
		}
	}

	room := newRoom(roomID)

	m.rooms[roomID] = room

	return room, nil
}

// GetRoom 根据房间号查询 RoomManager 中仍然存在的房间。
// 返回值中的 bool 表示是否找到了对应房间。
func (m *RoomManager) GetRoom(roomID string) (*Room, bool) {
	// 玩家输入房间号时可能带有空格或使用小写，统一转换成服务器保存的格式。
	roomID = strings.ToUpper(strings.TrimSpace(roomID))

	// 查询不会修改 rooms map，使用读锁允许多个查询同时执行。
	m.mu.RLock()
	defer m.mu.RUnlock()

	// comma-ok：room 接收对应的房间指针，exists 表示 Key 是否存在。
	room, exists := m.rooms[roomID]
	return room, exists
}

// DeleteRoom 根据房间号从 RoomManager 中删除房间。
// 返回被删除的房间和是否成功找到该房间，方便上层决定如何响应。
func (m *RoomManager) DeleteRoom(roomID string) (*Room, bool) {
	roomID = strings.ToUpper(strings.TrimSpace(roomID))

	// delete 会修改 rooms map，因此必须使用写锁。
	m.mu.Lock()
	defer m.mu.Unlock()

	room, exists := m.rooms[roomID]
	if !exists {
		return nil, false
	}

	delete(m.rooms, roomID)
	return room, true
}

// ListRooms 返回 RoomManager 当前管理的全部房间。
// Map 本身没有固定顺序，因此返回切片中的房间顺序也不保证固定。
func (m *RoomManager) ListRooms() []*Room {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rooms := make([]*Room, 0, len(m.rooms))
	for _, room := range m.rooms {
		rooms = append(rooms, room)
	}

	return rooms
}

// generateRoomID 使用操作系统提供的安全随机数生成玩家可读的短房间号。
func generateRoomID() (string, error) {
	randomBytes := make([]byte, roomIDLength)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}

	roomID := make([]byte, roomIDLength)
	for i, randomByte := range randomBytes {
		// 字符表长度是 32，& 31 等价于对 32 取模，并且不会产生取模偏差。
		roomID[i] = roomIDAlphabet[int(randomByte)&31]
	}

	return string(roomID), nil
}
