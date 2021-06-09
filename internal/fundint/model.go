package fundint

import (
	"fmt"
	"time"
)

// ResolveSnapState 解析快照生命周期。
// 状态机：
// fresh ──(TTL 到期)──▶ expired
// fresh ──(目录改绑检测到偏差)──▶ deviated
// deviated ──(管理员确认)──▶ closed

type ResolveSnapState string

const (
	ResolveSnapFresh ResolveSnapState = "fresh"
	ResolveSnapExpired  ResolveSnapState = "expired"
	ResolveSnapDeviated ResolveSnapState = "deviated"
	ResolveSnapClosed   ResolveSnapState = "closed"
)

// ResolveSnap 对手解析世代快照。
// 字段不变式：
// - FromUserID 不得与 ToUserID 相同（自转拒绝）。
// - ResolvedAt 必须早于 ExpiresAt。
// - Gen 单调递增，同 idem_key 下仅允许一个 fresh snap。

type ResolveSnap struct {
	ID         string // UUID v4
	FromUserID string // 发起方
	ToUserID string // 解析结果（接收方）
	Channel  string // uid|email|phone（首个命中渠道）
	Channels string // 所有参与解析渠道（逗号分隔）
	IdemKey  string // 对应 Send 请求幂等键
	Gen      int64  // 世代号，每次重新 resolve 递增
	Asset    string // 币种
	Amount     string           // decimal string（与 Send 请求一致）
	State      ResolveSnapState // 生命周期状态
	ResolvedAt time.Time        // 解析完成时刻
	ExpiresAt time.Time // 快照过期时刻（默认 +5min）
	DeviationAt *time.Time // 串户检测发现时刻
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Valid 检查快照是否仍在有效期内可用。

   func (s *ResolveSnap) Valid(now time.Time) error {
	   if s == nil {
		   return fmt.Errorf("resolve snap nil")
	  }
	  if s.State != ResolveSnapFresh {
		   return fmt.Errorf("resolve snap state=%s not usable", s.State)
	   }
	  if now.After(s.ExpiresAt) {
		  return fmt.Errorf("resolve snap expired at %s", s.ExpiresAt.Format(time.RFC3339))
	  }
	   return nil
  }

// DebitLeaseState 借记互斥租约生命周期。
// 状态机：
// pending ──(acquire)──▶ held
// held ──(commit)──▶ committed
// held ──(TTL 到期 / 手动释放)──▶ released
// pending ──(duplicate idem key)──▶ conflict
type DebitLeaseState string

const (
	DebitLeasePending DebitLeaseState = "pending"
	DebitLeaseHeld DebitLeaseState = "held"
	DebitLeaseCommitted DebitLeaseState = "committed"
	DebitLeaseReleased DebitLeaseState = "released"
	DebitLeaseConflict DebitLeaseState = "conflict"
)

// DebitLease 同幂等键双 Send 互斥租约。
// 字段不变式：
// - IdemKey 全局唯一；同 key 第二次申请时须 conflict 拒绝或返回已有 lease。
// - SnapID 指向对应 ResolveSnap；committed 后 LedgerID 必须填写。
// - ExpiresAt 到期后 patrol worker 将释放 held 租约。

type DebitLease struct {
	ID      string // UUID v4
	IdemKey string // 等同 Send.IdempotencyKey
	SnapID     string // 关联 ResolveSnap.ID
	FromUserID string // 扣款方
	ToUserID string // 收款方（快照锁定）
	Asset  string // 币种
	Amount string // decimal string
	State      DebitLeaseState // 租约状态
	LedgerID   string          // committed 后填入 ledger.Transfer.ID
	AcquiredAt *time.Time      // 进入 held 的时刻
	CommittedAt *time.Time // 进入 committed 的时刻
	ExpiresAt   time.Time  // 租约 TTL（默认 +30s）
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ConflictError 同幂等键冲突，含已有 lease 摘要。
type ConflictError struct {
	LeaseID string
	IdemKey string
}

  func (e *ConflictError) Error() string {
	   return fmt.Sprintf("debit lease conflict: idem_key=%s existing_lease=%s", e.IdemKey, e.LeaseID)
   }
