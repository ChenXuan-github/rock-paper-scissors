package matchmaking

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/game"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/realtime"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/user"
)

type memoryQueue struct {
	entries map[int64]QueueEntry
}

func newMemoryQueue() *memoryQueue {
	return &memoryQueue{entries: make(map[int64]QueueEntry)}
}

func (q *memoryQueue) Enqueue(_ context.Context, entry QueueEntry) error {
	if _, exists := q.entries[entry.UserID]; exists {
		return ErrAlreadyQueued
	}
	q.entries[entry.UserID] = entry
	return nil
}

func (q *memoryQueue) Requeue(_ context.Context, entries ...QueueEntry) error {
	for _, entry := range entries {
		if _, exists := q.entries[entry.UserID]; !exists {
			q.entries[entry.UserID] = entry
		}
	}
	return nil
}

func (q *memoryQueue) Remove(_ context.Context, userID int64) (bool, error) {
	if _, exists := q.entries[userID]; !exists {
		return false, nil
	}
	delete(q.entries, userID)
	return true, nil
}

func (q *memoryQueue) Position(_ context.Context, userID int64) (int, bool, error) {
	entries := q.sortedEntries()
	for index, entry := range entries {
		if entry.UserID == userID {
			return index + 1, true, nil
		}
	}
	return 0, false, nil
}

func (q *memoryQueue) PopPair(_ context.Context) ([]QueueEntry, error) {
	entries := q.sortedEntries()
	if len(entries) < 2 {
		return []QueueEntry{}, nil
	}
	pair := append([]QueueEntry(nil), entries[:2]...)
	delete(q.entries, pair[0].UserID)
	delete(q.entries, pair[1].UserID)
	return pair, nil
}

func (q *memoryQueue) Clear(context.Context) error {
	q.entries = make(map[int64]QueueEntry)
	return nil
}

func (q *memoryQueue) sortedEntries() []QueueEntry {
	entries := make([]QueueEntry, 0, len(q.entries))
	for _, entry := range q.entries {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].EnqueuedAt.Equal(entries[j].EnqueuedAt) {
			return entries[i].UserID < entries[j].UserID
		}
		return entries[i].EnqueuedAt.Before(entries[j].EnqueuedAt)
	})
	return entries
}

type matchmakingUserService struct {
	users map[int64]user.User
}

func (s matchmakingUserService) GetByID(_ context.Context, id int64) (user.User, error) {
	foundUser, exists := s.users[id]
	if !exists {
		return user.User{}, user.ErrUserNotFound
	}
	return foundUser, nil
}

type matchmakingRealtime struct {
	online map[int64]bool
	events map[int64][]realtime.Event
}

func (r *matchmakingRealtime) IsOnline(userID int64) bool {
	return r.online[userID]
}

func (r *matchmakingRealtime) SendToUser(userID int64, event realtime.Event) error {
	if !r.online[userID] {
		return realtime.ErrUserOffline
	}
	r.events[userID] = append(r.events[userID], event)
	return nil
}

func TestServiceMatchesFirstTwoOnlinePlayers(t *testing.T) {
	queue := newMemoryQueue()
	roomService := game.NewRoomService(game.NewRoomManager())
	realtimeGateway := &matchmakingRealtime{
		online: map[int64]bool{1: true, 2: true},
		events: make(map[int64][]realtime.Event),
	}
	service := NewService(
		queue,
		roomService,
		matchmakingUserService{users: map[int64]user.User{
			1: {ID: 1, Username: "first"},
			2: {ID: 2, Username: "second"},
		}},
		realtimeGateway,
	)

	firstState, err := service.Join(context.Background(), 1)
	if err != nil {
		t.Fatalf("first Join() error = %v", err)
	}
	if firstState.Status != StateWaiting || firstState.Position != 1 {
		t.Fatalf("first state = %#v, want waiting position 1", firstState)
	}
	// 保证两次入队的毫秒时间戳不同，使测试中的先进先后关系明确。
	time.Sleep(time.Millisecond)

	secondState, err := service.Join(context.Background(), 2)
	if err != nil {
		t.Fatalf("second Join() error = %v", err)
	}
	if secondState.Status != StateMatched || secondState.RoomID == "" {
		t.Fatalf("second state = %#v, want matched room", secondState)
	}

	firstCurrent, err := service.Current(context.Background(), 1)
	if err != nil {
		t.Fatalf("first Current() error = %v", err)
	}
	if firstCurrent.Status != StateMatched || firstCurrent.RoomID != secondState.RoomID {
		t.Fatalf("first current = %#v, want room %q", firstCurrent, secondState.RoomID)
	}
	roomSnapshot, err := roomService.GetCurrentRoom(1)
	if err != nil {
		t.Fatalf("GetCurrentRoom(1) error = %v", err)
	}
	if len(roomSnapshot.Room.Players) != 2 || roomSnapshot.Room.Status != game.RoomStatusReady {
		t.Fatalf("matched room = %#v, want two ready players", roomSnapshot.Room)
	}
	for _, userID := range []int64{1, 2} {
		if len(realtimeGateway.events[userID]) != 1 || realtimeGateway.events[userID][0].Type != "match_found" {
			t.Fatalf("events for user %d = %#v, want one match_found", userID, realtimeGateway.events[userID])
		}
	}
}

func TestServiceRejectsOfflinePlayer(t *testing.T) {
	service := NewService(
		newMemoryQueue(),
		game.NewRoomService(game.NewRoomManager()),
		matchmakingUserService{},
		&matchmakingRealtime{online: map[int64]bool{}, events: make(map[int64][]realtime.Event)},
	)

	_, err := service.Join(context.Background(), 1)
	if !errors.Is(err, ErrPlayerOffline) {
		t.Fatalf("Join() error = %v, want %v", err, ErrPlayerOffline)
	}
}

func TestServiceCancelAndCurrent(t *testing.T) {
	queue := newMemoryQueue()
	queue.entries[7] = QueueEntry{UserID: 7, EnqueuedAt: time.Now()}
	service := NewService(
		queue,
		game.NewRoomService(game.NewRoomManager()),
		matchmakingUserService{},
		&matchmakingRealtime{online: map[int64]bool{7: true}, events: make(map[int64][]realtime.Event)},
	)

	if err := service.Cancel(context.Background(), 7); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	state, err := service.Current(context.Background(), 7)
	if err != nil {
		t.Fatalf("Current() error = %v", err)
	}
	if state.Status != StateIdle {
		t.Fatalf("state = %#v, want idle", state)
	}
	if err := service.Cancel(context.Background(), 7); !errors.Is(err, ErrNotQueued) {
		t.Fatalf("second Cancel() error = %v, want %v", err, ErrNotQueued)
	}
}
