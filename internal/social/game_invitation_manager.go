package social

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

const defaultGameInvitationTTL = 60 * time.Second

var (
	ErrInvalidGameInvitation    = errors.New("invalid game invitation")
	ErrGameInvitationNotFound   = errors.New("game invitation not found")
	ErrGameInvitationPending    = errors.New("game invitation is already pending")
	ErrGameInvitationNotInvitee = errors.New("current user is not the game invitation invitee")
	ErrGameInvitationNotInviter = errors.New("current user is not the game invitation inviter")
	ErrGameInvitationNotPending = errors.New("game invitation is not pending")
	ErrGameInvitationExpired    = errors.New("game invitation has expired")
)

// invitationUserPair 是内存 map 使用的无方向用户对键。
// low/high 沿用好友边的规范化规则，所以 A 邀请 B 与 B 邀请 A 会命中同一个键。
type invitationUserPair struct {
	low  int64
	high int64
}

// GameInvitationManager 并发安全地管理当前进程中的短期对战邀请。
type GameInvitationManager struct {
	// mu 同时保护 invitations 和 activePairs；一次业务修改必须让两张索引保持一致。
	mu sync.Mutex
	// ttl 决定 pending 邀请的有效期，生产默认一分钟，测试可以注入更短时间。
	ttl time.Duration
	// invitations 是主存储：invitationID -> 可变邀请对象。
	invitations map[string]*GameInvitation
	// activePairs 保证同一对好友在同一时刻最多存在一个 pending 邀请，不区分邀请方向。
	activePairs map[invitationUserPair]string
}

// NewGameInvitationManager 创建使用默认 60 秒有效期的邀请管理器。
func NewGameInvitationManager() *GameInvitationManager {
	return newGameInvitationManager(defaultGameInvitationTTL)
}

// newGameInvitationManager 允许单元测试注入很短的有效期。
func newGameInvitationManager(ttl time.Duration) *GameInvitationManager {
	return &GameInvitationManager{
		ttl: ttl,
		// make 初始化可写 map；nil map 可以读取，但第一次写入就会 panic。
		invitations: make(map[string]*GameInvitation),
		activePairs: make(map[invitationUserPair]string),
	}
}

// Create 创建一条 pending 邀请，并拒绝同一对用户的正向或反向重复邀请。
func (m *GameInvitationManager) Create(inviter, invitee UserSummary) (GameInvitation, error) {
	// 对用户 ID 做统一领域校验，并生成忽略邀请方向的 low/high。
	low, high, err := canonicalUserPair(inviter.ID, invitee.ID)
	if err != nil || m.ttl <= 0 {
		return GameInvitation{}, ErrInvalidGameInvitation
	}

	// “检查是否重复 + 创建 + 写入两张 map”必须放在同一临界区，避免并发创建两条邀请。
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	pair := invitationUserPair{low: low, high: high}
	if invitationID, exists := m.activePairs[pair]; exists {
		// activePairs 中的旧邀请可能刚好已经超时，先惰性过期再决定是否冲突。
		if invitation := m.invitations[invitationID]; invitation != nil {
			m.expireLocked(invitation, now)
		}
		// expireLocked 会删除过期索引；索引仍存在才说明确有有效 pending 邀请。
		if _, stillActive := m.activePairs[pair]; stillActive {
			return GameInvitation{}, ErrGameInvitationPending
		}
	}

	// 随机 ID 不能从用户 ID 或时间推断，避免他人枚举并操作不属于自己的邀请。
	invitationID, err := generateGameInvitationID()
	if err != nil {
		return GameInvitation{}, err
	}
	invitation := &GameInvitation{
		ID:              invitationID,
		InviterID:       inviter.ID,
		InviterUsername: inviter.Username,
		InviteeID:       invitee.ID,
		InviteeUsername: invitee.Username,
		Status:          GameInvitationPending,
		CreatedAt:       now,
		ExpiresAt:       now.Add(m.ttl),
	}
	// 主表保存完整对象，反向索引只保存 ID；两者在同一把锁下同时写入。
	m.invitations[invitation.ID] = invitation
	m.activePairs[pair] = invitation.ID
	return *invitation, nil
}

// FindByID 返回邀请快照；读取时发现超时会先把状态更新为 expired。
func (m *GameInvitationManager) FindByID(invitationID string) (GameInvitation, error) {
	// 查询也可能触发 pending -> expired 的写操作，因此这里不能只使用读锁。
	m.mu.Lock()
	defer m.mu.Unlock()

	invitation := m.invitations[invitationID]
	if invitation == nil {
		return GameInvitation{}, ErrGameInvitationNotFound
	}
	// 采用惰性过期：读取或处理邀请时检查时间，不额外启动定时清理 goroutine。
	m.expireLocked(invitation, time.Now().UTC())
	// 返回值副本，防止调用方绕过 Manager 的锁直接修改内部对象。
	return *invitation, nil
}

// Accept 只允许被邀请者接受，并把成功创建的房间号记录到邀请结果中。
func (m *GameInvitationManager) Accept(
	invitationID string,
	currentUserID int64,
	roomID string,
) (GameInvitation, error) {
	if roomID == "" {
		return GameInvitation{}, ErrInvalidGameInvitation
	}
	return m.finish(invitationID, currentUserID, GameInvitationAccepted, roomID, false)
}

// Reject 只允许被邀请者拒绝邀请。
func (m *GameInvitationManager) Reject(invitationID string, currentUserID int64) (GameInvitation, error) {
	return m.finish(invitationID, currentUserID, GameInvitationRejected, "", false)
}

// Cancel 只允许邀请者在对方处理前撤销邀请。
func (m *GameInvitationManager) Cancel(invitationID string, currentUserID int64) (GameInvitation, error) {
	return m.finish(invitationID, currentUserID, GameInvitationCancelled, "", true)
}

// finish 统一完成 accepted/rejected/cancelled 三种终止操作。
// requireInviter=true 表示只有邀请者能操作；否则只有被邀请者能操作。
func (m *GameInvitationManager) finish(
	invitationID string,
	currentUserID int64,
	nextStatus GameInvitationStatus,
	roomID string,
	requireInviter bool,
) (GameInvitation, error) {
	// 检查状态、校验操作者、修改邀请和删除 activePairs 必须原子执行。
	m.mu.Lock()
	defer m.mu.Unlock()

	invitation := m.invitations[invitationID]
	if invitation == nil {
		return GameInvitation{}, ErrGameInvitationNotFound
	}
	now := time.Now().UTC()
	if m.expireLocked(invitation, now) {
		return GameInvitation{}, ErrGameInvitationExpired
	}
	if invitation.Status != GameInvitationPending {
		return GameInvitation{}, ErrGameInvitationNotPending
	}
	// 撤销属于邀请者权限；接受和拒绝属于被邀请者权限。
	if requireInviter {
		if invitation.InviterID != currentUserID {
			return GameInvitation{}, ErrGameInvitationNotInviter
		}
	} else if invitation.InviteeID != currentUserID {
		return GameInvitation{}, ErrGameInvitationNotInvitee
	}

	// 完成后的邀请仍保留在 invitations 中，供后续查询最终状态；只释放 activePairs。
	invitation.Status = nextStatus
	invitation.RoomID = roomID
	invitation.RespondedAt = &now
	m.removeActivePairLocked(invitation)
	return *invitation, nil
}

// expireLocked 在持锁状态下把过期的 pending 邀请转换成 expired。
func (m *GameInvitationManager) expireLocked(invitation *GameInvitation, now time.Time) bool {
	// now.Before(ExpiresAt) 为 false 时表示已经到达或超过截止时间。
	if invitation.Status != GameInvitationPending || now.Before(invitation.ExpiresAt) {
		return false
	}
	invitation.Status = GameInvitationExpired
	invitation.RespondedAt = &now
	m.removeActivePairLocked(invitation)
	return true
}

// removeActivePairLocked 只能在持有 mu 时调用；邀请结束后释放“该用户对正在邀请”的占位。
func (m *GameInvitationManager) removeActivePairLocked(invitation *GameInvitation) {
	low, high, err := canonicalUserPair(invitation.InviterID, invitation.InviteeID)
	if err == nil {
		delete(m.activePairs, invitationUserPair{low: low, high: high})
	}
}

// generateGameInvitationID 读取 128 位密码学随机数，并编码成 32 位十六进制字符串。
func generateGameInvitationID() (string, error) {
	// make([]byte, 16) 分配 16 字节；crypto/rand 使用操作系统安全随机源填充。
	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(randomBytes), nil
}
