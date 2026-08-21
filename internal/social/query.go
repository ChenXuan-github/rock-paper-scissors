package social

import (
	"context"
	"fmt"
)

// UserSummary 是社交模块可以对外使用的安全用户摘要，不包含 PasswordHash。
// JSON tag 也是 WebSocket 线协议的一部分：没有 tag 时 Go 会输出 ID/Username，
// 与前端约定的 id/username 不一致，运行时会读到 undefined。
type UserSummary struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

// FriendRequestDetail 把申请记录与双方用户摘要组合起来，供后续 Handler 返回给前端。
type FriendRequestDetail struct {
	Request   FriendRequest
	Requester UserSummary
	Receiver  UserSummary
}

// ListFriends 查询当前用户在好友图中的全部直接邻接节点。
// 它只返回直接好友，不会把“好友的好友”误当成当前用户的好友。
func (s *Service) ListFriends(ctx context.Context, currentUserID int64) ([]UserSummary, error) {
	if currentUserID <= 0 {
		return nil, ErrInvalidUserPair
	}
	if s.friendshipRepository == nil || s.users == nil {
		return nil, ErrSocialServiceNotInitialized
	}

	friendIDs, err := s.friendshipRepository.ListFriendIDs(ctx, currentUserID)
	if err != nil {
		return nil, fmt.Errorf("list friendship ids: %w", err)
	}
	if len(friendIDs) == 0 {
		// 返回空切片而不是 nil，使 JSON 稳定编码为 []，前端不需要额外判空。
		return []UserSummary{}, nil
	}

	// 用一条 WHERE id IN (...) 查询全部用户，避免好友数量增加后出现 N+1。
	usersByID, err := s.users.GetByIDs(ctx, friendIDs)
	if err != nil {
		return nil, fmt.Errorf("get friend users: %w", err)
	}
	// 预分配容量避免 append 过程中重复扩容；长度仍为 0，由循环逐项追加。
	friends := make([]UserSummary, 0, len(friendIDs))
	for _, friendID := range friendIDs {
		foundUser, exists := usersByID[friendID]
		if !exists {
			return nil, fmt.Errorf("friend user %d: %w", friendID, ErrSocialUserNotFound)
		}
		friends = append(friends, UserSummary{ID: foundUser.ID, Username: foundUser.Username})
	}
	return friends, nil
}

// ListIncomingFriendRequests 查询当前用户收到的指定状态申请。
func (s *Service) ListIncomingFriendRequests(
	ctx context.Context,
	currentUserID int64,
	status FriendRequestStatus,
	limit, offset int,
) ([]FriendRequestDetail, error) {
	if s.friendRequestRepository == nil || s.users == nil {
		return nil, ErrSocialServiceNotInitialized
	}
	requests, err := s.friendRequestRepository.ListIncoming(ctx, currentUserID, status, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list incoming friend requests: %w", err)
	}
	return s.buildFriendRequestDetails(ctx, requests)
}

// ListOutgoingFriendRequests 查询当前用户发出的指定状态申请。
func (s *Service) ListOutgoingFriendRequests(
	ctx context.Context,
	currentUserID int64,
	status FriendRequestStatus,
	limit, offset int,
) ([]FriendRequestDetail, error) {
	if s.friendRequestRepository == nil || s.users == nil {
		return nil, ErrSocialServiceNotInitialized
	}
	requests, err := s.friendRequestRepository.ListOutgoing(ctx, currentUserID, status, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list outgoing friend requests: %w", err)
	}
	return s.buildFriendRequestDetails(ctx, requests)
}

// buildFriendRequestDetails 收集申请双方的去重用户 ID，并批量查询一次用户表。
func (s *Service) buildFriendRequestDetails(
	ctx context.Context,
	requests []FriendRequest,
) ([]FriendRequestDetail, error) {
	if len(requests) == 0 {
		return []FriendRequestDetail{}, nil
	}

	// map[int64]struct{} 充当 Set：一名用户同时出现在多条申请中也只查询一次。
	uniqueIDs := make(map[int64]struct{}, len(requests)*2)
	for _, request := range requests {
		uniqueIDs[request.RequesterID] = struct{}{}
		uniqueIDs[request.ReceiverID] = struct{}{}
	}
	userIDs := make([]int64, 0, len(uniqueIDs))
	for userID := range uniqueIDs {
		userIDs = append(userIDs, userID)
	}

	// 一条 WHERE id IN (...) 获取全部用户，避免每条申请分别查两次 users 表。
	usersByID, err := s.users.GetByIDs(ctx, userIDs)
	if err != nil {
		return nil, fmt.Errorf("get friend request users: %w", err)
	}
	// usersByID 是批量查询结果的索引，后续组装时按 ID 近似 O(1) 查找。
	details := make([]FriendRequestDetail, 0, len(requests))
	for _, request := range requests {
		requester, requesterExists := usersByID[request.RequesterID]
		receiver, receiverExists := usersByID[request.ReceiverID]
		if !requesterExists || !receiverExists {
			return nil, ErrSocialUserNotFound
		}
		details = append(details, FriendRequestDetail{
			Request: request,
			Requester: UserSummary{
				ID:       requester.ID,
				Username: requester.Username,
			},
			Receiver: UserSummary{
				ID:       receiver.ID,
				Username: receiver.Username,
			},
		})
	}
	return details, nil
}
