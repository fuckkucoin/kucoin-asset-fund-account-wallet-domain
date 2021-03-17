package batchresiduefence

import "time"

// FenceState 围栏状态：issued → frozen → released / expired。
type FenceState string

const (
	FenceIssued FenceState = "issued"
	FenceFrozen FenceState = "frozen"
	FenceReleased FenceState = "released"
	FenceExpired FenceState = "expired"
)

// HalfState 半切案例：open → resumed / abandoned。

type HalfState string

const (
	HalfOpen    HalfState = "open"
	HalfResumed HalfState = "resumed"
	HalfAbandoned HalfState = "abandoned"
)

// ItemPhase 批次项对外只读相位，避免本包改写 wdbatch 项状态机。
type ItemPhase string

const (
	PhasePending   ItemPhase = "pending"
	PhaseSubmitted ItemPhase = "submitted"
	PhaseSkipped   ItemPhase = "skipped"
	PhaseFailed ItemPhase = "failed"
)

// ResidueFence 失败项级残差围栏。Retry/reopen 必须持有且未过期。
type ResidueFence struct {
	ID      string `json:"id"`
	BatchID string `json:"batch_id"`
	ItemIdem          string    `json:"item_idem"`
	FreezeFingerprint string    `json:"freeze_fingerprint"`
	Generation        int       `json:"generation"`
	TTL               int64     `json:"ttl_ms"`
	IssuedAt          time.Time `json:"issued_at"`
	ExpireAt time.Time `json:"expire_at"`
	FrozenAt *time.Time `json:"frozen_at,omitempty"`
	ReleasedAt *time.Time `json:"released_at,omitempty"`
	State      FenceState `json:"state"`
	Actor  string `json:"actor"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount            string `json:"amount"`
	OriginalFreezeRef string `json:"original_freeze_ref,omitempty"`
	ResumeOnly        bool   `json:"resume_only"`
	Reason string `json:"reason,omitempty"`
}

// BatchDupFreezeProof 同一 item_idem 第二冻账腿失败证明。

type BatchDupFreezeProof struct {
	ID string `json:"id"`
	BatchID       string `json:"batch_id"`
	ItemIdem      string `json:"item_idem"`
	WinnerFenceID string `json:"winner_fence_id"`
	LoserAttemptID string `json:"loser_attempt_id"`
	LoserActor string `json:"loser_actor"`
	Reason     string `json:"reason"`
	Fingerprint string `json:"fingerprint"`
	Exported    bool   `json:"exported"`
	ExportedAt *time.Time `json:"exported_at,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// HalfSubmitCase Submit 半切中断案例。Retry 必须围栏续冻，不得二次全额冻结。
type HalfSubmitCase struct {
	ID string `json:"id"`
	BatchID string `json:"batch_id"`
	ItemIdem string `json:"item_idem"`
	FenceID  string    `json:"fence_id"`
	State    HalfState `json:"state"`
	CutAt    time.Time `json:"cut_at"`
	ExpireAt time.Time `json:"expire_at"`
	ResumeAt  *time.Time `json:"resume_at,omitempty"`
	Actor     string     `json:"actor"`
	CutReason string     `json:"cut_reason"`
	FullFrozen bool `json:"full_frozen"`
}

// FreezeSlot 同一 item_idem 的唯一有效冻账槽。

type FreezeSlot struct {
	ItemIdem string `json:"item_idem"`
	BatchID  string `json:"batch_id"`
	FenceID     string     `json:"fence_id"`
	Fingerprint string     `json:"fingerprint"`
	Status      string     `json:"status"`
	HeldAt      time.Time  `json:"held_at"`
	ReleasedAt  *time.Time `json:"released_at,omitempty"`
}

// MutexHold Submit↔Retry 互斥持有。

type MutexHold struct {
	BatchID string `json:"batch_id"`
	Kind    string `json:"kind"` // submit | retry
	Holder  string `json:"holder"`
	HeldAt time.Time `json:"held_at"`
	ExpireAt time.Time `json:"expire_at"`
}

// AuditEvent 围栏审计。
type AuditEvent struct {
	ID string `json:"id"`
	BatchID string `json:"batch_id"`
	FenceID string `json:"fence_id,omitempty"`
	Event   string `json:"event"`
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
	Actor  string `json:"actor"`
	Detail string `json:"detail,omitempty"`
	At time.Time `json:"at"`
}

// BatchItemView 只读批次项快照，由 wdbatch 邻域供给。
type BatchItemView struct {
	BatchID  string    `json:"batch_id"`
	ItemIdem string    `json:"item_idem"`
	Phase    ItemPhase `json:"phase"`
	UserID string `json:"user_id"`
	Asset string `json:"asset"`
	Amount   string `json:"amount"`
	LedgerID string `json:"ledger_id,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// IssueFenceReq 为失败项签发围栏。

type IssueFenceReq struct {
	BatchID string `json:"batch_id"`
	ItemIdem string `json:"item_idem"`
	Actor string `json:"actor"`
	UserID string `json:"user_id"`
	Asset string `json:"asset"`
	Amount            string `json:"amount"`
	OriginalFreezeRef string `json:"original_freeze_ref,omitempty"`
	TTL               int64  `json:"ttl_ms,omitempty"`
}

// RetryWithFenceReq Retry 必须携带围栏身份。
type RetryWithFenceReq struct {
	BatchID  string `json:"batch_id"`
	ItemIdem string `json:"item_idem"`
	FenceID string `json:"fence_id"`
	Generation int    `json:"generation"`
	Actor      string `json:"actor"`
	AttemptID  string `json:"attempt_id,omitempty"`
	ResumeOnly bool   `json:"resume_only"`
}

// RetryWithFenceResp Retry 收口结果。

type RetryWithFenceResp struct {
	Allowed bool `json:"allowed"`
	Fence ResidueFence `json:"fence,omitempty"`
	DupProof *BatchDupFreezeProof `json:"dup_proof,omitempty"`
	Half     *HalfSubmitCase      `json:"half,omitempty"`
	ResumeOnly bool `json:"resume_only"`
	Reason string `json:"reason,omitempty"`
}

// SubmitAssertReq Submit 过闸断言，半切时登记 HalfSubmitCase。
type SubmitAssertReq struct {
	BatchID string `json:"batch_id"`
	ItemIdem   string    `json:"item_idem"`
	Actor      string    `json:"actor"`
	PhaseAfter ItemPhase `json:"phase_after"`
	HalfCut    bool      `json:"half_cut"`
	CutReason string `json:"cut_reason,omitempty"`
	FreezeRef string `json:"freeze_ref,omitempty"`
	Amount string `json:"amount,omitempty"`
	UserID string `json:"user_id,omitempty"`
	Asset  string `json:"asset,omitempty"`
}

// SubmitAssertResp Submit 断言结果。

type SubmitAssertResp struct {
	OK     bool            `json:"ok"`
	Fence  *ResidueFence   `json:"fence,omitempty"`
	Half   *HalfSubmitCase `json:"half,omitempty"`
	Reason string          `json:"reason,omitempty"`
}

// ProofExport 导出批次双冻证明。

type ProofExport struct {
	BatchID string                `json:"batch_id"`
	Proofs  []BatchDupFreezeProof `json:"proofs"`
	Count int `json:"count"`
	At time.Time `json:"at"`
}

// PatrolReport 残差巡逻报告。
type PatrolReport struct {
	ExpiredFences int `json:"expired_fences"`
	AbandonedHalf int       `json:"abandoned_half"`
	Exported      int       `json:"exported"`
	At            time.Time `json:"at"`
}

  func (f ResidueFence) Copy() ResidueFence { return f }
   func (p BatchDupFreezeProof) Copy() BatchDupFreezeProof {
	   p2 := p
	   if p.ExportedAt != nil {
		  t := *p.ExportedAt
		   p2.ExportedAt = &t
	   }
	  return p2
  }
  func (h HalfSubmitCase) Copy() HalfSubmitCase {
	  h2 := h
	   if h.ResumeAt != nil {
		   t := *h.ResumeAt
		  h2.ResumeAt = &t
	   }
	   return h2
   }
