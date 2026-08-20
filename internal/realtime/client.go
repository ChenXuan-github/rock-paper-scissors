package realtime

import (
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	clientSendBufferSize = 16
	// CloseCodeSessionReplaced 是应用自定义关闭码：同账号在另一个客户端建立了新连接。
	CloseCodeSessionReplaced = 4001
	// writeWait 限制一帧消息写入客户端的最长时间。
	writeWait = 10 * time.Second
	// pongWait 是服务端在没有收到客户端 pong 时允许连接存活的时间。
	pongWait = 60 * time.Second
	// pingPeriod 必须小于 pongWait，确保超时前会主动探测连接。
	pingPeriod = pongWait * 9 / 10
	// maxMessageSize 防止客户端通过超大消息持续占用服务端内存。
	maxMessageSize = 4 * 1024
)

var (
	// ErrClientDisconnected 表示目标连接已经离线。
	ErrClientDisconnected = errors.New("websocket client is disconnected")
	// ErrClientSendQueueFull 表示客户端消费消息太慢，发送缓冲区已经写满。
	ErrClientSendQueueFull = errors.New("websocket client send queue is full")
)

// Event 是服务端通过 WebSocket 推送给客户端的统一消息外壳。
// Type 说明发生了什么，Data 保存该事件对应的具体数据。
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// Client 表示一名已登录用户与服务器之间的一条 WebSocket 连接。
type Client struct {
	UserID   int64
	Username string

	connection *websocket.Conn
	// send 是业务代码向该客户端投递消息的有缓冲 channel。
	send chan []byte
	// done 被关闭后，读写循环都会知道该连接已经结束。
	done      chan struct{}
	closeOnce sync.Once
}

// NewClient 把已经完成握手的底层连接包装成服务端可管理的客户端。
func NewClient(userID int64, username string, connection *websocket.Conn) *Client {
	return &Client{
		UserID:     userID,
		Username:   username,
		connection: connection,
		send:       make(chan []byte, clientSendBufferSize),
		done:       make(chan struct{}),
	}
}

// SendJSON 序列化消息并投递到发送队列，不直接并发写 WebSocket 连接。
func (c *Client) SendJSON(value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}

	select {
	case <-c.done:
		return ErrClientDisconnected
	case c.send <- payload:
		return nil
	default:
		return ErrClientSendQueueFull
	}
}

// WritePump 是该连接唯一的写协程，避免多个 goroutine 同时写同一 WebSocket。
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return
		case payload := <-c.send:
			if c.connection == nil {
				return
			}
			_ = c.connection.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.connection.WriteMessage(websocket.TextMessage, payload); err != nil {
				c.disconnect()
				return
			}
		case <-ticker.C:
			if c.connection == nil {
				return
			}
			_ = c.connection.SetWriteDeadline(time.Now().Add(writeWait))
			// 浏览器会自动回复 pong；ReadPump 的 PongHandler 收到后延长读期限。
			if err := c.connection.WriteMessage(websocket.PingMessage, nil); err != nil {
				c.disconnect()
				return
			}
		}
	}
}

// ReadPump 持续读取客户端消息；当前阶段只用它感知连接关闭。
func (c *Client) ReadPump() {
	if c.connection == nil {
		return
	}
	c.connection.SetReadLimit(maxMessageSize)
	_ = c.connection.SetReadDeadline(time.Now().Add(pongWait))
	c.connection.SetPongHandler(func(string) error {
		return c.connection.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		if _, _, err := c.connection.ReadMessage(); err != nil {
			return
		}
	}
}

// disconnect 只允许 Hub 和 Client 内部调用，保证资源只关闭一次。
func (c *Client) disconnect() {
	c.disconnectWithCode(websocket.CloseNormalClosure, "connection closed")
}

// disconnectSessionReplaced 明确通知旧客户端它已被新登录挤下线。
func (c *Client) disconnectSessionReplaced() {
	c.disconnectWithCode(CloseCodeSessionReplaced, "session replaced")
}

func (c *Client) disconnectWithCode(code int, reason string) {
	c.closeOnce.Do(func() {
		close(c.done)
		if c.connection != nil {
			// WriteControl 可以与当前写协程并发调用，用关闭帧把原因传给浏览器。
			_ = c.connection.WriteControl(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(code, reason),
				time.Now().Add(writeWait),
			)
			_ = c.connection.Close()
		}
	})
}
