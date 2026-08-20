package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestHubRegistersAndUnregistersClient(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub := NewHub()
	go hub.Run(ctx)

	client := NewClient(7, "chenxuan", nil)
	hub.Register(client)
	if !hub.IsOnline(7) {
		t.Fatal("user 7 should be online after Register")
	}

	hub.Unregister(client)
	if hub.IsOnline(7) {
		t.Fatal("user 7 should be offline after Unregister")
	}
}

func TestHubKeepsNewConnectionWhenOldConnectionUnregisters(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub := NewHub()
	go hub.Run(ctx)

	oldClient := NewClient(7, "chenxuan", nil)
	newClient := NewClient(7, "chenxuan", nil)
	hub.Register(oldClient)
	hub.Register(newClient)

	// 被替换的旧连接随后退出时，不能删除同一用户的新连接。
	hub.Unregister(oldClient)
	if !hub.IsOnline(7) {
		t.Fatal("new connection should remain online")
	}
}

func TestHubSendsEventToSpecifiedUser(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub := NewHub()
	go hub.Run(ctx)

	client := NewClient(7, "chenxuan", nil)
	hub.Register(client)

	event := Event{
		Type: "round_settled",
		Data: map[string]string{"result": "win"},
	}
	if err := hub.SendToUser(7, event); err != nil {
		t.Fatalf("SendToUser() error = %v", err)
	}

	// 不启动 WritePump，直接从发送 channel 读取，验证 Hub 投递给了正确的 Client。
	payload := <-client.send
	var received Event
	if err := json.Unmarshal(payload, &received); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if received.Type != "round_settled" {
		t.Fatalf("event type = %q, want round_settled", received.Type)
	}
}

func TestHubSendToUserRejectsOfflineUser(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub := NewHub()
	go hub.Run(ctx)

	if err := hub.SendToUser(99, Event{Type: "test"}); !errors.Is(err, ErrUserOffline) {
		t.Fatalf("SendToUser() error = %v, want %v", err, ErrUserOffline)
	}
}
