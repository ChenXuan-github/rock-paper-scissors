package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/api/middleware"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/auth"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/game"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/realtime"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// realtimeFlowTokenVerifier 用两段固定测试令牌模拟两名不同的登录用户。
type realtimeFlowTokenVerifier struct{}

func (realtimeFlowTokenVerifier) Parse(token string) (auth.Claims, error) {
	switch token {
	case "host-token":
		return auth.Claims{UserID: 1, Username: "host"}, nil
	case "guest-token":
		return auth.Claims{UserID: 2, Username: "guest"}, nil
	default:
		return auth.Claims{}, auth.ErrInvalidToken
	}
}

type realtimeFlowEvent struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

func TestTwoWebSocketClientsReceiveMoveAndSettlementEvents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	verifier := realtimeFlowTokenVerifier{}

	// 准备一间已经开始的 1v1 房间，使测试聚焦于“HTTP 出拳 → WebSocket 推送”。
	manager := game.NewRoomManager()
	service := game.NewRoomService(manager)
	room, err := service.CreateRoom(&game.Player{UserID: 1, Username: "host"})
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	if _, err := service.JoinRoom(room.ID, &game.Player{UserID: 2, Username: "guest"}); err != nil {
		t.Fatalf("JoinRoom() error = %v", err)
	}
	if _, err := service.StartCurrentRoom(1); err != nil {
		t.Fatalf("StartCurrentRoom() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	hub := realtime.NewHub()
	go hub.Run(ctx)

	roomHandler := NewRoomHandler(service, hub, nil)
	webSocketHandler := NewWebSocketHandler(verifier, hub)
	router := gin.New()
	router.GET("/api/v1/ws", webSocketHandler.Connect)
	router.POST(
		"/api/v1/rooms/me/move",
		middleware.Authenticate(verifier),
		roomHandler.SubmitMove,
	)
	server := httptest.NewServer(router)

	hostConnection := dialRealtimeTestClient(t, server.URL, "host-token")
	guestConnection := dialRealtimeTestClient(t, server.URL, "guest-token")
	defer func() {
		_ = hostConnection.Close()
		_ = guestConnection.Close()
		server.Close()
		cancel()
	}()

	// 两条连接都先收到各自的 connected，证明 Hub 已按用户分别登记。
	if event := readRealtimeTestEvent(t, hostConnection); event.Type != "connected" {
		t.Fatalf("host first event = %q, want connected", event.Type)
	}
	if event := readRealtimeTestEvent(t, guestConnection); event.Type != "connected" {
		t.Fatalf("guest first event = %q, want connected", event.Type)
	}

	performRealtimeMove(t, server.URL, "host-token", "rock")
	moveEvent := readRealtimeTestEvent(t, guestConnection)
	if moveEvent.Type != "move_submitted" {
		t.Fatalf("guest event = %q, want move_submitted", moveEvent.Type)
	}
	var moveData moveSubmittedEventData
	if err := json.Unmarshal(moveEvent.Data, &moveData); err != nil {
		t.Fatal(err)
	}
	if moveData.SubmittedCount != 1 {
		t.Fatalf("submittedCount = %d, want 1", moveData.SubmittedCount)
	}
	// move_submitted 的结构没有 Move 字段，测试确保第一拳没有被提前泄露。
	if bytes.Contains(moveEvent.Data, []byte(`"move"`)) || bytes.Contains(moveEvent.Data, []byte("rock")) {
		t.Fatalf("move_submitted leaked move: %s", moveEvent.Data)
	}

	performRealtimeMove(t, server.URL, "guest-token", "scissors")
	hostSettled := readRealtimeTestEvent(t, hostConnection)
	guestSettled := readRealtimeTestEvent(t, guestConnection)
	assertRealtimeSettlement(t, hostSettled, "rock", "scissors", "win")
	assertRealtimeSettlement(t, guestSettled, "scissors", "rock", "lose")
}

func dialRealtimeTestClient(t *testing.T, serverURL, token string) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(serverURL, "http") + "/api/v1/ws?token=" + token
	connection, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("Dial(%s) error = %v", token, err)
	}
	return connection
}

func performRealtimeMove(t *testing.T, serverURL, token, move string) {
	t.Helper()
	request, err := http.NewRequest(
		http.MethodPost,
		serverURL+"/api/v1/rooms/me/move",
		strings.NewReader(fmt.Sprintf(`{"move":%q}`, move)),
	)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("move request error = %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("move HTTP status = %d, want %d", response.StatusCode, http.StatusOK)
	}
}

func readRealtimeTestEvent(t *testing.T, connection *websocket.Conn) realtimeFlowEvent {
	t.Helper()
	_ = connection.SetReadDeadline(time.Now().Add(2 * time.Second))
	var event realtimeFlowEvent
	if err := connection.ReadJSON(&event); err != nil {
		t.Fatalf("ReadJSON() error = %v", err)
	}
	return event
}

func assertRealtimeSettlement(
	t *testing.T,
	event realtimeFlowEvent,
	wantMove string,
	wantOpponentMove string,
	wantResult string,
) {
	t.Helper()
	if event.Type != "round_settled" {
		t.Fatalf("event type = %q, want round_settled", event.Type)
	}
	var data roundSettledEventData
	if err := json.Unmarshal(event.Data, &data); err != nil {
		t.Fatal(err)
	}
	if data.Move != wantMove || data.OpponentMove != wantOpponentMove || data.Result != wantResult {
		t.Fatalf("settlement = %#v", data)
	}
}
