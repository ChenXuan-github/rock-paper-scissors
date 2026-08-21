package social

import "time"

// FriendRequestStatus 表示好友申请当前所处的生命周期状态。
type FriendRequestStatus string

const (
	// FriendRequestPending 表示接收者尚未处理申请。
	FriendRequestPending FriendRequestStatus = "pending"
	// FriendRequestAccepted 表示接收者同意申请，双方已经成为好友。
	FriendRequestAccepted FriendRequestStatus = "accepted"
	// FriendRequestRejected 表示接收者明确拒绝了申请。
	FriendRequestRejected FriendRequestStatus = "rejected"
	// FriendRequestCancelled 表示发送者在处理前主动撤回申请。
	FriendRequestCancelled FriendRequestStatus = "cancelled"
)

// Valid 判断状态是否属于数据库 CHECK 约束允许的四种值。
func (s FriendRequestStatus) Valid() bool {
	switch s {
	case FriendRequestPending, FriendRequestAccepted, FriendRequestRejected, FriendRequestCancelled:
		return true
	default:
		return false
	}
}

// FriendRequest 表示一条有方向的好友申请。
// RequesterID 是发送者，ReceiverID 是接收者；PairUserIDLow/High 由 MySQL 生成列计算。
type FriendRequest struct {
	// ID 是申请生命周期记录的主键；同一对用户重新申请时会复用这条记录。
	ID int64
	// RequesterID/ReceiverID 保留申请方向，决定谁能撤销、谁能接受或拒绝。
	RequesterID int64
	ReceiverID  int64
	// PairUserIDLow/High 忽略方向，用于把 A→B 与 B→A 识别为同一对用户。
	PairUserIDLow  int64
	PairUserIDHigh int64
	// Status 控制申请从 pending 向 accepted/rejected/cancelled 的单向流转。
	Status    FriendRequestStatus
	CreatedAt time.Time
	UpdatedAt time.Time
	// RespondedAt 只有接收者接受或拒绝时才有值；发送者撤销时保持 nil。
	RespondedAt *time.Time
}

// NewFriendRequest 创建一条待处理申请；规范化用户对仅用于判重，不会改变申请方向。
func NewFriendRequest(requesterID, receiverID int64) (FriendRequest, error) {
	// 先规范化用户对，既能拒绝“自己加自己”，也为后续双向判重准备 low/high。
	low, high, err := canonicalUserPair(requesterID, receiverID)
	if err != nil {
		return FriendRequest{}, err
	}
	return FriendRequest{
		RequesterID:    requesterID,
		ReceiverID:     receiverID,
		PairUserIDLow:  low,
		PairUserIDHigh: high,
		Status:         FriendRequestPending,
	}, nil
}
