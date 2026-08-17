package game

import (
	"errors"
	"fmt"
	"sync"
)

var (
	// ErrPlayerAlreadyInAnotherRoom 表示玩家已经加入其他房间，不能同时出现在多间房。
	ErrPlayerAlreadyInAnotherRoom = errors.New("player is already in another room")
	// ErrRoomNotFound 表示指定房间号当前不存在。
	ErrRoomNotFound = errors.New("room not found")
)

// RoomService 编排跨越 RoomManager 和 Room 的房间业务流程。
type RoomService struct {
	// mu 保护跨房间的玩家归属关系，并保证检查和登记归属是一个原子操作。
	mu sync.Mutex
	// roomManager 保存服务器当前全部房间。
	roomManager *RoomManager
	// playerRoomIDs 根据 UserID 记录玩家当前所在的唯一房间。
	playerRoomIDs map[int64]string
}

// NewRoomService 注入唯一的 RoomManager，让所有请求共享同一份房间集合。
func NewRoomService(roomManager *RoomManager) *RoomService {
	return &RoomService{
		roomManager:   roomManager,
		playerRoomIDs: make(map[int64]string),
	}
}

// CreateRoom 创建房间，并让创建者自动成为房间中的第一名玩家。
func (s *RoomService) CreateRoom(host *Player) (*Room, error) {
	if host == nil {
		return nil, ErrPlayerRequired
	}

	// “检查是否已有房间”和“登记新房间”必须处于同一临界区，
	// 否则同一用户的两个并发请求可能同时通过检查并各自创建房间。
	s.mu.Lock()
	defer s.mu.Unlock()

	if existingRoomID, exists := s.playerRoomIDs[host.UserID]; exists {
		return nil, fmt.Errorf(
			"%w: %s",
			ErrPlayerAlreadyInAnotherRoom,
			existingRoomID,
		)
	}

	room, err := s.roomManager.CreateRoom()
	if err != nil {
		return nil, err
	}

	if err := room.AddPlayer(host); err != nil {
		// 创建者加入失败时回滚刚创建的空房间，避免留下无主房间。
		s.roomManager.DeleteRoom(room.ID)
		return nil, fmt.Errorf("add room host: %w", err)
	}

	// 房间和首名玩家都创建成功后，登记玩家的唯一房间归属。
	s.playerRoomIDs[host.UserID] = room.ID

	return room, nil
}

// ListRooms 返回当前进程中仍由 RoomManager 管理的全部房间。
func (s *RoomService) ListRooms() []*Room {
	return s.roomManager.ListRooms()
}

// GetCurrentRoom 返回指定玩家当前所在房间的安全视角快照。
func (s *RoomService) GetCurrentRoom(userID int64) (PlayerRoomSnapshot, error) {
	// 查询也与加入和退出共用业务锁，保证 playerRoomIDs 与 RoomManager 的关系稳定。
	// 这里不用 RWMutex/RLock，是因为发现失效索引时需要执行 delete 清理写操作。
	s.mu.Lock()
	defer s.mu.Unlock()

	roomID, exists := s.playerRoomIDs[userID]
	if !exists {
		return PlayerRoomSnapshot{}, ErrPlayerNotInRoom
	}

	room, exists := s.roomManager.GetRoom(roomID)
	if !exists {
		// 出现失效索引时立即清理，避免该用户以后一直被判断为“已在房间”。
		delete(s.playerRoomIDs, userID)
		return PlayerRoomSnapshot{}, fmt.Errorf(
			"room %s referenced by player %d does not exist",
			roomID,
			userID,
		)
	}

	return room.SnapshotForPlayer(userID)
}

// JoinRoom 让玩家根据房间号加入一间已有房间。
func (s *RoomService) JoinRoom(roomID string, player *Player) (*Room, error) {
	if player == nil {
		return nil, ErrPlayerRequired
	}

	// 检查归属、加入房间和登记归属必须作为一个整体执行，
	// 防止同一玩家并发加入多间房，或多名玩家同时突破房间人数限制。
	s.mu.Lock()
	defer s.mu.Unlock()

	if existingRoomID, exists := s.playerRoomIDs[player.UserID]; exists {
		return nil, fmt.Errorf(
			"%w: %s",
			ErrPlayerAlreadyInAnotherRoom,
			existingRoomID,
		)
	}

	room, exists := s.roomManager.GetRoom(roomID)
	if !exists {
		return nil, ErrRoomNotFound
	}

	if err := room.AddPlayer(player); err != nil {
		return nil, err
	}

	// 使用 Room 中的规范化 ID，避免把小写输入保存进玩家归属索引。
	s.playerRoomIDs[player.UserID] = room.ID
	return room, nil
}

// StartCurrentRoom 让当前玩家尝试开始自己所在房间的对局。
func (s *RoomService) StartCurrentRoom(userID int64) (*Room, error) {
	// 与加入、退出共用同一把业务锁，避免开始过程中玩家恰好离开。
	s.mu.Lock()
	defer s.mu.Unlock()

	roomID, exists := s.playerRoomIDs[userID]
	if !exists {
		return nil, ErrPlayerNotInRoom
	}

	room, exists := s.roomManager.GetRoom(roomID)
	if !exists {
		delete(s.playerRoomIDs, userID)
		return nil, fmt.Errorf("room %s referenced by player %d does not exist", roomID, userID)
	}

	if err := room.Start(userID); err != nil {
		return nil, err
	}

	return room, nil
}

// SubmitMove 让当前玩家向自己所在房间提交本局出拳。
func (s *RoomService) SubmitMove(userID int64, move Move) (RoundStateSnapshot, error) {
	// 与开始和退出共用业务锁，避免查到房间后玩家恰好退出或房间被删除。
	s.mu.Lock()
	defer s.mu.Unlock()

	roomID, exists := s.playerRoomIDs[userID]
	if !exists {
		return RoundStateSnapshot{}, ErrPlayerNotInRoom
	}

	room, exists := s.roomManager.GetRoom(roomID)
	if !exists {
		delete(s.playerRoomIDs, userID)
		return RoundStateSnapshot{}, fmt.Errorf(
			"room %s referenced by player %d does not exist",
			roomID,
			userID,
		)
	}

	// Service 只负责编排和定位；玩家资格、房间状态、判重与结算由 Room 统一保证。
	return room.SubmitMove(userID, move)
}

// LeaveCurrentRoom 让玩家退出当前房间，并在房间变空后删除该房间。
// 返回退出的房间号、房间是否被删除，以及可能出现的错误。
func (s *RoomService) LeaveCurrentRoom(userID int64) (string, bool, error) {
	// 退出过程同时修改玩家归属索引和房间状态，必须与创建、加入流程互斥。
	s.mu.Lock()
	defer s.mu.Unlock()

	roomID, exists := s.playerRoomIDs[userID]
	if !exists {
		return "", false, ErrPlayerNotInRoom
	}

	room, exists := s.roomManager.GetRoom(roomID)
	if !exists {
		// 索引与房间集合不一致时先清理失效索引，再向上报告内部状态错误。
		delete(s.playerRoomIDs, userID)
		return "", false, fmt.Errorf("room %s referenced by player %d does not exist", roomID, userID)
	}

	if removed := room.RemovePlayer(userID); !removed {
		// 同样清理已经失效的索引，避免用户以后永远无法加入新房间。
		delete(s.playerRoomIDs, userID)
		return "", false, fmt.Errorf("player %d is missing from room %s", userID, roomID)
	}

	// Room 和全局玩家归属必须同步更新。
	delete(s.playerRoomIDs, userID)

	roomDeleted := len(room.GetPlayers()) == 0
	if roomDeleted {
		s.roomManager.DeleteRoom(roomID)
	}

	return roomID, roomDeleted, nil
}
