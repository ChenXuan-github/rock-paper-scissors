package game

// Player 表示进入游戏流程后的玩家身份。
// 它是房间中的轻量运行时对象，只保留游戏逻辑当前需要的数据，
// 不直接持有 User 中的 PasswordHash、CreatedAt 等账号系统字段。
type Player struct {
	// UserID 关联 MySQL users 表的用户主键，也是玩家身份的唯一依据。
	UserID int64
	// Username 用于房间信息展示，是玩家进入房间时的用户名快照。
	Username string
}
