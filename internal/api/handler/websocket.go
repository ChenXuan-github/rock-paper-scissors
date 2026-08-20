package handler

import (
	"log"
	"net/http"
	"strings"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/api/middleware"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/api/response"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/realtime"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// WebSocketHandler 负责 WebSocket 握手与连接生命周期。
// 当前先完成最小连接闭环；后续 Hub 会负责登记连接和主动推送房间事件。
type WebSocketHandler struct {
	tokenVerifier middleware.TokenVerifier
	hub           *realtime.Hub
	upgrader      websocket.Upgrader
}

// NewWebSocketHandler 创建 WebSocket 接口处理器。
func NewWebSocketHandler(tokenVerifier middleware.TokenVerifier, hub *realtime.Hub) *WebSocketHandler {
	return &WebSocketHandler{
		tokenVerifier: tokenVerifier,
		hub:           hub,
		upgrader: websocket.Upgrader{
			// H5 开发服务器和 Go API 使用不同端口，浏览器会把它们视为不同 Origin。
			// Day5 本地联调阶段先允许跨 Origin；正式部署时应改成明确的域名白名单。
			CheckOrigin: func(_ *http.Request) bool { return true },
		},
	}
}

type connectedEventData struct {
	UserID   int64  `json:"userId"`
	Username string `json:"username"`
}

// Connect 校验查询参数中的 JWT，把 HTTP 请求升级为 WebSocket 长连接。
// 浏览器原生 WebSocket API 不能自定义 Authorization Header，因此 Demo 暂时使用 ?token=JWT。
func (h *WebSocketHandler) Connect(c *gin.Context) {
	// HTTP 升级前先完成身份校验；校验失败时连接不会建立。
	signedToken := strings.TrimSpace(c.Query("token"))
	if signedToken == "" || h.tokenVerifier == nil || h.hub == nil {
		c.JSON(http.StatusUnauthorized, response.Error(
			http.StatusUnauthorized,
			"invalid or missing access token",
		))
		return
	}

	claims, err := h.tokenVerifier.Parse(signedToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, response.Error(
			http.StatusUnauthorized,
			"invalid or missing access token",
		))
		return
	}

	// Upgrade 成功后，后续通信不再是一问一答的 HTTP，而是在同一条连接上传递消息帧。
	connection, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("upgrade websocket: %v", err)
		return
	}

	// JWT 中的可信身份与底层连接共同组成 Client，并登记到全局 Hub。
	client := realtime.NewClient(claims.UserID, claims.Username, connection)
	h.hub.Register(client)
	defer h.hub.Unregister(client)

	// 每条连接只有这个写 goroutine 可以操作 WebSocket 的写方向。
	go client.WritePump()

	// Handler 不再直接写 connection，而是把连接确认事件放进 Client 的发送 channel。
	if err := client.SendJSON(realtime.Event{
		Type: "connected",
		Data: connectedEventData{
			UserID:   claims.UserID,
			Username: claims.Username,
		},
	}); err != nil {
		return
	}

	// 当前 Gin 请求 goroutine 负责读方向；客户端断开后返回并触发上面的 Unregister。
	client.ReadPump()
}
