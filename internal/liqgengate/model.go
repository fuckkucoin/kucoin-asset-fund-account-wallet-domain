package liqgengate

import (
	"fmt"
	"strings"
	"time"
)

// ─── 借款+强平联合状态机 ─────────────────────────────────────────────────
// LiqPhase 借款在强平世代面上的相位。
// open ──(FreezeWithGen)──▶ freezing ──(hook 成功)──▶ frozen
// freezing ──(hook 失败)──▶ half_open
// frozen ──(ReleaseWithGen)──▶ releasing ──(CloseWithGen)──▶ closed
// half_open ──(运维收口 / 重试 hook 成功)──▶ frozen
// open | freezing | frozen | releasing 不得在缺世代或世代漂移时进入释放成功路径。

type LiqPhase string

const (
	PhaseOpen     LiqPhase = "open"
	PhaseFreezing LiqPhase = "freezing"
	PhaseFrozen   LiqPhase = "frozen"
	PhaseReleasing LiqPhase = "releasing"
	PhaseClosed   LiqPhase = "closed"
	PhaseHalfOpen LiqPhase = "half_open"
)

// Closed 终态。
   func (p LiqPhase) Closed() bool { return p == PhaseClosed }

// Releasable 仅 frozen 可进入释放闸。
  func (p LiqPhase) Releasable() bool { return p == PhaseFrozen }

// Freezable 仅 open 可进入冻结闸（half_open 走运维重试，不走首次 Freeze）。

  func (p LiqPhase) Freezable() bool { return p == PhaseOpen || p == "" }

// ActiveMutex 占用 Freeze↔Release 互斥锁的相位。
   func (p LiqPhase) ActiveMutex() bool {
	   switch p {
	  case PhaseFreezing, PhaseReleasing:
		   return true
	  }
	  return false
   }

// ValidPhaseTransitions 合法转换。
var ValidPhaseTransitions = map[LiqPhase][]LiqPhase{
	PhaseOpen:     {PhaseFreezing},
	PhaseFreezing: {PhaseFrozen, PhaseHalfOpen},
	PhaseFrozen:    {PhaseReleasing},
	PhaseReleasing: {PhaseClosed, PhaseFrozen},
	PhaseHalfOpen: {PhaseFreezing, PhaseFrozen, PhaseClosed},
	PhaseClosed: {},
}

// CanTransitPhase 校验相位转换。
   func CanTransitPhase(from, to LiqPhase) bool {
	   if from == "" {
		  from = PhaseOpen
	   }
	  for _, t := range ValidPhaseTransitions[from] {
		  if t == to {
			   return true
		   }
	  }
	   return false
   }

// ─── 冻结租约状态 ─────────────────────────────────────────────────────────
// LeaseState 冻结意图租约状态。

type LeaseState string

const (
	LeaseIssued LeaseState = "issued"
	LeaseFreezing LeaseState = "freezing"
	LeaseFrozen    LeaseState = "frozen"
	LeaseReleasing LeaseState = "releasing"
	LeaseConsumed  LeaseState = "consumed"
	LeaseExpired   LeaseState = "expired"
	LeaseRevoked   LeaseState = "revoked"
	LeaseHalfOpen LeaseState = "half_open"
)

// Closed 租约终态。
  func (s LeaseState) Closed() bool {
	   switch s {
	  case LeaseConsumed, LeaseExpired, LeaseRevoked:
		  return true
	  }
	  return false
   }

// Active 未终态。

   func (s LeaseState) Active() bool {
	   switch s {
	   case LeaseIssued, LeaseFreezing, LeaseFrozen, LeaseReleasing, LeaseHalfOpen:
		   return true
	  }
	  return false
  }

// ValidLeaseTransitions 租约合法转换。
var ValidLeaseTransitions = map[LeaseState][]LeaseState{
	LeaseIssued: {LeaseFreezing, LeaseExpired, LeaseRevoked},
	LeaseFreezing: {LeaseFrozen,
		LeaseHalfOpen, LeaseExpired, LeaseRevoked},
	LeaseFrozen:    {LeaseReleasing, LeaseExpired, LeaseRevoked},
	LeaseReleasing: {LeaseConsumed, LeaseFrozen, LeaseRevoked},
	LeaseHalfOpen:  {LeaseFreezing, LeaseFrozen, LeaseRevoked, LeaseExpired},
	LeaseConsumed:  {},
	LeaseExpired:   {},
	LeaseRevoked: {},
}

// CanTransitLease 校验租约转换。
   func CanTransitLease(from, to LeaseState) bool {
	   for _, t := range ValidLeaseTransitions[from] {
		  if t == to {
			   return true
		   }
	   }
	  return false
  }

// ─── 证明状态 ─────────────────────────────────────────────────────────────
// ProofKind 证明种类。
type ProofKind string

const (
	ProofKindHalf ProofKind = "half"
	ProofKindRace ProofKind = "race"
)

// ProofState 证明仓状态。

type ProofState string

const (
	ProofOpen ProofState = "open"
	ProofProven ProofState = "proven"
	ProofClosed ProofState = "closed"
	ProofDismiss ProofState = "dismissed"
)

// Closed 证明终态。
  func (s ProofState) Closed() bool {
	   return s == ProofClosed || s == ProofDismiss
   }

// RaceReason 竞态原因。
type RaceReason string

const (
	RaceStaleGen   RaceReason = "stale_gen"
	RaceMissingGen RaceReason = "missing_gen"
	RaceMutexHeld       RaceReason = "mutex_held"
	RaceUnexpectedPhase RaceReason = "unexpected_phase"
	RaceDoubleRelease RaceReason = "double_release"
	RaceLeaseMismatch RaceReason = "lease_mismatch"
	RaceExpiredLease  RaceReason = "expired_lease"
	RaceHalfBlocked   RaceReason = "half_blocked"
)

// HalfReason 半截原因。
type HalfReason string

const (
	HalfHookFailed HalfReason = "hook_failed"
	HalfHoldOrphan HalfReason = "hold_orphan"
	HalfRetryTimeout HalfReason = "retry_timeout"
	HalfInspected HalfReason = "inspected"
)

// ─── 领域模型 ─────────────────────────────────────────────────────────────
// LiqGeneration 借款维度强平世代。
// 不变量：
// - Gen 单调递增，FreezeWithGen 每次成功签发必须 +1。
// - LTVBPS / CollateralID 为签发快照，只读。
// - 不改写 LTV 公式；快照仅用于世代。
type LiqGeneration struct {
	ID string
	BorrowID     string
	UserID       string
	CollateralID string
	Gen uint64
	LTVBPS int
	CapBPS        int
	BorrowAmt     string
	CollateralAmt string
	Asset string
	Phase LiqPhase
	LeaseID string
	HoldID  string
	IdemKey string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ScopeKey user|borrow。

  func (g *LiqGeneration) ScopeKey() string {
	  if g == nil {
		   return ""
	  }
	  return g.UserID + "|" + g.BorrowID
   }

// Snapshot 世代快照串（进入证明哈希）。

   func (g *LiqGeneration) Snapshot() string {
	   if g == nil {
		   return ""
	  }
	  return fmt.Sprintf("%s|%s|%d|%d|%s|%s", g.BorrowID, g.CollateralID, g.Gen, g.LTVBPS, g.BorrowAmt, g.CollateralAmt)
   }

// LiqFreezeLease 冻结意图租约。
// 不变量：
// - BorrowID / CollateralID / LiqGen / LTVBPS 签发后只读。
// - 终态不可逆。
// - 同一 borrow 同一世代至多一张活跃租约。
type LiqFreezeLease struct {
	ID string
	BorrowID string
	UserID string
	CollateralID string
	HoldID       string
	LiqGen uint64
	LTVBPS int
	CapBPS int
	Asset  string
	Amount string
	State LeaseState
	IdemKey string
	IssuedAt  time.Time
	ExpiresAt time.Time
	FrozenAt *time.Time
	ReleasedAt *time.Time
	ConsumedAt *time.Time
	RevokedAt  *time.Time
	RevokeReason string
	HalfProofID  string
	RaceProofID  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Expired 实时 TTL。

  func (l *LiqFreezeLease) Expired(now time.Time) bool {
	  if l == nil {
		  return true
	   }
	   return !l.ExpiresAt.IsZero() && (now.After(l.ExpiresAt) || now.Equal(l.ExpiresAt))
   }

// MatchesGen 世代匹配。
   func (l *LiqFreezeLease) MatchesGen(gen uint64) bool {
	   return l != nil && l.LiqGen == gen
   }

// LiqHalfProof 半截冻结证明。
// 冻结 Hold 已落、freeze hook 失败或超时：借款不得按已完全强平释放。

type LiqHalfProof struct {
	ID       string
	BorrowID string
	UserID   string
	LeaseID  string
	HoldID   string
	LiqGen uint64
	Reason HalfReason
	HookError    string
	PhaseAtIssue LiqPhase
	State        ProofState
	IdemKey string
	Digest  string
	ProvenAt *time.Time
	ClosedAt *time.Time
	Note     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// LiqRaceProof 世代竞态/互斥拒释证明。
type LiqRaceProof struct {
	ID string
	BorrowID string
	UserID   string
	LeaseID  string
	ExpectedGen uint64
	ActualGen uint64
	Reason RaceReason
	Actor        string
	PhaseAtIssue LiqPhase
	State        ProofState
	IdemKey string
	Digest string
	ProvenAt *time.Time
	ClosedAt *time.Time
	Note      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// LiqProof 统一导出视图（半截与竞态共用仓位表）。
type LiqProof struct {
	ID string
	Kind     ProofKind
	BorrowID string
	UserID  string
	LeaseID string
	LiqGen  uint64
	PeerGen uint64
	Reason string
	State  ProofState
	IdemKey string
	Digest       string
	HookError    string
	PhaseAtIssue string
	Actor string
	ProvenAt  *time.Time
	ClosedAt  *time.Time
	Note      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// KindOf 从 Reason 推断（仓位表存 kind 列）。
  func (p *LiqProof) KindOf() ProofKind {
	   if p == nil {
		  return ""
	   }
	  return p.Kind
  }

// LiqAudit 相位/租约审计。
type LiqAudit struct {
	ID       string
	BorrowID string
	LeaseID string
	FromPhase string
	ToPhase   string
	FromLease string
	ToLease string
	Gen       uint64
	Actor     string
	Note      string
	CreatedAt time.Time
}

// MutexRecord 同 borrow Freeze↔Release 互斥占位。
type MutexRecord struct {
	BorrowID string
	Owner      string
	Kind       string
	Gen        uint64
	AcquiredAt time.Time
}

// LiveBorrow 只读挂钩：marginhook.Borrow 快照。

type LiveBorrow struct {
	ID     string
	UserID string
	Asset string
	Amount     string
	Collateral string
	LTVBPS     int
	Status string
	LiqGen   uint64
	LiqPhase string
}

// LiveHold 只读挂钩：marginhook.Hold 快照。
type LiveHold struct {
	ID string
	UserID string
	Kind   string
	Asset  string
	Amount string
	CapBPS int
	Status string
	BorrowID string
}

// LTVSnapshot 签发时 LTV 只读快照（不重算公式内核）。

type LTVSnapshot struct {
	BorrowAmt     string
	CollateralAmt string
	LTVBPS        int
	CapBPS        int
}

// Combined 快照输入串。

   func (s LTVSnapshot) Combined() string {
	   return strings.Join([]string{s.BorrowAmt, s.CollateralAmt, fmt.Sprintf("%d", s.LTVBPS), fmt.Sprintf("%d", s.CapBPS)}, "|")
   }

// Equal 精确比较。
  func (s LTVSnapshot) Equal(o LTVSnapshot) bool {
	  return s.BorrowAmt == o.BorrowAmt &&
		   s.CollateralAmt == o.CollateralAmt &&
		   s.LTVBPS == o.LTVBPS &&
		   s.CapBPS == o.CapBPS
  }
