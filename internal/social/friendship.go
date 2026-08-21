// Package social 定义好友关系、好友申请以及后续社交业务所需的领域类型。
package social

import "time"

// Friendship 表示好友图中的一条无向边。
// UserIDLow 永远小于 UserIDHigh，因此 A-B 和 B-A 在内存与数据库中都是同一条关系。
type Friendship struct {
	// UserIDLow/UserIDHigh 共同构成联合主键；数据库中不会同时出现 A-B 与 B-A。
	UserIDLow  int64
	UserIDHigh int64
	// CreatedAt 由 MySQL 生成，表示双方正式成为好友的时间。
	CreatedAt time.Time
}

// NewFriendship 校验两个用户并按照“小 ID 在前”构造规范化的无向好友关系。
func NewFriendship(firstUserID, secondUserID int64) (Friendship, error) {
	low, high, err := canonicalUserPair(firstUserID, secondUserID)
	if err != nil {
		return Friendship{}, err
	}
	return Friendship{UserIDLow: low, UserIDHigh: high}, nil
}

// canonicalUserPair 把两个用户 ID 转换为无方向的稳定顺序，并拒绝无效 ID 和自己关联自己。
// 好友申请也需要使用同一规则判断两个方向的申请是否属于同一对用户。
func canonicalUserPair(firstUserID, secondUserID int64) (int64, int64, error) {
	// 用户 ID 必须有效，并且好友边和申请都不允许形成指向自己的自环。
	if firstUserID <= 0 || secondUserID <= 0 || firstUserID == secondUserID {
		return 0, 0, ErrInvalidUserPair
	}
	if firstUserID < secondUserID {
		return firstUserID, secondUserID, nil
	}
	// 调用方传入顺序不影响结果，因此 canonical(A, B) 永远等于 canonical(B, A)。
	return secondUserID, firstUserID, nil
}
