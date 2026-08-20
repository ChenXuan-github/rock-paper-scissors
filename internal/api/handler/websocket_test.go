package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/auth"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/realtime"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type webSocketTestTokenVerifier struct{}

func (webSocketTestTokenVerifier) Parse(token string) (auth.Claims, error) {
	if token != "valid-token" {
		return auth.Claims{}, auth.ErrInvalidToken
	}
	return auth.Claims{UserID: 7, Username: "chenxuan"}, nil
}

func TestWebSocketConnectRejectsMissingToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hub := realtime.NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)
	h := NewWebSocketHandler(webSocketTestTokenVerifier{}, hub)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil)

	// 使用真实 Gin 路由执行，以验证 HTTP 响应状态。
	r := gin.New()
	r.GET("/api/v1/ws", h.Connect)
	r.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestWebSocketConnectReturnsConnectedEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hub := realtime.NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)
	h := NewWebSocketHandler(webSocketTestTokenVerifier{}, hub)
	r := gin.New()
	r.GET("/api/v1/ws", h.Connect)
	server := httptest.NewServer(r)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/ws?token=valid-token"
	connection, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer connection.Close()

	var event struct {
		Type string `json:"type"`
		Data struct {
			UserID   int64  `json:"userId"`
			Username string `json:"username"`
		} `json:"data"`
	}
	if err := connection.ReadJSON(&event); err != nil {
		t.Fatalf("ReadJSON() error = %v", err)
	}
	if event.Type != "connected" || event.Data.UserID != 7 || event.Data.Username != "chenxuan" {
		t.Fatalf("event = %+v", event)
	}
	if !hub.IsOnline(7) {
		t.Fatal("user 7 should be registered in Hub")
	}
}
