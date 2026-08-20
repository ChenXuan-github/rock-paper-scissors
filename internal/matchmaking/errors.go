package matchmaking

import "errors"

var (
	// ErrInvalidUserID 表示匹配操作没有提供合法用户身份。
	ErrInvalidUserID = errors.New("invalid matchmaking user id")
	// ErrAlreadyQueued 表示同一玩家已经在匹配队列中。
	ErrAlreadyQueued = errors.New("player is already queued")
	// ErrNotQueued 表示玩家当前不在匹配队列中，无法取消。
	ErrNotQueued = errors.New("player is not queued")
	// ErrPlayerOffline 表示玩家尚未建立 WebSocket，无法接收匹配成功通知。
	ErrPlayerOffline = errors.New("player websocket is offline")
	// ErrPlayerAlreadyInRoom 表示玩家已经有房间，不能同时参加自动匹配。
	ErrPlayerAlreadyInRoom = errors.New("player is already in a room")
)
