package realtime

import (
	"context"
	"errors"
)

// ErrUserOffline 表示指定用户当前没有可用的 WebSocket 连接。
var ErrUserOffline = errors.New("websocket user is offline")

type onlineRequest struct {
	userID int64
	result chan bool
}

type deliveryRequest struct {
	userID int64
	event  Event
	result chan error
}

// Hub 是全体 WebSocket 连接的管理中心。
// clients 只由 Run 所在的一个 goroutine 访问，因此不需要再给 map 加 Mutex。
type Hub struct {
	clients    map[int64]*Client
	register   chan *Client
	unregister chan *Client
	online     chan onlineRequest
	delivery   chan deliveryRequest
}

// NewHub 创建尚未启动的连接管理器；调用者还需要执行 go hub.Run(ctx)。
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[int64]*Client),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		online:     make(chan onlineRequest),
		delivery:   make(chan deliveryRequest),
	}
}

// Run 串行处理连接上线、下线和查询，是唯一允许修改 clients map 的地方。
func (h *Hub) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			for userID, client := range h.clients {
				client.disconnect()
				delete(h.clients, userID)
			}
			return

		case client := <-h.register:
			// Demo 规定一名用户只保留一条连接；重复登录时让新连接替换旧连接。
			if previous := h.clients[client.UserID]; previous != nil && previous != client {
				previous.disconnect()
			}
			h.clients[client.UserID] = client

		case client := <-h.unregister:
			// 旧连接晚到的下线通知不能误删同一用户后来建立的新连接。
			if current := h.clients[client.UserID]; current == client {
				delete(h.clients, client.UserID)
				client.disconnect()
			}

		case request := <-h.online:
			_, exists := h.clients[request.userID]
			request.result <- exists

		case request := <-h.delivery:
			client := h.clients[request.userID]
			if client == nil {
				request.result <- ErrUserOffline
				continue
			}

			// Hub 只负责找到目标连接；Client 负责序列化并写入自己的发送队列。
			request.result <- client.SendJSON(request.event)
		}
	}
}

// Register 把一条已完成握手的连接交给 Hub 管理。
func (h *Hub) Register(client *Client) {
	h.register <- client
}

// Unregister 通知 Hub 删除并关闭指定连接。
func (h *Hub) Unregister(client *Client) {
	h.unregister <- client
}

// IsOnline 查询指定用户当前是否存在 WebSocket 连接。
// 查询也通过 channel 进入 Run，避免在其他 goroutine 直接读取 clients map。
func (h *Hub) IsOnline(userID int64) bool {
	result := make(chan bool)
	h.online <- onlineRequest{userID: userID, result: result}
	return <-result
}

// SendToUser 根据用户 ID 找到其当前连接，并把事件投递到该 Client 的发送 channel。
// 调用者只面对用户身份和业务事件，不需要接触底层 WebSocket 连接。
func (h *Hub) SendToUser(userID int64, event Event) error {
	result := make(chan error)
	h.delivery <- deliveryRequest{
		userID: userID,
		event:  event,
		result: result,
	}
	return <-result
}
