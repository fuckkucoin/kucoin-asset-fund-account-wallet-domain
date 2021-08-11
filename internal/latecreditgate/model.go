package latecreditgate

import (
	"time"
	"account-assets.kucoin.net/funding/internal/reclaim"
)

// OutputKey 复用 reclaim 只读视图的输出键。

type OutputKey = reclaim.OutputKey

// ─────────────────────────── Generation ─────────────────────────────────────
// GenState 输出键迟贷记世代状态。
// free → occupied : OccupyWithGen
// occupied → credited : CreditDurable（仅当未占用冲突）
// occupied → reclaimed : Reclaim / Resolve 写路径
// free → credited : CheckCredit / AutoCredit 在无占用时
// 任意非终态 → 冲突开 RaceProof，credit 路径 fail-closed
type GenState string

const (
	GenFree GenState = "free"
	GenOccupied GenState = "occupied"
	GenCredited GenState = "credited"
	GenReclaimed GenState = "reclaimed"
)

// LateCreditGeneration 按 output_key 持久化的迟贷记世代。

type LateCreditGeneration struct {
	GenID     string    `json:"gen_id"`
	OutputKey OutputKey `json:"output_key"`
	Generation int64    `json:"generation"`
	State      GenState `json:"state"`
	HolderKind string   `json:"holder_kind"` // occupy | credit | auto_credit
	HolderActor string `json:"holder_actor"`
	OccupancyID string `json:"occupancy_id,omitempty"`
	CaseID      string `json:"case_id,omitempty"`
	Asset      string `json:"asset,omitempty"`
	Amount     string `json:"amount,omitempty"`
	FenceEpoch int64  `json:"fence_epoch"`
	FenceToken string    `json:"fence_token"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// CanOccupy 当前世代是否允许占用。

   func (g LateCreditGeneration) CanOccupy() bool {
	  return g.State == GenFree
  }

// CanCredit 当前世代是否允许贷记。occupied / reclaimed 一律 fail-closed。

   func (g LateCreditGeneration) CanCredit() bool {
	   return g.State == GenFree
   }

// IsTerminal 终态不可再推进写路径。

   func (g LateCreditGeneration) IsTerminal() bool {
	  return g.State == GenCredited || g.State == GenReclaimed
  }

// ─────────────────────────── Fence ──────────────────────────────────────────
// FenceToken 写路径必须携带的围栏凭证。

type FenceToken struct {
	Epoch int64  `json:"epoch"`
	Token string `json:"token"`
}

// FenceMode 围栏策略模式。

type FenceMode string

const (
	FenceForced     FenceMode = "forced"      // 写路径强制校验
	FenceLegacyRead FenceMode = "legacy_read" // 仅显式只读允许省略
)

// FencePolicy 强制围栏策略闸。
type FencePolicy struct {
	Mode            FenceMode `json:"mode"`
	MinEpoch        int64     `json:"min_epoch"`
	RequireToken    bool      `json:"require_token"`
	AllowLegacyRead bool      `json:"allow_legacy_read"`
}

// DefaultFencePolicy 默认：写路径强制 epoch>0 且 token 非空。

   func DefaultFencePolicy() FencePolicy {
	   return FencePolicy{
		  Mode: FenceForced,
		   MinEpoch: 1,
		  RequireToken: true,
		  AllowLegacyRead: true,
	  }
  }

// ─────────────────────────── Race ───────────────────────────────────────────
// RaceKind 竞态事件类型。
type RaceKind string

const (
	RaceOccupyVsCredit RaceKind = "occupy_vs_credit"
	RaceOccupyVsAutoCredit RaceKind = "occupy_vs_auto_credit"
	RaceDoubleOccupy       RaceKind = "double_occupy"
	RaceFenceOmit          RaceKind = "fence_omit"
	RaceGenRollback        RaceKind = "gen_rollback"
)

// RaceState 竞态队列状态。
type RaceState string

const (
	RaceOpen  RaceState = "open"
	RaceAcked RaceState = "acked"
)

// RaceEvent 持久化竞态事件，进程重启后仍可 Drain。

type RaceEvent struct {
	EventID    string    `json:"event_id"`
	OutputKey  OutputKey `json:"output_key"`
	GenID      string    `json:"gen_id"`
	Generation int64     `json:"generation"`
	Kind       RaceKind  `json:"kind"`
	State  RaceState `json:"state"`
	ActorA string    `json:"actor_a"`
	ActorB string    `json:"actor_b"`
	Detail    string     `json:"detail,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	AckedAt   *time.Time `json:"acked_at,omitempty"`
}

// LateCreditRaceProof 可导出竞态证明仓。

type LateCreditRaceProof struct {
	ProofID string `json:"proof_id"`
	OutputKey OutputKey `json:"output_key"`
	GenID     string    `json:"gen_id"`
	Generation  int64    `json:"generation"`
	EventID     string   `json:"event_id"`
	Kind        RaceKind `json:"kind"`
	OccupyActor string   `json:"occupy_actor"`
	CreditActor string     `json:"credit_actor"`
	FailClosed  bool       `json:"fail_closed"`
	Summary     string     `json:"summary"`
	OpenedAt    time.Time  `json:"opened_at"`
	ExportedAt  *time.Time `json:"exported_at,omitempty"`
}

// ─────────────────────────── Requests ───────────────────────────────────────
// OccupyRequest OccupyWithGen 请求。

type OccupyRequest struct {
	OutputKey OutputKey `json:"output_key"`
	ActorID   string    `json:"actor_id"`
	CaseID    string    `json:"case_id"`
	OccupancyID string `json:"occupancy_id"`
	Asset       string `json:"asset"`
	Amount      string `json:"amount"`
	Fence FenceToken `json:"fence"`
}

// CreditRequest CheckCreditDurable / AutoCredit 请求。
type CreditRequest struct {
	OutputKey OutputKey `json:"output_key"`
	ActorID string `json:"actor_id"`
	Kind    string `json:"kind"` // credit | auto_credit
	Asset   string `json:"asset"`
	Amount  string `json:"amount"`
	IdemKey string `json:"idem_key"`
	Fence FenceToken `json:"fence"`
}

// WritePath 写路径名称（Reserve / Reclaim / Resolve / AutoCredit）。
type WritePath string

const (
	PathReserve    WritePath = "reserve"
	PathReclaim    WritePath = "reclaim"
	PathResolve    WritePath = "resolve"
	PathAutoCredit WritePath = "auto_credit"
	PathOccupy     WritePath = "occupy"
	PathCredit WritePath = "credit"
)

// WriteRequest 通用写路径请求（强制围栏）。
type WriteRequest struct {
	Path      WritePath  `json:"path"`
	OutputKey OutputKey  `json:"output_key"`
	ActorID   string     `json:"actor_id"`
	Fence     FenceToken `json:"fence"`
	NextState GenState `json:"next_state,omitempty"`
}

// ResolveRaceRequest 关闭竞态事件。

type ResolveRaceRequest struct {
	EventID string `json:"event_id"`
	ActorID string `json:"actor_id"`
}

// OutputView 只读摘要（挂钩 ReclaimCase / Occupancy / Timeline）。

type OutputView struct {
	OutputKey OutputKey `json:"output_key"`
	Generation *LateCreditGeneration `json:"generation,omitempty"`
	Occupancy *reclaim.OccupancyView `json:"occupancy,omitempty"`
	Case      *reclaim.CaseView `json:"case,omitempty"`
	OpenRaces []RaceEvent       `json:"open_races,omitempty"`
	Proofs   []LateCreditRaceProof `json:"proofs,omitempty"`
	Timeline *reclaim.ViewTimeline `json:"timeline,omitempty"`
}
