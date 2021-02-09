package airimportgate

import "time"

// ─── 导入世代租约 ────────────────────────────────────────────────────────────
// LeaseState 导入租约状态机。

type LeaseState string

const (
	LeaseOpen LeaseState = "open"
	LeaseImporting LeaseState = "importing"
	LeaseImported  LeaseState = "imported"
	LeaseAborted   LeaseState = "aborted"
	LeaseExpired   LeaseState = "expired"
)

   func (s LeaseState) Terminal() bool {
	  return s == LeaseImported || s == LeaseAborted || s == LeaseExpired
   }

// ImportLease 一次密封导入的独占世代租约。
// 同 Session 最多一条 open/importing 租约；冲突时败者落 PhaseConflictProof。
type ImportLease struct {
	LeaseID string `json:"lease_id"`
	SessionID string `json:"session_id"`
	ImportGen string `json:"import_gen"` // air_import_gen，外部传入，不能过期
	BlobDigest string     `json:"blob_digest"` // sha256(sealed_blob) hex
	Actor      string     `json:"actor"`
	State      LeaseState `json:"state"`
	ExpiresAt  time.Time  `json:"expires_at"`
	ImportedAt *time.Time `json:"imported_at,omitempty"`
	AbortedAt  *time.Time `json:"aborted_at,omitempty"`
	AbortReason string `json:"abort_reason,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

   func (l ImportLease) Copy() ImportLease {
	   cp := l
	  if l.ImportedAt != nil {
		   t := *l.ImportedAt
		   cp.ImportedAt = &t
	  }
	   if l.AbortedAt != nil {
		  t := *l.AbortedAt
		   cp.AbortedAt = &t
	  }
	   return cp
  }

// ─── 密封 blob 摘要（重放挡） ──────────────────────────────────────────────
// SealedBlobDigest 密封 blob 摘要记录，全局唯一，只追加，用于重放检测。

type SealedBlobDigest struct {
	DigestID string `json:"digest_id"`
	DigestHex string `json:"digest_hex"` // sha256(blob) hex，唯一索引
	SessionID string `json:"session_id"`
	LeaseID    string    `json:"lease_id"`
	ImportedBy string    `json:"imported_by"`
	ImportedAt time.Time `json:"imported_at"`
}

   func (d SealedBlobDigest) Copy() SealedBlobDigest { return d }

// ─── 相位冲突证明 ──────────────────────────────────────────────────────────
// ConflictKind 冲突类型。
type ConflictKind string

const (
	ConflictImportVsAbort ConflictKind = "import_vs_abort"
	ConflictImportVsClose ConflictKind = "import_vs_close"
	ConflictImportVsExport ConflictKind = "import_vs_export"
)

// ConflictState 冲突证明状态。

type ConflictState string

const (
	ConflictOpen ConflictState = "open"
	ConflictProven ConflictState = "proven"
	ConflictClosed ConflictState = "closed"
)

// PhaseConflictProof Import 与 Abort/Close/Export 并发时的冲突证明。
// 败者操作落此记录；胜者正常推进相位。

type PhaseConflictProof struct {
	ConflictID string `json:"conflict_id"`
	SessionID string `json:"session_id"`
	ConflictKind ConflictKind `json:"conflict_kind"`
	State ConflictState `json:"state"`
	WinnerOp    string `json:"winner_op"`
	WinnerActor string `json:"winner_actor"`
	LoserOp string `json:"loser_op"`
	LoserActor string `json:"loser_actor"`
	PhaseAtConflict string `json:"phase_at_conflict"`
	Note string `json:"note,omitempty"`
	ProvenAt *time.Time `json:"proven_at,omitempty"`
	ClosedAt *time.Time `json:"closed_at,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

   func (p PhaseConflictProof) Copy() PhaseConflictProof {
	   cp := p
	  if p.ProvenAt != nil {
		   t := *p.ProvenAt
		   cp.ProvenAt = &t
	  }
	  if p.ClosedAt != nil {
		  t := *p.ClosedAt
		  cp.ClosedAt = &t
	  }
	  return cp
   }

// ─── 法定人数绑定世代 ─────────────────────────────────────────────────────
// QuorumBind 导入时签名法定人数与 Session Threshold 绑定的世代记录。
// 若 actual_quorum < threshold，QuorumPassed=false，导入被拒绝。

type QuorumBind struct {
	BindID    string `json:"bind_id"`
	SessionID string `json:"session_id"`
	LeaseID   string `json:"lease_id"`
	BindGen string `json:"bind_gen"` // 同 air_import_gen
	Threshold    int `json:"threshold"`
	ActualQuorum int `json:"actual_quorum"`
	QuorumPassed bool `json:"quorum_passed"`
	BoundBy string    `json:"bound_by"`
	BoundAt time.Time `json:"bound_at"`
	CreatedAt time.Time `json:"created_at"`
}

   func (q QuorumBind) Copy() QuorumBind { return q }

// ─── 强制中止请求 ─────────────────────────────────────────────────────────
// ForceAbortReq 双人确认强制中止导入租约的请求。
type ForceAbortReq struct {
	LeaseID string `json:"lease_id"`
	ConfirmerOne string `json:"confirmer_one"`
	ConfirmerTwo string `json:"confirmer_two"`
	Reason string `json:"reason"`
}
