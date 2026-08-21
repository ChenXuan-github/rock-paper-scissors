package social

import (
	"errors"
	"testing"
)

func TestNewFriendshipCanonicalizesUndirectedEdge(t *testing.T) {
	friendship, err := NewFriendship(9, 3)
	if err != nil {
		t.Fatal(err)
	}
	if friendship.UserIDLow != 3 || friendship.UserIDHigh != 9 {
		t.Fatalf("friendship = %+v, want edge 3-9", friendship)
	}
}

func TestNewFriendshipRejectsSelfFriendship(t *testing.T) {
	_, err := NewFriendship(7, 7)
	if !errors.Is(err, ErrInvalidUserPair) {
		t.Fatalf("error = %v, want ErrInvalidUserPair", err)
	}
}

func TestNewFriendRequestPreservesDirectionAndCanonicalizesPair(t *testing.T) {
	request, err := NewFriendRequest(9, 3)
	if err != nil {
		t.Fatal(err)
	}
	if request.RequesterID != 9 || request.ReceiverID != 3 {
		t.Fatalf("request direction = %d -> %d, want 9 -> 3", request.RequesterID, request.ReceiverID)
	}
	if request.PairUserIDLow != 3 || request.PairUserIDHigh != 9 {
		t.Fatalf("request pair = %d-%d, want 3-9", request.PairUserIDLow, request.PairUserIDHigh)
	}
	if request.Status != FriendRequestPending {
		t.Fatalf("status = %q, want pending", request.Status)
	}
}

func TestFriendRequestStatusValid(t *testing.T) {
	for _, status := range []FriendRequestStatus{
		FriendRequestPending,
		FriendRequestAccepted,
		FriendRequestRejected,
		FriendRequestCancelled,
	} {
		if !status.Valid() {
			t.Fatalf("status %q should be valid", status)
		}
	}
	if FriendRequestStatus("unknown").Valid() {
		t.Fatal("unknown status should be invalid")
	}
}
