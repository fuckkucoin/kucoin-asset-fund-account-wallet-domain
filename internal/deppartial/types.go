package deppartial

import "time"

// ──────────────────────────────────────────────────
// 批次残余状态
// ──────────────────────────────────────────────────

type BatchResidueState string

const (
	BRSubmitted BatchResidueState = "submitted"
	BRResidueOpen BatchResidueState = "residue_open"
	BRReopening BatchResidueState = "reopening"
	BRClosed BatchResidueState = "closed"
)

// ──────────────────────────────────────────────────
// 残余行状态
// ──────────────────────────────────────────────────

type ResidueState string

const (
	ResOpen     ResidueState = "open"
	ResCredited ResidueState = "credited"
	ResAbandoned ResidueState = "abandoned"
	ResEscalated ResidueState = "escalated"
)

// ──────────────────────────────────────────────────
// 租约状态
// ──────────────────────────────────────────────────

type LeaseStatus string

const (
	LeaseHeld LeaseStatus = "held"
	LeaseReleased LeaseStatus = "released"
	LeaseExpired  LeaseStatus = "expired"
)

// ──────────────────────────────────────────────────
// 一致证明状态
// ──────────────────────────────────────────────────

type ProofStatus string

const (
	ProofPending ProofStatus = "pending"
	ProofPassed  ProofStatus = "passed"
	ProofRejected ProofStatus = "rejected"
)

// ──────────────────────────────────────────────────
// 残余类型
// ──────────────────────────────────────────────────

type ResidueKind string

const (
	KindSeenNoCredit ResidueKind = "seen_no_credit"
	KindCreditNoItem  ResidueKind = "credit_no_item"
	KindMismatchState ResidueKind = "mismatch_state"
)

// ──────────────────────────────────────────────────
// ObserveResidue — 观察残余行
// ──────────────────────────────────────────────────

type ObserveResidue struct {
	ID      string `json:"id"`
	BatchID string `json:"batch_id"`
	Seq int `json:"seq"`
	Chain string `json:"chain"`
	TxHash string `json:"tx_hash"`
	Asset  string      `json:"asset"`
	UserID string      `json:"user_id"`
	Kind   ResidueKind `json:"kind"`
	State      ResidueState `json:"state"`
	LedgerID   string       `json:"ledger_id,omitempty"`
	Amount     string       `json:"amount,omitempty"`
	Generation int64        `json:"generation"`
	Reason string `json:"reason,omitempty"`
	DetectedAt time.Time `json:"detected_at"`
	ResolvedAt time.Time `json:"resolved_at,omitempty"`
	ResolvedBy string    `json:"resolved_by,omitempty"`
}

// ──────────────────────────────────────────────────
// ReopenLease — 重开互斥租约
// ──────────────────────────────────────────────────

type ReopenLease struct {
	ID      string `json:"id"`
	BatchID string `json:"batch_id"`
	Holder     string      `json:"holder"`
	Generation int64       `json:"generation"`
	Status     LeaseStatus `json:"status"`
	AcquiredAt time.Time   `json:"acquired_at"`
	ExpiresAt  time.Time   `json:"expires_at"`
	ReleasedAt time.Time `json:"released_at,omitempty"`
	Reason     string    `json:"reason,omitempty"`
}

// ──────────────────────────────────────────────────
// CreditAlignProof — 贷记一致证明
// ──────────────────────────────────────────────────

type CreditAlignProof struct {
	ID string `json:"id"`
	BatchID string `json:"batch_id"`
	Seq int `json:"seq"`
	Chain string `json:"chain"`
	TxHash   string `json:"tx_hash"`
	LedgerID string `json:"ledger_id"`
	ItemState   string      `json:"item_state"`
	LedgerState string      `json:"ledger_state"`
	Status      ProofStatus `json:"status"`
	Aligned     bool        `json:"aligned"`
	Checker     string      `json:"checker"`
	CheckedAt   time.Time   `json:"checked_at"`
	Note string `json:"note,omitempty"`
}

// ──────────────────────────────────────────────────
// SeenFence — 观察围栏
// ──────────────────────────────────────────────────

type SeenFence struct {
	Chain  string `json:"chain"`
	TxHash string `json:"tx_hash"`
	BatchID string `json:"batch_id"`
	Generation int64 `json:"generation"`
	Closed bool `json:"closed"`
}

// ──────────────────────────────────────────────────
// 批次残余聚合视图
// ──────────────────────────────────────────────────

type BatchResidueView struct {
	BatchID string `json:"batch_id"`
	State         BatchResidueState `json:"state"`
	TotalItems    int               `json:"total_items"`
	ResidueCount  int               `json:"residue_count"`
	CreditedCount int               `json:"credited_count"`
	OpenCount int `json:"open_count"`
	HasLease    bool   `json:"has_lease"`
	LeaseHolder string `json:"lease_holder,omitempty"`
	ProofCount  int    `json:"proof_count"`
	AllAligned  bool   `json:"all_aligned"`
	CanClose bool `json:"can_close"`
}

// ──────────────────────────────────────────────────
// 请求与响应
// ──────────────────────────────────────────────────

type ScanReq struct {
	BatchID string `json:"batch_id"`
	Actor string `json:"actor"`
}

type ReopenReq struct {
	BatchID string `json:"batch_id"`
	Holder string `json:"holder"`
	Approver string `json:"approver"`
	Reason   string `json:"reason"`
}

type RepairCreditReq struct {
	BatchID string `json:"batch_id"`
	ResidueID string `json:"residue_id"`
	Actor     string `json:"actor"`
	LedgerID  string `json:"ledger_id"`
	Amount     string `json:"amount"`
	Generation int64  `json:"generation"`
}

type AbandonReq struct {
	BatchID   string `json:"batch_id"`
	ResidueID string `json:"residue_id"`
	Actor string `json:"actor"`
	Reason string `json:"reason"`
}

type EscalateReq struct {
	BatchID string `json:"batch_id"`
	ResidueID string `json:"residue_id"`
	Actor string `json:"actor"`
	Reason string `json:"reason"`
}

type AlignCheckReq struct {
	BatchID string `json:"batch_id"`
	Seq     int    `json:"seq"`
	Checker string `json:"checker"`
}

type CloseReq struct {
	BatchID string `json:"batch_id"`
	Actor string `json:"actor"`
}

type ProofExportReq struct {
	BatchID string `json:"batch_id"`
	Format  string `json:"format,omitempty"`
}

type ProofExportResp struct {
	BatchID string             `json:"batch_id"`
	Proofs  []CreditAlignProof `json:"proofs"`
	Summary ProofSummary       `json:"summary"`
}

type ProofSummary struct {
	Total    int `json:"total"`
	Passed   int `json:"passed"`
	Rejected int `json:"rejected"`
	Pending  int `json:"pending"`
}

  func (r ObserveResidue) Copy() ObserveResidue     { return r }
   func (l ReopenLease) Copy() ReopenLease           { return l }
   func (p CreditAlignProof) Copy() CreditAlignProof { return p }

  func (f SeenFence) Copy() SeenFence { return f }
