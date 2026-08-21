package social

import "time"

// GameInvitationStatus 表示一次短期好友对战邀请的状态。
type GameInvitationStatus string

const (
	// GameInvitationPending 表示邀请仍在有效期内，等待被邀请者处理。
	GameInvitationPending GameInvitationStatus = "pending"
	// GameInvitationAccepted 表示被邀请者已同意，并且双方房间已经创建成功。
	GameInvitationAccepted GameInvitationStatus = "accepted"
	// GameInvitationRejected 表示被邀请者主动拒绝。
	GameInvitationRejected GameInvitationStatus = "rejected"
	// GameInvitationCancelled 表示邀请者在处理前主动撤销。
	GameInvitationCancelled GameInvitationStatus = "cancelled"
	// GameInvitationExpired 表示超过 TTL 后自动失效。
	GameInvitationExpired GameInvitationStatus = "expired"
)

// GameInvitation 是好友之间的一次临时对战邀请。
// 它只保存在当前进程内存中，过期或服务重启后无需恢复。
type GameInvitation struct {
	// ID 是使用 crypto/rand 生成的不可预测标识，供后续接受、拒绝和撤销接口定位邀请。
	ID string
	// 用户名快照用于实时消息和建房，避免处理短期邀请时再次查询用户表。
	InviterID       int64
	InviterUsername string
	InviteeID       int64
	InviteeUsername string
	// Status 表示这条短期邀请当前所处的生命周期阶段。
	Status GameInvitationStatus
	// RoomID 仅在 accepted 后写入，双方客户端用它确认进入的是同一个房间。
	RoomID    string
	CreatedAt time.Time
	ExpiresAt time.Time
	// RespondedAt 记录接受、拒绝、撤销或过期的终止时刻；pending 时为 nil。
	RespondedAt *time.Time
}
