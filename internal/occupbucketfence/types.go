package occupbucketfence

import "time"

// FenceState 围栏状态机。
type FenceState string

const (
	FenceIssued FenceState = "issued"
	FenceBound    FenceState = "bound"
	FenceConsumed FenceState = "consumed"
	FenceExpired FenceState = "expired"
)

// Terminal 围栏是否终态。
  func (s FenceState) Terminal() bool {
	   return s == FenceConsumed || s == FenceExpired
  }

// CanBind issued 可绑定。
   func (s FenceState) CanBind() bool {
	   return s == FenceIssued
  }

// CanConsume bound 可消费。
  func (s FenceState) CanConsume() bool {
	   return s == FenceBound
   }

// BreakState 击穿证明状态机。

type BreakState string

const (
	BreakOpen   BreakState = "open"
	BreakProven BreakState = "proven"
	BreakClosed BreakState = "closed"
)

// Terminal 证明是否终态。
  func (s BreakState) Terminal() bool {
	  return s == BreakClosed
   }

// HalfState 半截续提状态。

type HalfState string

const (
	HalfOpen     HalfState = "open"
	HalfResumed  HalfState = "resumed"
	HalfRejected HalfState = "rejected"
)

// BucketFence lot 级桶围栏。
type BucketFence struct {
	FenceID string `json:"bucket_fence_id"`
	LotID             string `json:"lot_id"`
	UserID            string `json:"user_id"`
	Asset             string `json:"asset"`
	BucketFingerprint string `json:"bucket_fingerprint"`
	Generation        int64  `json:"generation"`
	TTLSeconds        int64  `json:"ttl_seconds"`
	State FenceState `json:"state"`
	BoundHoldID string `json:"bound_hold_id,omitempty"`
	ConsumedBy string `json:"consumed_by,omitempty"`
	IssuedAt   time.Time  `json:"issued_at"`
	BoundAt    *time.Time `json:"bound_at,omitempty"`
	ConsumedAt *time.Time `json:"consumed_at,omitempty"`
	ExpiresAt  time.Time  `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// OccupancyBreakProof 三桶守恒击穿证明。
type OccupancyBreakProof struct {
	ProofID string `json:"proof_id"`
	LotID   string `json:"lot_id"`
	FenceID string     `json:"bucket_fence_id"`
	State   BreakState `json:"state"`
	Kind     string `json:"kind"`
	Inflight string `json:"inflight"`
	Available string `json:"available"`
	Void      string `json:"void"`
	Sum string `json:"sum"`
	Total      string `json:"total"`
	Delta      string `json:"delta"`
	ConflictOp string `json:"conflict_op"`
	WinnerOp string    `json:"winner_op"`
	Reason   string    `json:"reason"`
	Digest   string    `json:"digest"`
	OpenedAt time.Time `json:"opened_at"`
	ProvenAt *time.Time `json:"proven_at,omitempty"`
	ClosedAt *time.Time `json:"closed_at,omitempty"`
}

// HalfBucketCase Hold 成功后桶落库中断的半截案。
type HalfBucketCase struct {
	CaseID string `json:"case_id"`
	LotID   string    `json:"lot_id"`
	HoldID  string    `json:"hold_id"`
	FenceID string    `json:"bucket_fence_id"`
	State   HalfState `json:"state"`
	Amount string `json:"amount"`
	Reason    string     `json:"reason"`
	OpenedAt  time.Time  `json:"opened_at"`
	ResumedAt *time.Time `json:"resumed_at,omitempty"`
}

// IssueFenceReq 签发围栏。

type IssueFenceReq struct {
	LotID string `json:"lot_id"`
	TTLSeconds int64 `json:"ttl_seconds"`
}

// HoldWithFenceReq 带围栏 Hold。

type HoldWithFenceReq struct {
	LotID string `json:"lot_id"`
	UserID string `json:"user_id"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	RiskClass string `json:"risk_class"`
	ObserveRef string `json:"observe_ref"`
	HoldID     string `json:"hold_id"`
	FenceID    string `json:"bucket_fence_id"`
	IdemKey    string `json:"idem_key"`
	TTLSeconds int64  `json:"ttl_seconds"`
}

// ReleaseAssertReq 带围栏 Release。

type ReleaseAssertReq struct {
	LotID     string `json:"lot_id"`
	HoldID    string `json:"hold_id"`
	ReleaseID string `json:"release_id"`
	FenceID   string `json:"bucket_fence_id"`
	Amount      string `json:"amount"`
	IdemKey     string `json:"idem_key"`
	Fingerprint string `json:"bucket_fingerprint"`
	Generation  int64  `json:"generation"`
}

// ReclassAssertReq 带围栏 Reclass。
type ReclassAssertReq struct {
	LotID string `json:"lot_id"`
	HoldID    string `json:"hold_id"`
	ReclassID string `json:"reclass_id"`
	FenceID   string `json:"bucket_fence_id"`
	Amount     string `json:"amount"`
	ToClass    string `json:"to_class"`
	ReasonCode string `json:"reason_code"`
	IdemKey string `json:"idem_key"`
	Fingerprint string `json:"bucket_fingerprint"`
	Generation int64 `json:"generation"`
}

// ExportProofReq 证明导出。
type ExportProofReq struct {
	LotID string `json:"lot_id"`
	ProofID string `json:"proof_id"`
}

// HoldWithFenceResp Hold 闸响应。
type HoldWithFenceResp struct {
	Fence  *BucketFence    `json:"fence"`
	HoldID string          `json:"hold_id"`
	LotID  string          `json:"lot_id"`
	Half   *HalfBucketCase `json:"half_case,omitempty"`
	Inflight string `json:"inflight"`
	Available string `json:"available"`
	Void string `json:"void"`
}

// MutateResp Release/Reclass 响应。

type MutateResp struct {
	Fence *BucketFence `json:"fence"`
	Proof *OccupancyBreakProof `json:"proof,omitempty"`
	Winner   string `json:"winner,omitempty"`
	LotID    string `json:"lot_id"`
	Inflight string `json:"inflight"`
	Available string `json:"available"`
	Void string `json:"void"`
	Conserve bool `json:"conserve"`
}
