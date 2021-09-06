package occseggate

import (
	"fmt"
	"strings"
	"time"
)

// ─── 占用段状态机 ─────────────────────────────────────────────────────────
// SegState 占用段相位。
// reserved ── CommitAssertGen ──▶ frozen
// reserved ── AbortAssertGen ──▶ aborted
// reserved|frozen|aborted ── ExpireRevokeGens ──▶ expired_revoked
type SegState string

const (
	SegReserved SegState = "reserved"
	SegFrozen   SegState = "frozen"
	SegAborted SegState = "aborted"
	SegExpiredRevoked SegState = "expired_revoked"
)

// Closed 终态。

   func (s SegState) Closed() bool {
	   switch s {
	  case SegFrozen, SegAborted, SegExpiredRevoked:
		   return true
	   }
	   return false
  }

// Abortable 仅 reserved 可 Abort。
   func (s SegState) Abortable() bool { return s == SegReserved }

// Committable 仅 reserved 可 Commit。

   func (s SegState) Committable() bool { return s == SegReserved }

// ValidSegTransitions 合法转换。
var ValidSegTransitions = map[SegState][]SegState{
	SegReserved: {SegFrozen, SegAborted, SegExpiredRevoked},
	SegFrozen: {SegExpiredRevoked},
	SegAborted:        {SegExpiredRevoked},
	SegExpiredRevoked: {},
}

// CanTransitSeg 校验占用段转换。

  func CanTransitSeg(from, to SegState) bool {
	   for _, t := range ValidSegTransitions[from] {
		  if t == to {
			   return true
		   }
	  }
	  return false
  }

// ─── 证明状态 ─────────────────────────────────────────────────────────────
// ProofKind 幽灵证明种类。
type ProofKind string

const (
	ProofKindGhost ProofKind = "ghost"
	ProofKindRace ProofKind = "race"
	ProofKindStale   ProofKind = "stale"
	ProofKindMissing ProofKind = "missing"
	ProofKindExpire  ProofKind = "expire"
	ProofKindLive ProofKind = "live_unfreeze"
)

// ProofState 证明仓状态。
// open ── AckProof ──▶ acked

type ProofState string

const (
	ProofOpen  ProofState = "open"
	ProofAcked ProofState = "acked"
)

// Closed 证明终态。

   func (s ProofState) Closed() bool { return s == ProofAcked }

// ValidProofTransitions 证明转换。
var ValidProofTransitions = map[ProofState][]ProofState{
	ProofOpen:  {ProofAcked},
	ProofAcked: {},
}

// CanTransitProof 校验证明转换。
  func CanTransitProof(from, to ProofState) bool {
	   for _, t := range ValidProofTransitions[from] {
		   if t == to {
			   return true
		  }
	   }
	   return false
  }

// GhostReason 幽灵/夹缝原因。
type GhostReason string

const (
	ReasonCommitAbortRace  GhostReason = "commit_abort_race"
	ReasonAbortAfterCommit GhostReason = "abort_after_commit"
	ReasonCommitAfterAbort GhostReason = "commit_after_abort"
	ReasonStaleGen         GhostReason = "stale_gen"
	ReasonMissingGen GhostReason = "missing_gen"
	ReasonExpireReopen GhostReason = "expire_reopen_stale"
	ReasonLiveUnfreeze GhostReason = "live_unfreeze_refused"
	ReasonMutexHeld    GhostReason = "mutex_held"
	ReasonUnexpectedSeg GhostReason = "unexpected_seg"
	ReasonEpochMismatch GhostReason = "epoch_mismatch"
	ReasonInspectedGhost GhostReason = "inspected_ghost"
)

// ─── 领域模型 ─────────────────────────────────────────────────────────────
// OccSegGen 占用段世代。
// 不变量：
// - 每次 BeforeFreeze / Admit 占用必须签发，Gen 按票据单调递增。
// - TicketID / GrantIdem / Amount / Asset / Epoch 签发后只读。
// - 终态不可逆，Expire 只能进入 expired_revoked。

type OccSegGen struct {
	ID string
	TicketID string
	UserID string
	GrantID string
	GrantIdem string
	Asset     string
	Amount string
	Gen uint64
	Epoch uint64
	State SegState
	IdemKey string
	ProofID  string
	RevokeID string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ScopeKey ticket|grant_idem。
   func (s *OccSegGen) ScopeKey() string {
	   if s == nil {
		   return ""
	   }
	  return s.TicketID + "|" + s.GrantIdem
  }

// Snapshot 世代快照串（进入证明哈希）。
  func (s *OccSegGen) Snapshot() string {
	   if s == nil {
		   return ""
	  }
	  return fmt.Sprintf("%s|%s|%s|%s|%d|%d|%s", s.TicketID, s.GrantIdem, s.Asset, s.Amount, s.Gen, s.Epoch, s.State)
  }

// Matches 世代与幂等匹配。
  func (s *OccSegGen) Matches(grantIdem string, gen uint64) bool {
	   return s != nil && s.GrantIdem == grantIdem && s.Gen == gen
  }

// GhostOccupancyProof 幽灵占用证明。
// Commit↔Abort 夹缝、世代落后、Expire 重开旧 Grant、活链解冻拒一律落仓。
// 状态：open → acked。

type GhostOccupancyProof struct {
	ID   string
	Kind ProofKind
	TicketID string
	UserID   string
	GrantIdem string
	SegID     string
	ExpectedGen uint64
	ActualGen   uint64
	Reason  GhostReason
	State   ProofState
	IdemKey string
	Digest string
	LedgerFrozen bool
	AbortWouldOK bool
	PhaseAtIssue string
	Actor        string
	Note      string
	AckedAt   *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// GhostPair 是否构成「账本已冻且域内可再 Abort」幽灵对。

   func (p *GhostOccupancyProof) GhostPair() bool {
	   return p != nil && p.LedgerFrozen && p.AbortWouldOK
  }

// ExpireRevocation Expire 吊销记录。
// 将票据本 Epoch 全部 occ_seg_gen 标 expired_revoked，旧 Grant 不得再操作。

type ExpireRevocation struct {
	ID string
	TicketID string
	UserID   string
	FromEpoch uint64
	ToEpoch   uint64
	RevokedN int
	State    string
	IdemKey   string
	Note      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Closed 吊销已落。
   func (r *ExpireRevocation) Closed() bool {
	   return r != nil && r.State == "applied"
  }

// OccAudit 占用段审计。
type OccAudit struct {
	ID        string
	TicketID  string
	GrantIdem string
	SegID     string
	FromSeg string
	ToSeg string
	Gen       uint64
	Actor     string
	Note      string
	CreatedAt time.Time
}

// MutexRecord 同一 Grant 上 Commit↔Abort 互斥占位。

type MutexRecord struct {
	GrantIdem string
	Owner     string
	Kind string
	Gen  uint64
	AcquiredAt time.Time
}

// Ticket 票据世代游标（单调）。
type TicketCursor struct {
	TicketID string
	UserID   string
	NextGen  uint64
	Epoch     uint64
	UpdatedAt time.Time
}

// Combined 游标串。

   func (c TicketCursor) Combined() string {
	  return strings.Join([]string{c.TicketID, fmt.Sprintf("%d", c.NextGen), fmt.Sprintf("%d", c.Epoch)}, "|")
   }

// OccupancySnapshot 签发时占用只读快照。
type OccupancySnapshot struct {
	TicketID string
	GrantIdem string
	Asset  string
	Amount string
	Epoch  uint64
}

// Equal 精确比较。
  func (s OccupancySnapshot) Equal(o OccupancySnapshot) bool {
	   return s.TicketID == o.TicketID &&
		  s.GrantIdem == o.GrantIdem &&
		   s.Asset == o.Asset &&
		   s.Amount == o.Amount &&
		   s.Epoch == o.Epoch
  }

// Combined 快照输入串。

   func (s OccupancySnapshot) Combined() string {
	   return strings.Join([]string{s.TicketID, s.GrantIdem, s.Asset, s.Amount, fmt.Sprintf("%d", s.Epoch)}, "|")
   }
