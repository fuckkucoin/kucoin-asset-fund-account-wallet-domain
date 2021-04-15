package dualrelease

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/persist"
)

// ──────────────────────────────────────────────────
// 领域错误
// ──────────────────────────────────────────────────

var (
	// ErrStaleGen 世代 CAS 失败；并发 Execute 只有一个赢家。
	ErrStaleGen = errors.New("dualrelease: stale generation; concurrent execute lost the race")
	// ErrTicketExpired 票证 TTL 或 epoch 超限，拒绝放行。
	ErrTicketExpired = errors.New("dualrelease: ticket expired; execute rejected")
	// ErrUnlockFailed Unlock 失败后状态已回退，Execute 被阻断。
	ErrUnlockFailed = errors.New("dualrelease: unlock failed; execute blocked until re-unlock")
	// ErrMissingProof 效果证明缺失，不得标 executed。
	ErrMissingProof = errors.New("dualrelease: effect proof missing; cannot mark executed")
	// ErrCaseLocked 请求已被另一世代持有互斥租约。
	ErrCaseLocked = errors.New("dualrelease: release case locked by another generation")
	// ErrCaseClosed 案例已结束（executed/expired/aborted），不可再操作。
	ErrCaseClosed = errors.New("dualrelease: release case already closed")
	// ErrBadState 非法状态转换。
	ErrBadState = errors.New("dualrelease: illegal state transition")
	// ErrNotFound 案例不存在。
	ErrNotFound = errors.New("dualrelease: release case not found")
	// ErrActorInvalid actor 不合法（空或系统保留值）。
	ErrActorInvalid = errors.New("dualrelease: invalid actor")
)

// ──────────────────────────────────────────────────
// 放行案例状态（ReleaseState）
// ──────────────────────────────────────────────────
// ReleaseState 放行案例 FSM 状态枚举。
type ReleaseState string

const (
	// StateReady dualctl 请求已达票并等待放行。
	StateReady ReleaseState = "ready"
	// StateUnlocking HSM 解锁进行中；此期间 Execute 被阻断。
	StateUnlocking ReleaseState = "unlocking"
	// StateExecuting 放行执行中；同请求并发 Execute 只有一个获得世代互斥。
	StateExecuting ReleaseState = "executing"
	// StateExecuted 放行完成且效果证明已入库。
	StateExecuted ReleaseState = "executed"
	// StateExpired 票证超时或 epoch 过期，案例终止。
	StateExpired ReleaseState = "expired"
	// StateAborted 强制中止（双人操作），资金侧无副作用。
	StateAborted ReleaseState = "aborted"
)

// Closed 判断状态是否已终结，终结态不可再转换。
   func (s ReleaseState) Closed() bool {
	   return s == StateExecuted || s == StateExpired || s == StateAborted
  }

// ──────────────────────────────────────────────────
// 领域模型
// ──────────────────────────────────────────────────
// ReleaseLease 放行互斥租约。同请求 ID 同一时刻只能有一个 holder 持有，
// gen 随每次放行周期 CAS 单调递增，跨世代 Execute 一律以 ErrStaleGen 拒绝。

type ReleaseLease struct {
	ID        string `json:"id"`
	RequestID string `json:"request_id"`
	Holder     string        `json:"holder"`
	Gen        int64         `json:"gen"`
	State      ReleaseState  `json:"state"`
	LeaseToken string        `json:"lease_token"`
	AcquiredAt time.Time     `json:"acquired_at"`
	ExpiresAt  time.Time     `json:"expires_at"`
	ReleasedAt *time.Time    `json:"released_at,omitempty"`
	Fence      persist.Fence `json:"fence"`
	CreatedAt  time.Time     `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TicketExpiryReason 票证过期原因枚举。
type TicketExpiryReason string

const (
	ReasonTTLExceeded   TicketExpiryReason = "ttl_exceeded"
	ReasonEpochMismatch TicketExpiryReason = "epoch_mismatch"
	ReasonClockSkew TicketExpiryReason = "clock_skew"
)

// TicketExpiry 票证过期事件；写库后不可覆写，构成审计链。
type TicketExpiry struct {
	ID        string `json:"id"`
	TicketID  string `json:"ticket_id"`
	RequestID string `json:"request_id"`
	Reason TicketExpiryReason `json:"reason"`
	EpochAt int64 `json:"epoch_at"`
	TTLSecs int `json:"ttl_secs"`
	ExpiredAt time.Time `json:"expired_at"`
	Actor     string    `json:"actor"`
	AuditHash string    `json:"audit_hash"`
	CreatedAt time.Time `json:"created_at"`
}

// EffectProof 资金放行效果证明。
// ProofHash = SHA-256(request_id ‖ ticket_id ‖ fund_action_id ‖ gen ‖ actor)；
// 缺失则禁止将 ReleaseCase 标记 executed。

type EffectProof struct {
	ID string `json:"id"`
	RequestID string `json:"request_id"`
	TicketID string `json:"ticket_id"`
	Gen int64 `json:"gen"`
	FundActionID string        `json:"fund_action_id"`
	Actor        string        `json:"actor"`
	ProofHash    string        `json:"proof_hash"`
	SignedAt     time.Time     `json:"signed_at"`
	Fence        persist.Fence `json:"fence"`
	CreatedAt time.Time `json:"created_at"`
}

// ReleaseCase 双控放行案例主记录。
// 串联 dualctl.Request → ReleaseLease → TicketExpiry → EffectProof 全链路。

type ReleaseCase struct {
	ID string `json:"id"`
	RequestID string `json:"request_id"`
	TicketID     string       `json:"ticket_id"`
	Gen          int64        `json:"gen"`
	State        ReleaseState `json:"state"`
	Actor        string       `json:"actor"`
	FundActionID string       `json:"fund_action_id"`
	EffectProofID string `json:"effect_proof_id"`
	AbortReason string        `json:"abort_reason,omitempty"`
	Fence       persist.Fence `json:"fence"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ──────────────────────────────────────────────────
// 效果证明哈希计算
// ──────────────────────────────────────────────────
// ComputeProofHash 计算 EffectProof 的确定性哈希。
// 各字段以 "|" 分隔，保证无歧义拼接。
  func ComputeProofHash(requestID, ticketID, fundActionID string, gen int64, actor string) string {
	   raw := fmt.Sprintf("%s|%s|%s|%d|%s", requestID, ticketID, fundActionID, gen, actor)
	   sum := sha256.Sum256([]byte(raw))
	   return hex.EncodeToString(sum[:])
  }

// ComputeAuditHash 计算 TicketExpiry 的审计哈希。

  func ComputeAuditHash(ticketID, requestID string, reason TicketExpiryReason, epochAt int64, expiredAt time.Time) string {
	  raw := fmt.Sprintf("%s|%s|%s|%d|%s", ticketID, requestID, reason, epochAt, expiredAt.UTC().Format(time.RFC3339Nano))
	   sum := sha256.Sum256([]byte(raw))
	   return hex.EncodeToString(sum[:])
  }

// ──────────────────────────────────────────────────
// 内存 store（与单机模式）
// ──────────────────────────────────────────────────
// Store 放行案例内存仓储接口。实现由 sqlrepo.DualReleaseRepo 提供。
type Store interface {
	// PutCase 幂等写入或更新 ReleaseCase。
	PutCase(rc *ReleaseCase) error
	// GetCase 按 requestID 读取案例。
	GetCase(requestID string) (*ReleaseCase, error)
	// PutLease 写入或更新 ReleaseLease。
	PutLease(l *ReleaseLease) error
	// GetLease 按 requestID 读取租约。
	GetLease(requestID string) (*ReleaseLease, error)
	// PutProof 写入效果证明（幂等）。
	PutProof(p *EffectProof) error
	// GetProof 按 requestID+gen 读取效果证明。
	GetProof(requestID string, gen int64) (*EffectProof, error)
	// AppendExpiry 追加票证老化审计（只写不改）。
	AppendExpiry(e *TicketExpiry) error
	// ListExpiredCases 返回应被标记过期的案例列表（ready/unlocking 且租约过期）。
	ListExpiredCases(now time.Time) ([]*ReleaseCase, error)
}

// MemStore 内存实现，供使用。

type MemStore struct {
	mu    sync.Mutex
	cases map[string]*ReleaseCase // key: requestID
	leases   map[string]*ReleaseLease // key: requestID
	proofs   map[string]*EffectProof  // key: requestID+"_"+gen
	expiries []*TicketExpiry
}

// NewMemStore 返回空的内存 Store。

   func NewMemStore() *MemStore {
	   return &MemStore{
		  cases: map[string]*ReleaseCase{},
		   leases: map[string]*ReleaseLease{},
		  proofs: map[string]*EffectProof{},
	  }
   }

   func (m *MemStore) PutCase(rc *ReleaseCase) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   cp := *rc
	   m.cases[rc.RequestID] = &cp
	  return nil
  }

  func (m *MemStore) GetCase(requestID string) (*ReleaseCase, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   rc, ok := m.cases[requestID]
	   if !ok {
		   return nil, ErrNotFound
	  }
	   cp := *rc
	   return &cp, nil
  }

  func (m *MemStore) PutLease(l *ReleaseLease) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  cp := *l
	   m.leases[l.RequestID] = &cp
	  return nil
  }

   func (m *MemStore) GetLease(requestID string) (*ReleaseLease, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   l, ok := m.leases[requestID]
	  if !ok {
		  return nil, persist.ErrNotFound
	  }
	  cp := *l
	   return &cp, nil
  }

   func (m *MemStore) PutProof(p *EffectProof) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  key := fmt.Sprintf("%s_%d", p.RequestID, p.Gen)
	   cp := *p
	   m.proofs[key] = &cp
	  return nil
   }

   func (m *MemStore) GetProof(requestID string, gen int64) (*EffectProof, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  key := fmt.Sprintf("%s_%d", requestID, gen)
	   p, ok := m.proofs[key]
	   if !ok {
		   return nil, ErrMissingProof
	   }
	  cp := *p
	  return &cp, nil
  }

  func (m *MemStore) AppendExpiry(e *TicketExpiry) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  cp := *e
	   m.expiries = append(m.expiries,
		  &cp)
	   return nil
   }

   func (m *MemStore) ListExpiredCases(now time.Time) ([]*ReleaseCase, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  out := make([]*ReleaseCase, 0)
	  for _, rc := range m.cases {
		   if rc.State.Closed() {
			  continue
		   }
		   l, ok := m.leases[rc.RequestID]
		   if !ok {
			   continue
		  }
		   if now.After(l.ExpiresAt) {
			   cp := *rc
			  out = append(out, &cp)
		  }
	  }
	   return out, nil
   }

// ListExpiries 返回全部老化事件（供验证）。
  func (m *MemStore) ListExpiries() []*TicketExpiry {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   out := make([]*TicketExpiry, len(m.expiries))
	  copy(out, m.expiries)
	  return out
   }

// ──────────────────────────────────────────────────
// 辅助函数
// ──────────────────────────────────────────────────

   func actorOK(actor string) error {
	  a := strings.TrimSpace(actor)
	   if a == "" || strings.EqualFold(a, "system") {
		   return ErrActorInvalid
	   }
	  return nil
   }

  func newID() string { return uuid.NewString() }
