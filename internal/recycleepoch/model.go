package recycleepoch

import (
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/addrpool"
)

// SlotKey 地址槽位键。与 RecycleBook 槽位一一对应。

type SlotKey struct {
	Chain   string `json:"chain"`
	Address string `json:"address"`
}

   func (k SlotKey) String() string {
	  return strings.ToUpper(strings.TrimSpace(k.Chain)) + "|" + strings.ToLower(strings.TrimSpace(k.Address))
   }

  func (k SlotKey) Empty() bool {
	   return strings.TrimSpace(k.Chain) == "" || strings.TrimSpace(k.Address) == ""
   }

   func NormalizeSlot(chain, address string) SlotKey {
	  return SlotKey{
		   Chain:   strings.ToUpper(strings.TrimSpace(chain)),
		  Address: strings.ToLower(strings.TrimSpace(address)),
	   }
  }

// EpochState 回收世代状态。
// cooling → ready : ReadyAssert（仅冷却期满）
// ready → assigned : AssignUser（新用户绑定）
// assigned → cooling : 下一轮 MarkWithEpoch（推进 generation）
// 任意 → retired : 槽位退役，不得再贷记
type EpochState string

const (
	EpochCooling EpochState = "cooling"
	EpochReady    EpochState = "ready"
	EpochAssigned EpochState = "assigned"
	EpochRetired EpochState = "retired"
)

   func (s EpochState) Valid() bool {
	   switch s {
	   case EpochCooling, EpochReady, EpochAssigned, EpochRetired:
		   return true
	  default:
		   return false
	   }
  }

   func (s EpochState) Terminal() bool { return s == EpochRetired }

   func (s EpochState) AllowsAssign() bool { return s == EpochReady }

  func (s EpochState) AllowsCredit() bool { return s == EpochAssigned }

// RecycleEpoch 按地址槽位持久化的回收世代。
type RecycleEpoch struct {
	EpochID string `json:"epoch_id"`
	Slot       SlotKey    `json:"slot"`
	Generation int64      `json:"generation"`
	State      EpochState `json:"state"`
	PrevUser   string     `json:"prev_user,omitempty"`
	NextUser   string     `json:"next_user,omitempty"`
	CoolUntil time.Time `json:"cool_until"`
	MarkedAt  time.Time `json:"marked_at"`
	ReadyAt *time.Time `json:"ready_at,omitempty"`
	AssignedAt *time.Time `json:"assigned_at,omitempty"`
	ActorID    string     `json:"actor_id"`
	IdemKey    string    `json:"idem_key,omitempty"`
	RecycleRef string    `json:"recycle_ref,omitempty"` // 只读挂钩 RecycleRec
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

  func (e RecycleEpoch) CoolElapsed(now time.Time) bool {
	  if e.CoolUntil.IsZero() {
		   return false
	   }
	   return !now.Before(e.CoolUntil)
  }

   func (e RecycleEpoch) CurrentUser() string {
	  if e.State == EpochAssigned && strings.TrimSpace(e.NextUser) != "" {
		   return e.NextUser
	   }
	  return ""
   }

  func (e RecycleEpoch) HistoricalUser() string { return e.PrevUser }

// BindState 观察绑定状态。
type BindState string

const (
	BindActive BindState = "active"
	BindRevoked BindState = "revoked"
)

// ObserveBind 当前地址观察贷记权。再分配后旧绑定立即 revoked。
type ObserveBind struct {
	BindID string `json:"bind_id"`
	Slot       SlotKey `json:"slot"`
	EpochID    string  `json:"epoch_id"`
	Generation int64   `json:"generation"`
	UserID  string    `json:"user_id"`
	State   BindState `json:"state"`
	BoundAt time.Time `json:"bound_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	RevokeReason string `json:"revoke_reason,omitempty"`
}

  func (b ObserveBind) AllowsCredit(user string, gen int64) bool {
	  if b.State != BindActive {
		   return false
	  }
	  if b.Generation != gen {
		  return false
	   }
	   return strings.TrimSpace(b.UserID) == strings.TrimSpace(user)
   }

// ProofKind 错户证明类型。
type ProofKind string

const (
	ProofLateObserveOldUser ProofKind = "late_observe_old_user"
	ProofStaleEpoch   ProofKind = "stale_epoch"
	ProofMissingEpoch ProofKind = "missing_epoch"
	ProofForcedReuse  ProofKind = "forced_reuse_before_cool"
	ProofRevokedBind  ProofKind = "revoked_bind_credit"
	ProofReadyBeforeCool ProofKind = "ready_before_cool"
	ProofWrongNextUser ProofKind = "wrong_next_user"
)

// ProofState 证明仓状态。
type ProofState string

const (
	ProofOpen ProofState = "open"
	ProofAcked ProofState = "acked"
)

// MiscreditDisposition 迟观察处置。
type MiscreditDisposition string

const (
	DispReject MiscreditDisposition = "reject"
	DispQuarantine MiscreditDisposition = "quarantine"
)

// MiscreditProof 错户贷记证明仓。

type MiscreditProof struct {
	ProofID string `json:"proof_id"`
	Slot    SlotKey `json:"slot"`
	EpochID string  `json:"epoch_id"`
	Generation int64 `json:"generation"`
	CarriedGen int64     `json:"carried_gen"`
	Kind       ProofKind `json:"kind"`
	State ProofState `json:"state"`
	PrevUser string `json:"prev_user"`
	AttemptUser string `json:"attempt_user"`
	CurrentUser string `json:"current_user,omitempty"`
	TxHash      string `json:"tx_hash,omitempty"`
	Asset       string               `json:"asset,omitempty"`
	Amount      string               `json:"amount,omitempty"`
	Disposition MiscreditDisposition `json:"disposition"`
	FailClosed  bool                 `json:"fail_closed"`
	Summary     string               `json:"summary"`
	OpenedAt    time.Time            `json:"opened_at"`
	AckedAt *time.Time `json:"acked_at,omitempty"`
	ExportedAt *time.Time `json:"exported_at,omitempty"`
	ObserveID string `json:"observe_id,omitempty"`
}

// ObserveOutcome 观察贷记闸结果。

type ObserveOutcome string

const (
	OutcomeCredited ObserveOutcome = "credited"
	OutcomeRejected ObserveOutcome = "rejected"
	OutcomeQuarantined ObserveOutcome = "quarantined"
	OutcomeIdempotent  ObserveOutcome = "idempotent"
)

// ObserveRecord 已复验的观察尝试（成功或隔离）。
type ObserveRecord struct {
	ObserveID  string  `json:"observe_id"`
	Slot       SlotKey `json:"slot"`
	EpochID    string  `json:"epoch_id"`
	Generation int64   `json:"generation"`
	CarriedGen int64   `json:"carried_gen"`
	UserID string `json:"user_id"`
	TxHash string `json:"tx_hash"`
	Asset   string         `json:"asset"`
	Amount  string         `json:"amount"`
	Outcome ObserveOutcome `json:"outcome"`
	ProofID string `json:"proof_id,omitempty"`
	IdemKey string `json:"idem_key"`
	CreatedAt time.Time `json:"created_at"`
}

// CreditPosted 是否允许增加用户可用。仅 credited / 同键幂等 credited 为真。
  func (r ObserveRecord) CreditPosted() bool {
	  return r.Outcome == OutcomeCredited || r.Outcome == OutcomeIdempotent
  }

// RecycleView 只读挂钩 RecycleRec / Inventory / 观察绑定。

type RecycleView struct {
	Slot  SlotKey       `json:"slot"`
	Epoch *RecycleEpoch `json:"epoch,omitempty"`
	Bind *ObserveBind `json:"bind,omitempty"`
	RecycleRec  *addrpool.RecycleRec `json:"recycle_rec,omitempty"`
	Inventory   *InventoryHint       `json:"inventory,omitempty"`
	OpenProofs  []MiscreditProof     `json:"open_proofs,omitempty"`
	LastObserve *ObserveRecord       `json:"last_observe,omitempty"`
}

// InventoryHint 库存槽只读摘要，不回写 Inventory FSM。
type InventoryHint struct {
	Address string `json:"address"`
	Chain  string `json:"chain"`
	UserID string `json:"user_id,omitempty"`
	Status string `json:"status,omitempty"`
	CoolUntil string `json:"cool_until,omitempty"`
}

// ─────────────────────────── Requests ───────────────────────────────────────

type MarkRequest struct {
	Chain    string `json:"chain"`
	Address  string `json:"address"`
	PrevUser string `json:"prev_user"`
	ActorID string `json:"actor_id"`
	IdemKey    string    `json:"idem_key"`
	CoolUntil  time.Time `json:"cool_until"`
	RecycleRef string    `json:"recycle_ref,omitempty"`
}

type ReadyRequest struct {
	Chain string `json:"chain"`
	Address string `json:"address"`
	ActorID string `json:"actor_id"`
	Force   bool   `json:"force,omitempty"` // 强制 Ready：仍 fail-closed，开证明，不得进入成功复用
}

type AssignRequest struct {
	Chain   string `json:"chain"`
	Address string `json:"address"`
	NextUser string `json:"next_user"`
	ActorID string `json:"actor_id"`
	IdemKey string `json:"idem_key"`
}

type ObserveCreditRequest struct {
	Chain      string `json:"chain"`
	Address    string `json:"address"`
	UserID     string `json:"user_id"`
	CarriedGen int64  `json:"carried_gen"`
	TxHash     string `json:"tx_hash"`
	Asset   string `json:"asset"`
	Amount  string `json:"amount"`
	ActorID string `json:"actor_id"`
	IdemKey string `json:"idem_key"`
}

type AckProofRequest struct {
	ProofID string `json:"proof_id"`
	ActorID string `json:"actor_id"`
}

type RetireRequest struct {
	Chain   string `json:"chain"`
	Address string `json:"address"`
	ActorID string `json:"actor_id"`
}

// AuditEvent 内部审计轨迹（不写邻接包 FSM）。
type AuditEvent struct {
	EventID string  `json:"event_id"`
	Slot    SlotKey `json:"slot"`
	Kind    string  `json:"kind"`
	Actor      string    `json:"actor"`
	Detail     string    `json:"detail"`
	OccurredAt time.Time `json:"occurred_at"`
}
