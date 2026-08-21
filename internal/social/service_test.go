package social

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/user"
)

type socialTestUsers struct {
	users         map[int64]user.User
	getByIDsCalls int
}

func (s *socialTestUsers) GetByIDs(_ context.Context, ids []int64) (map[int64]user.User, error) {
	s.getByIDsCalls++
	result := make(map[int64]user.User, len(ids))
	for _, id := range ids {
		if found, exists := s.users[id]; exists {
			result[id] = found
		}
	}
	return result, nil
}

func (s socialTestUsers) GetByID(_ context.Context, id int64) (user.User, error) {
	found, exists := s.users[id]
	if !exists {
		return user.User{}, user.ErrUserNotFound
	}
	return found, nil
}

type socialTestFriendships struct {
	exists      bool
	friendIDs   []int64
	deleteFound bool
	deletedLow  int64
	deletedHigh int64
}

func (s *socialTestFriendships) Create(_ context.Context, friendship Friendship) (Friendship, error) {
	return friendship, nil
}
func (s *socialTestFriendships) Delete(_ context.Context, firstUserID, secondUserID int64) (bool, error) {
	low, high, err := canonicalUserPair(firstUserID, secondUserID)
	if err != nil {
		return false, err
	}
	s.deletedLow = low
	s.deletedHigh = high
	return s.deleteFound, nil
}
func (s *socialTestFriendships) Exists(_ context.Context, _, _ int64) (bool, error) {
	return s.exists, nil
}
func (s *socialTestFriendships) ListFriendIDs(_ context.Context, _ int64) ([]int64, error) {
	return s.friendIDs, nil
}

type socialTestFriendRequests struct {
	existing *FriendRequest
	created  FriendRequest
	reopened FriendRequest
	updated  FriendRequest
	incoming []FriendRequest
	outgoing []FriendRequest
}

func (s *socialTestFriendRequests) Create(_ context.Context, request FriendRequest) (FriendRequest, error) {
	request.ID = 1
	s.created = request
	return request, nil
}
func (s *socialTestFriendRequests) Reopen(
	_ context.Context,
	requestID, requesterID, receiverID int64,
) (FriendRequest, error) {
	request, err := NewFriendRequest(requesterID, receiverID)
	if err != nil {
		return FriendRequest{}, err
	}
	request.ID = requestID
	s.reopened = request
	return request, nil
}
func (s *socialTestFriendRequests) FindByID(_ context.Context, requestID int64) (FriendRequest, error) {
	if s.existing == nil || s.existing.ID != requestID {
		return FriendRequest{}, ErrFriendRequestNotFound
	}
	return *s.existing, nil
}
func (s *socialTestFriendRequests) FindByPair(_ context.Context, _, _ int64) (FriendRequest, error) {
	if s.existing == nil {
		return FriendRequest{}, ErrFriendRequestNotFound
	}
	return *s.existing, nil
}
func (s *socialTestFriendRequests) ListIncoming(
	_ context.Context,
	_ int64,
	_ FriendRequestStatus,
	_, _ int,
) ([]FriendRequest, error) {
	return s.incoming, nil
}
func (s *socialTestFriendRequests) ListOutgoing(
	_ context.Context,
	_ int64,
	_ FriendRequestStatus,
	_, _ int,
) ([]FriendRequest, error) {
	return s.outgoing, nil
}
func (s *socialTestFriendRequests) UpdateStatus(
	_ context.Context,
	requestID int64,
	expectedStatus, nextStatus FriendRequestStatus,
	respondedAt *time.Time,
) (FriendRequest, error) {
	if s.existing == nil || s.existing.ID != requestID {
		return FriendRequest{}, ErrFriendRequestNotFound
	}
	if s.existing.Status != expectedStatus {
		return FriendRequest{}, ErrFriendRequestStateChanged
	}
	updated := *s.existing
	updated.Status = nextStatus
	updated.RespondedAt = respondedAt
	s.updated = updated
	return updated, nil
}

func newSocialTestService(
	friendships *socialTestFriendships,
	requests *socialTestFriendRequests,
) *Service {
	return &Service{
		friendshipRepository:    friendships,
		friendRequestRepository: requests,
		users: &socialTestUsers{users: map[int64]user.User{
			1: {ID: 1, Username: "requester"},
			2: {ID: 2, Username: "receiver"},
		}},
	}
}

func TestListFriendsUsesOneBatchUserQuery(t *testing.T) {
	friendships := &socialTestFriendships{friendIDs: []int64{2, 3}}
	users := &socialTestUsers{users: map[int64]user.User{
		2: {ID: 2, Username: "friend-2"},
		3: {ID: 3, Username: "friend-3"},
	}}
	service := &Service{
		friendshipRepository:    friendships,
		friendRequestRepository: &socialTestFriendRequests{},
		users:                   users,
	}

	friends, err := service.ListFriends(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(friends) != 2 || friends[0].ID != 2 || friends[1].ID != 3 {
		t.Fatalf("friends = %+v", friends)
	}
	if users.getByIDsCalls != 1 {
		t.Fatalf("GetByIDs calls = %d, want 1", users.getByIDsCalls)
	}
}

func TestListIncomingFriendRequestsBatchLoadsBothUsers(t *testing.T) {
	requests := &socialTestFriendRequests{incoming: []FriendRequest{
		{ID: 12, RequesterID: 1, ReceiverID: 2, Status: FriendRequestPending},
	}}
	users := &socialTestUsers{users: map[int64]user.User{
		1: {ID: 1, Username: "requester"},
		2: {ID: 2, Username: "receiver"},
	}}
	service := &Service{
		friendshipRepository:    &socialTestFriendships{},
		friendRequestRepository: requests,
		users:                   users,
	}

	details, err := service.ListIncomingFriendRequests(
		context.Background(),
		2,
		FriendRequestPending,
		20,
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(details) != 1 || details[0].Requester.Username != "requester" {
		t.Fatalf("details = %+v", details)
	}
	if users.getByIDsCalls != 1 {
		t.Fatalf("GetByIDs calls = %d, want 1", users.getByIDsCalls)
	}
}

func TestSendFriendRequestCreatesNewRequest(t *testing.T) {
	requests := &socialTestFriendRequests{}
	service := newSocialTestService(&socialTestFriendships{}, requests)

	created, err := service.SendFriendRequest(context.Background(), 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != 1 || created.RequesterID != 1 || created.ReceiverID != 2 {
		t.Fatalf("created request = %+v", created)
	}
}

func TestSendFriendRequestRejectsExistingFriendship(t *testing.T) {
	service := newSocialTestService(
		&socialTestFriendships{exists: true},
		&socialTestFriendRequests{},
	)

	_, err := service.SendFriendRequest(context.Background(), 1, 2)
	if !errors.Is(err, ErrAlreadyFriends) {
		t.Fatalf("error = %v, want ErrAlreadyFriends", err)
	}
}

func TestSendFriendRequestRejectsPendingPairInEitherDirection(t *testing.T) {
	requests := &socialTestFriendRequests{existing: &FriendRequest{
		ID:          8,
		RequesterID: 2,
		ReceiverID:  1,
		Status:      FriendRequestPending,
	}}
	service := newSocialTestService(&socialTestFriendships{}, requests)

	_, err := service.SendFriendRequest(context.Background(), 1, 2)
	if !errors.Is(err, ErrFriendRequestPending) {
		t.Fatalf("error = %v, want ErrFriendRequestPending", err)
	}
}

func TestSendFriendRequestReopensFinishedPairWithNewDirection(t *testing.T) {
	requests := &socialTestFriendRequests{existing: &FriendRequest{
		ID:          9,
		RequesterID: 2,
		ReceiverID:  1,
		Status:      FriendRequestRejected,
	}}
	service := newSocialTestService(&socialTestFriendships{}, requests)

	reopened, err := service.SendFriendRequest(context.Background(), 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.ID != 9 || reopened.RequesterID != 1 || reopened.ReceiverID != 2 {
		t.Fatalf("reopened request = %+v", reopened)
	}
	if reopened.Status != FriendRequestPending {
		t.Fatalf("status = %q, want pending", reopened.Status)
	}
}

func TestRejectFriendRequestRequiresReceiverAndRecordsResponseTime(t *testing.T) {
	requests := &socialTestFriendRequests{existing: &FriendRequest{
		ID:          10,
		RequesterID: 1,
		ReceiverID:  2,
		Status:      FriendRequestPending,
	}}
	service := newSocialTestService(&socialTestFriendships{}, requests)

	if _, err := service.RejectFriendRequest(context.Background(), 10, 1); !errors.Is(err, ErrFriendRequestNotReceiver) {
		t.Fatalf("sender rejection error = %v, want ErrFriendRequestNotReceiver", err)
	}
	rejected, err := service.RejectFriendRequest(context.Background(), 10, 2)
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Status != FriendRequestRejected || rejected.RespondedAt == nil {
		t.Fatalf("rejected request = %+v", rejected)
	}
}

func TestCancelFriendRequestRequiresRequesterAndKeepsResponseTimeEmpty(t *testing.T) {
	requests := &socialTestFriendRequests{existing: &FriendRequest{
		ID:          11,
		RequesterID: 1,
		ReceiverID:  2,
		Status:      FriendRequestPending,
	}}
	service := newSocialTestService(&socialTestFriendships{}, requests)

	if _, err := service.CancelFriendRequest(context.Background(), 11, 2); !errors.Is(err, ErrFriendRequestNotRequester) {
		t.Fatalf("receiver cancellation error = %v, want ErrFriendRequestNotRequester", err)
	}
	cancelled, err := service.CancelFriendRequest(context.Background(), 11, 1)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != FriendRequestCancelled || cancelled.RespondedAt != nil {
		t.Fatalf("cancelled request = %+v", cancelled)
	}
}

func TestRemoveFriendDeletesCanonicalUndirectedEdge(t *testing.T) {
	friendships := &socialTestFriendships{deleteFound: true}
	service := newSocialTestService(friendships, &socialTestFriendRequests{})

	if err := service.RemoveFriend(context.Background(), 2, 1); err != nil {
		t.Fatal(err)
	}
	if friendships.deletedLow != 1 || friendships.deletedHigh != 2 {
		t.Fatalf("deleted edge = %d-%d, want 1-2", friendships.deletedLow, friendships.deletedHigh)
	}
}

func TestRemoveFriendReturnsNotFoundWhenEdgeDoesNotExist(t *testing.T) {
	service := newSocialTestService(&socialTestFriendships{}, &socialTestFriendRequests{})

	err := service.RemoveFriend(context.Background(), 1, 2)
	if !errors.Is(err, ErrFriendshipNotFound) {
		t.Fatalf("error = %v, want ErrFriendshipNotFound", err)
	}
}
