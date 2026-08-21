package social

import "errors"

var (
	// ErrInvalidUserPair 表示用户 ID 非法，或者试图与自己建立关系。
	ErrInvalidUserPair = errors.New("invalid social user pair")
	// ErrFriendshipAlreadyExists 表示这条无向好友边已经存在。
	ErrFriendshipAlreadyExists = errors.New("friendship already exists")
	// ErrFriendshipNotFound 表示两个用户当前不是好友。
	ErrFriendshipNotFound = errors.New("friendship not found")
	// ErrFriendRequestAlreadyExists 表示这一对用户已经存在申请生命周期记录。
	ErrFriendRequestAlreadyExists = errors.New("friend request already exists")
	// ErrFriendRequestNotFound 表示指定好友申请不存在。
	ErrFriendRequestNotFound = errors.New("friend request not found")
	// ErrInvalidFriendRequestStatus 表示申请状态不属于约定的四种合法状态。
	ErrInvalidFriendRequestStatus = errors.New("invalid friend request status")
	// ErrInvalidFriendRequestPage 表示申请列表的 limit 或 offset 不合法。
	ErrInvalidFriendRequestPage = errors.New("invalid friend request page")
	// ErrFriendRequestStateChanged 表示申请已被其他并发操作处理，不能按预期状态继续更新。
	ErrFriendRequestStateChanged = errors.New("friend request state has changed")
	// ErrSocialUserNotFound 表示准备添加的目标用户不存在。
	ErrSocialUserNotFound = errors.New("social target user not found")
	// ErrAlreadyFriends 表示两个用户之间已经存在好友关系。
	ErrAlreadyFriends = errors.New("users are already friends")
	// ErrFriendRequestPending 表示两个用户之间已经存在尚未处理的申请。
	ErrFriendRequestPending = errors.New("friend request is already pending")
	// ErrFriendRequestNotReceiver 表示当前登录用户不是这条申请的接收者，无权处理它。
	ErrFriendRequestNotReceiver = errors.New("current user is not the friend request receiver")
	// ErrFriendRequestNotRequester 表示当前登录用户不是这条申请的发送者，无权撤销它。
	ErrFriendRequestNotRequester = errors.New("current user is not the friend request requester")
	// ErrFriendRequestNotPending 表示申请已经被接受、拒绝或撤销，不能再次处理。
	ErrFriendRequestNotPending = errors.New("friend request is not pending")
	// ErrSocialServiceNotInitialized 表示社交服务缺少数据库或必要依赖。
	ErrSocialServiceNotInitialized = errors.New("social service dependencies are not initialized")
)
