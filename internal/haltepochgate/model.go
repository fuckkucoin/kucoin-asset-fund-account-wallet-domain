package haltepochgate

import (
	"strings"
	"time"
)

// ScopeKey 熔断作用域键。与 custodyhalt Scope 一致，本包只读挂钩。
type ScopeKey struct {
	Scope string `json:"scope"`
	Chain string `json:"chain,omitempty"`
	Asset string `json:"asset,omitempty"`
}

   func (k ScopeKey) String() string {
	  sc := strings.ToLower(strings.TrimSpace(k.Scope))
	  if sc == "" {
		   sc = "global"
	  }
	   ch := strings.ToUpper(strings.TrimSpace(k.Chain))
	  as := strings.ToUpper(strings.TrimSpace(k.Asset))
	  switch sc {
	   case "chain":
		  return "chain:" + ch
	  case "asset":
		  return "asset:" + as
	   case "pair":
		   return "pair:" + ch + "/" + as
	  default:
		   return "global"
	  }
  }

  func (k ScopeKey) Empty() bool {
	   return strings.TrimSpace(k.Scope) == "" && strings.TrimSpace(k.Chain) == "" && strings.TrimSpace(k.Asset) == ""
   }

  func NormalizeScope(scope, chain, asset string) ScopeKey {
	  sc := strings.ToLower(strings.TrimSpace(scope))
	   if sc == "" {
		   sc = "global"
	  }
	   return ScopeKey{
		  Scope: sc,
		  Chain: strings.ToUpper(strings.TrimSpace(chain)),
		   Asset: strings.ToUpper(strings.TrimSpace(asset)),
	  }
   }

// EpochState 熔断世代状态。
// idle → engaged : EngageWithEpoch
// engaged → clearing : ClearWithEpoch（持互斥 + quorum）
// clearing → cleared : 吊销在途意图后封窗
// cleared → reengaged : 再 Engage（抬升世代）
// cleared → idle : SealResume（封窗完成，新世代可放行）
// reengaged 等同 engaged

type EpochState string

const (
	EpochIdle    EpochState = "idle"
	EpochEngaged EpochState = "engaged"
	EpochClearing EpochState = "clearing"
	EpochCleared   EpochState = "cleared"
	EpochReengaged EpochState = "reengaged"
)

   func (s EpochState) Valid() bool {
	  switch s {
	  case EpochIdle,
		  EpochEngaged, EpochClearing, EpochCleared, EpochReengaged:
		   return true
	  default:
		   return false
	  }
  }

   func (s EpochState) Halted() bool {
	   return s == EpochEngaged || s == EpochReengaged || s == EpochClearing
  }

// WindowSealed 清窗期内资金入口必须拒。idle 且世代>0 且已封窗才可按新 epoch 放行。
  func (s EpochState) WindowSealed() bool {
	  return s == EpochClearing || s == EpochCleared
   }

   func (s EpochState) AllowsNewLane(sealed bool) bool {
	  if s.Halted() || s.WindowSealed() {
		   return false
	  }
	  return s == EpochIdle && sealed
   }

  func (s EpochState) AllowsEngage() bool {
	  return s == EpochIdle || s == EpochCleared
   }

   func (s EpochState) AllowsClear() bool {
	   return s == EpochEngaged || s == EpochReengaged
  }

// HaltEpoch 按作用域持久化的熔断世代。
type HaltEpoch struct {
	EpochID string `json:"epoch_id"`
	Scope      ScopeKey `json:"scope"`
	Generation int64    `json:"generation"`
	State       EpochState `json:"state"`
	PrevGen     int64      `json:"prev_gen"`
	LaneOpen    bool       `json:"lane_open"`
	MutexHolder string     `json:"mutex_holder,omitempty"`
	MutexKind   string     `json:"mutex_kind,omitempty"`
	Actor  string `json:"actor"`
	Reason string `json:"reason"`
	QuorumID string `json:"quorum_id,omitempty"`
	IdemKey  string `json:"idem_key,omitempty"`
	WindowCase string    `json:"window_case_id,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	ClearedAt time.Time `json:"cleared_at,omitempty"`
	EngagedAt time.Time `json:"engaged_at,omitempty"`
}

  func (e HaltEpoch) Current() int64 { return e.Generation }

   func (e HaltEpoch) BindOK(carried int64) error {
	   if carried <= 0 {
		   return ErrEpochMissing
	   }
	  if carried != e.Generation {
		   return ErrEpochStale
	   }
	   return nil
   }

  func (e HaltEpoch) LaneConclusion() LaneConclusion {
	  blocked := e.State.Halted() || e.State.WindowSealed() || !e.LaneOpen
	  reason := ""
	  switch {
	   case e.State.Halted():
		   reason = "halt_engaged"
	  case e.State.WindowSealed():
		   reason = "clear_window_sealed"
	  case !e.LaneOpen:
		   reason = "lane_not_open"
	   default:
		   reason = "lane_open"
	  }
	  return LaneConclusion{
		  Scope: e.Scope.String(),
		  Generation: e.Generation,
		  State:    e.State,
		   Blocked:  blocked,
		   LaneOpen: e.LaneOpen && !blocked,
		   Reason:   reason,
	  }
   }

// LaneConclusion Lane 读闸与 epoch 一致的结论。
type LaneConclusion struct {
	Scope string `json:"scope"`
	Lane string `json:"lane,omitempty"`
	Generation int64      `json:"generation"`
	State      EpochState `json:"state"`
	Blocked    bool       `json:"blocked"`
	LaneOpen   bool       `json:"lane_open"`
	FailClosed bool       `json:"fail_closed,omitempty"`
	Reason     string     `json:"reason"`
	ProofID    string     `json:"proof_id,omitempty"`
}

// FundLane 充/提/清扫入口。

type FundLane string

const (
	LaneDeposit  FundLane = "deposit"
	LaneWithdraw FundLane = "withdraw"
	LaneSweep FundLane = "sweep"
)

  func (l FundLane) Valid() bool {
	   switch l {
	  case LaneDeposit, LaneWithdraw, LaneSweep:
		  return true
	  default:
		  return false
	  }
   }

// SlipKind 滑窗证明类别。

type SlipKind string

const (
	SlipClearWindowDeposit SlipKind = "clear_window_deposit"
	SlipClearWindowWithdraw SlipKind = "clear_window_withdraw"
	SlipClearWindowSweep    SlipKind = "clear_window_sweep"
	SlipStaleEpochBroadcast SlipKind = "stale_epoch_broadcast"
	SlipStaleEpochCredit SlipKind = "stale_epoch_credit"
	SlipMissingEpoch    SlipKind = "missing_epoch"
	SlipEngageClearRace SlipKind = "engage_clear_race"
	SlipLaneMismatch       SlipKind = "lane_epoch_mismatch"
	SlipReadGateFail       SlipKind = "read_gate_fail"
	SlipBridgeClearAttempt SlipKind = "bridge_clear_attempt"
)

   func (k SlipKind) Valid() bool {
	   switch k {
	  case SlipClearWindowDeposit, SlipClearWindowWithdraw, SlipClearWindowSweep,
		   SlipStaleEpochBroadcast, SlipStaleEpochCredit, SlipMissingEpoch,
		   SlipEngageClearRace,
		  SlipLaneMismatch, SlipReadGateFail, SlipBridgeClearAttempt:
		  return true
	  default:
		  return false
	  }
   }

   func SlipKindForLane(lane FundLane) SlipKind {
	  switch lane {
	  case LaneWithdraw:
		  return SlipClearWindowWithdraw
	   case LaneSweep:
		   return SlipClearWindowSweep
	  default:
		   return SlipClearWindowDeposit
	   }
   }

// SlipState 证明仓状态。
type SlipState string

const (
	SlipOpen SlipState = "open"
	SlipProven SlipState = "proven"
	SlipClosed SlipState = "closed"
)

  func (s SlipState) Valid() bool {
	   return s == SlipOpen || s == SlipProven || s == SlipClosed
   }

// SlipProof Clear↔Engage 滑窗证明。

type SlipProof struct {
	ProofID    string   `json:"proof_id"`
	EpochID    string   `json:"epoch_id"`
	Scope      ScopeKey `json:"scope"`
	Generation int64    `json:"generation"`
	CarriedGen int64    `json:"carried_gen"`
	Kind SlipKind `json:"kind"`
	State SlipState `json:"state"`
	Lane     FundLane `json:"lane"`
	IntentID string   `json:"intent_id,omitempty"`
	CaseID   string   `json:"case_id,omitempty"`
	Actor   string `json:"actor,omitempty"`
	Summary string `json:"summary"`
	FailClosed  bool      `json:"fail_closed"`
	Disposition string    `json:"disposition"`
	Digest      string    `json:"digest"`
	OpenedAt    time.Time `json:"opened_at"`
	ProvenAt    time.Time `json:"proven_at,omitempty"`
	ClosedAt    time.Time `json:"closed_at,omitempty"`
	ExportedAt  time.Time `json:"exported_at,omitempty"`
}

// WindowState 清窗案状态。
type WindowState string

const (
	WindowOpen WindowState = "open"
	WindowSealing WindowState = "sealing"
	WindowSealed  WindowState = "sealed"
	WindowConflict WindowState = "conflict"
)

  func (s WindowState) Valid() bool {
	  switch s {
	  case WindowOpen, WindowSealing, WindowSealed, WindowConflict:
		  return true
	   default:
		  return false
	  }
   }

// ClearWindowCase Clear 成功后的封窗案。
type ClearWindowCase struct {
	CaseID string `json:"case_id"`
	EpochID string `json:"epoch_id"`
	Scope ScopeKey `json:"scope"`
	ClearGen int64 `json:"clear_gen"`
	PrevGen        int64       `json:"prev_gen"`
	State          WindowState `json:"state"`
	RevokedIntents int         `json:"revoked_intents"`
	SlipCount      int         `json:"slip_count"`
	Actor    string    `json:"actor"`
	Reason   string    `json:"reason"`
	OpenedAt time.Time `json:"opened_at"`
	SealedAt time.Time `json:"sealed_at,omitempty"`
}

// IntentState 在途资金意图。

type IntentState string

const (
	IntentAllowed   IntentState = "allowed"
	IntentRevoked   IntentState = "revoked"
	IntentBroadcast IntentState = "broadcast"
	IntentCredited  IntentState = "credited"
	IntentRejected IntentState = "rejected"
)

  func (s IntentState) Terminal() bool {
	   return s == IntentRevoked || s == IntentBroadcast || s == IntentCredited || s == IntentRejected
  }

// FundIntent 绑定 halt_epoch 的充/提/清扫在途意图。
type FundIntent struct {
	IntentID string `json:"intent_id"`
	Scope      ScopeKey    `json:"scope"`
	Lane       FundLane    `json:"lane"`
	Generation int64       `json:"generation"`
	State      IntentState `json:"state"`
	Account    string      `json:"account,omitempty"`
	Asset      string      `json:"asset,omitempty"`
	Amount     string      `json:"amount,omitempty"`
	IdemKey   string    `json:"idem_key,omitempty"`
	AllowTick string    `json:"allow_ticket,omitempty"`
	ProofID   string    `json:"proof_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

   func (i FundIntent) StaleVersus(gen int64) bool {
	  return i.Generation != gen
  }

// QuorumVote Clear 法定人数投票（本包自持，不改写 r10 误清四眼编排）。

type QuorumVote struct {
	VoteID string `json:"vote_id"`
	QuorumID string `json:"quorum_id"`
	EpochID string `json:"epoch_id"`
	Actor string `json:"actor"`
	Approve   bool      `json:"approve"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

// QuorumBundle Clear 投票集合。
type QuorumBundle struct {
	QuorumID string `json:"quorum_id"`
	EpochID  string `json:"epoch_id"`
	Scope ScopeKey `json:"scope"`
	Threshold int          `json:"threshold"`
	Votes     []QuorumVote `json:"votes"`
	Satisfied bool         `json:"satisfied"`
	CreatedAt time.Time `json:"created_at"`
}

   func (b QuorumBundle) DistinctApprovals() int {
	   seen := map[string]struct{}{}
	  n := 0
	   for _, v := range b.Votes {
		   if !v.Approve {
			  continue
		   }
		  a := strings.TrimSpace(v.Actor)
		  if a == "" {
			   continue
		   }
		   if _, ok := seen[a]; ok {
			  continue
		  }
		   seen[a] = struct{}{}
		   n++
	   }
	  return n
  }

// AuditEvent 本包审计；不回写 custodyhalt History。

type AuditEvent struct {
	EventID string `json:"event_id"`
	Kind    string `json:"kind"`
	Scope  string `json:"scope"`
	Actor  string `json:"actor"`
	Detail string `json:"detail"`
	At         time.Time `json:"at"`
	Generation int64     `json:"generation"`
}

// EngageRequest EngageWithEpoch 入参。

type EngageRequest struct {
	Scope string
	Chain string
	Asset string
	Actor  string
	Reason string
	IdemKey string
}

// ClearRequest ClearWithEpoch 入参。
type ClearRequest struct {
	Scope string
	Chain string
	Asset string
	Actor string
	Reason     string
	IdemKey    string
	Force      bool
	FromBridge bool
}

// LaneAssertRequest 充/提/清扫读闸复验。
type LaneAssertRequest struct {
	Scope string
	Chain string
	Asset string
	Lane       FundLane
	CarriedGen int64
	Account    string
	Amount     string
	IdemKey    string
	AllowTick string
	IntentID string
}

// BroadcastRequest 广播复验。
type BroadcastRequest struct {
	IntentID string
	CarriedGen int64
	Actor      string
}

// CreditRequest 贷记复验。
type CreditRequest struct {
	IntentID   string
	CarriedGen int64
	Actor      string
}
