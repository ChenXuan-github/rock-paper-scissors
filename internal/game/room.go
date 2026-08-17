package game

import (
	"errors"
	"sync"
)

const maxPlayersPerRoom = 2

var (
	// ErrPlayerRequired 表示调用加入房间时没有提供玩家对象。
	ErrPlayerRequired = errors.New("player is required")
	// ErrRoomFull 表示 1v1 房间已经有两名玩家。
	ErrRoomFull = errors.New("room is full")
	// ErrPlayerAlreadyInRoom 表示同一个用户不能重复加入同一房间。
	ErrPlayerAlreadyInRoom = errors.New("player is already in room")
	// ErrRoomNotJoinable 表示房间正在对局或已经关闭，不能再加入玩家。
	ErrRoomNotJoinable = errors.New("room is not joinable")
	// ErrRoomNotReady 表示房间尚未集齐两名玩家，不能开始对局。
	ErrRoomNotReady = errors.New("room is not ready")
	// ErrPlayerNotInRoom 表示指定玩家不属于当前房间。
	ErrPlayerNotInRoom = errors.New("player is not in a room")
	// ErrOnlyHostCanStart 表示只有当前房主有权开始对局。
	ErrOnlyHostCanStart = errors.New("only room host can start")
	// ErrRoomNotPlaying 表示房间还没有开始对局，不能提交出拳。
	ErrRoomNotPlaying = errors.New("room is not playing")
	// ErrMoveAlreadySubmitted 表示同一名玩家本局不能重复出拳。
	ErrMoveAlreadySubmitted = errors.New("move already submitted")
)

// RoomStatus 表示房间当前处于生命周期的哪个阶段。
type RoomStatus string

const (
	// RoomStatusWaiting 表示房间只有一名玩家，正在等待另一人加入。
	RoomStatusWaiting RoomStatus = "waiting"
	// RoomStatusReady 表示两名玩家都在房间中，可以由房主开始下一小局。
	RoomStatusReady RoomStatus = "ready"
	// RoomStatusPlaying 表示一小局已经开始，正在等待双方提交出拳。
	RoomStatusPlaying RoomStatus = "playing"
	// RoomStatusClosed 表示房间已关闭，不能再加入玩家或开始游戏。
	RoomStatusClosed RoomStatus = "closed"
)

// Room 表示服务器内存中的一间 1v1 游戏房间。
// 当前 Demo 不持久化房间，服务重启或房间销毁后，该对象会随之消失。
type Room struct {
	// mu 只保护当前这一间房的玩家和后续对局状态，不保护 RoomManager 的 rooms map。
	mu sync.RWMutex
	// ID 是房间的唯一标识，后续玩家通过它查找并加入指定房间。
	ID string
	// hostUserID 保存当前房主的 UserID；首名玩家成为房主，退出时可以移交。
	hostUserID int64
	// status 只能通过 Room 的方法读取和流转，不能由其他包随意赋值。
	status RoomStatus
	// players 以 UserID 为 Key 保存玩家，便于常数时间查找、判重和删除。
	// 使用小写私有字段，避免其他包绕过锁直接修改玩家集合。
	players map[int64]*Player
	// currentRound 只保存当前一小局的运行状态；历史战绩在后续阶段写入 MySQL。
	currentRound *RoundState
}

// newRoom 创建一间空房。房间只能由同包内的 RoomManager 创建，因此构造函数不导出。
func newRoom(roomID string) *Room {
	return &Room{
		ID:     roomID,
		status: RoomStatusWaiting,
		// Map 当前没有玩家，并提前按 1v1 房间的最大人数分配空间。
		players: make(map[int64]*Player, maxPlayersPerRoom),
	}
}

// AddPlayer 把玩家加入当前房间。
func (r *Room) AddPlayer(player *Player) error {
	if player == nil {
		return ErrPlayerRequired
	}

	// 加入玩家会修改 players map，因此使用写锁。
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.status == RoomStatusPlaying || r.status == RoomStatusClosed {
		return ErrRoomNotJoinable
	}

	// comma-ok 直接根据 UserID 判断玩家是否已在房间，不再遍历切片。
	if _, exists := r.players[player.UserID]; exists {
		return ErrPlayerAlreadyInRoom
	}

	if len(r.players) >= maxPlayersPerRoom {
		return ErrRoomFull
	}

	r.players[player.UserID] = player
	// 空房间的第一名玩家自动成为房主。
	if r.hostUserID == 0 {
		r.hostUserID = player.UserID
	}
	if len(r.players) == maxPlayersPerRoom {
		r.status = RoomStatusReady
	} else {
		r.status = RoomStatusWaiting
	}
	return nil
}

// RemovePlayer 根据用户 ID 将玩家移出当前房间。
func (r *Room) RemovePlayer(userID int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.players[userID]; !exists {
		return false
	}

	delete(r.players, userID)
	// 任意玩家中途退出，本局都无法继续，直接清除尚未持久化的当前小局。
	r.currentRound = nil
	if len(r.players) == 0 {
		// 最后一名玩家退出后，房间不再存在房主。
		r.hostUserID = 0
		r.status = RoomStatusClosed
	} else {
		// 当前 1v1 房间只会剩下一名玩家；房主退出时由该玩家自动接任。
		if userID == r.hostUserID {
			for remainingUserID := range r.players {
				r.hostUserID = remainingUserID
				break
			}
		}
		// 有一名玩家退出后，对局不能继续，房间重新等待另一名玩家。
		r.status = RoomStatusWaiting
	}
	return true
}

// IsFull 返回当前房间是否已经坐满两名玩家。
func (r *Room) IsFull() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.players) >= maxPlayersPerRoom
}

// Status 安全读取房间当前状态。
func (r *Room) Status() RoomStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.status
}

// HostUserID 安全读取当前房主的用户 ID。
func (r *Room) HostUserID() int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.hostUserID
}

// Start 校验发起者身份和房间状态，然后开始对局。
func (r *Room) Start(userID int64) error {
	// Start 同时会检查玩家集合、读取房主、修改 currentRound 和 status，必须使用写锁。
	r.mu.Lock()
	defer r.mu.Unlock()

	// 是否属于该房间
	if _, exists := r.players[userID]; !exists {
		return ErrPlayerNotInRoom
	}

	// 是否为房主
	if userID != r.hostUserID {
		return ErrOnlyHostCanStart
	}

	// 是否已经坐满两人
	if r.status != RoomStatusReady {
		return ErrRoomNotReady
	}

	// 每次开始都创建一个全新的当前小局，上一局结果后续会写入战绩表。
	r.currentRound = newRoundState()
	r.status = RoomStatusPlaying
	return nil
}

// SubmitMove 保存当前玩家的出拳；第二名玩家提交后立即复用 Round 完成结算。
func (r *Room) SubmitMove(userID int64, move Move) (RoundStateSnapshot, error) {
	// 两个 HTTP 请求可能同时到达；写锁保证保存第二只拳和结算只执行一次。
	r.mu.Lock()
	defer r.mu.Unlock()

	// 只有实际属于这间房的用户，才能参与当前小局。
	if _, exists := r.players[userID]; !exists {
		return RoundStateSnapshot{}, ErrPlayerNotInRoom
	}
	// 房主必须先调用 Start 创建 currentRound，结算后的 ready 状态也不能继续补交。
	if r.status != RoomStatusPlaying || r.currentRound == nil {
		return RoundStateSnapshot{}, ErrRoomNotPlaying
	}
	// moves Map 同时承担本局提交记录和快速判重作用，禁止玩家覆盖第一次选择。
	if _, exists := r.currentRound.moves[userID]; exists {
		return RoundStateSnapshot{}, ErrMoveAlreadySubmitted
	}

	// 第一名玩家提交时只保存，不结算，也不能向另一个客户端泄露具体拳型。
	r.currentRound.moves[userID] = move
	if len(r.currentRound.moves) < maxPlayersPerRoom {
		return r.currentRound.snapshot(), nil
	}

	// Map 没有固定顺序，但这里无须固定：选出的第一方和第二方会分别保存自己的结果。
	userIDs := make([]int64, 0, maxPlayersPerRoom)
	for submittedUserID := range r.currentRound.moves {
		userIDs = append(userIDs, submittedUserID)
	}
	firstUserID, secondUserID := userIDs[0], userIDs[1]

	// 复用 Day 1 的 Round：它仍然只负责根据两只拳计算第一方的结果。
	round := Round{
		PlayerMove:   r.currentRound.moves[firstUserID],
		OpponentMove: r.currentRound.moves[secondUserID],
	}
	round.Evaluate()

	// Round.Result 是 firstUserID 视角，另一名玩家必须保存方向相反的结果。
	r.currentRound.results[firstUserID] = round.Result
	r.currentRound.results[secondUserID] = oppositeResult(round.Result)
	// 两份结果全部写入后才标记 settled，避免读取方看到半完成的结算状态。
	r.currentRound.settled = true
	// 本局已经结束且两名玩家仍在房间，因此重新进入可开始下一局的 ready 状态。
	r.status = RoomStatusReady

	return r.currentRound.snapshot(), nil
}

// Close 将房间标记为关闭，阻止后续玩家加入。
func (r *Room) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.currentRound = nil
	r.status = RoomStatusClosed
}

// GetPlayers 把内部 Map 中的玩家整理成一个新切片返回。
// Map 没有固定遍历顺序，因此返回玩家的顺序也不保证固定。
func (r *Room) GetPlayers() []*Player {
	r.mu.RLock()
	defer r.mu.RUnlock()

	players := make([]*Player, 0, len(r.players))
	for _, player := range r.players {
		players = append(players, player)
	}

	return players
}

// RoomSnapshot 是对外读取房间信息时取得的一致性快照。
type RoomSnapshot struct {
	ID         string
	HostUserID int64
	Status     RoomStatus
	Players    []*Player
}

// PlayerRoundSnapshot 是当前小局针对某一名玩家生成的安全视图。
// Move 的零值是 Rock，因此必须配合 Submitted 判断该玩家是否真的已经出拳。
type PlayerRoundSnapshot struct {
	SubmittedCount int
	Submitted      bool
	Settled        bool
	Move           Move
	OpponentMove   Move
	Result         Result
}

// PlayerRoomSnapshot 同时包含房间公共信息和当前玩家可见的小局信息。
// Round 为 nil 表示房间还没有开始过对局，或进行中的对局因玩家退出而被取消。
type PlayerRoomSnapshot struct {
	Room  RoomSnapshot
	Round *PlayerRoundSnapshot
}

// Snapshot 在同一次读锁中复制状态与玩家，避免并发修改时两者互相对不上。
func (r *Room) Snapshot() RoomSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()

	players := make([]*Player, 0, len(r.players))
	for _, player := range r.players {
		players = append(players, player)
	}

	return RoomSnapshot{
		ID:         r.ID,
		HostUserID: r.hostUserID,
		Status:     r.status,
		Players:    players,
	}
}

// SnapshotForPlayer 在同一把读锁中生成当前玩家视角的房间快照。
// 未结算时绝不返回对手的拳；只有双方都提交并完成结算后才公开双方选择。
func (r *Room) SnapshotForPlayer(userID int64) (PlayerRoomSnapshot, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 即使调用方知道房间号，也只能读取自己实际加入的房间视角。
	if _, exists := r.players[userID]; !exists {
		return PlayerRoomSnapshot{}, ErrPlayerNotInRoom
	}

	// 复制玩家切片，避免把 Room 内部 players Map 直接交给上层。
	players := make([]*Player, 0, len(r.players))
	for _, player := range r.players {
		players = append(players, player)
	}

	snapshot := PlayerRoomSnapshot{
		Room: RoomSnapshot{
			ID:         r.ID,
			HostUserID: r.hostUserID,
			Status:     r.status,
			Players:    players,
		},
	}
	if r.currentRound == nil {
		return snapshot, nil
	}

	// 先返回所有玩家都可以知道的状态：已提交人数、自己是否提交以及是否结算。
	move, submitted := r.currentRound.moves[userID]
	playerRound := &PlayerRoundSnapshot{
		SubmittedCount: len(r.currentRound.moves),
		Submitted:      submitted,
		Settled:        r.currentRound.settled,
		Move:           move,
		Result:         r.currentRound.results[userID],
	}

	// 对手的具体拳型只有结算完成后才能写入玩家视图。
	if r.currentRound.settled {
		for submittedUserID, submittedMove := range r.currentRound.moves {
			if submittedUserID != userID {
				playerRound.OpponentMove = submittedMove
				break
			}
		}
	}
	snapshot.Round = playerRound

	return snapshot, nil
}
