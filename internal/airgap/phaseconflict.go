package airgap

import "time"

// ConflictKind 冲突类型枚举。
type ConflictKind string

const (
	ConflictImportVsAbort ConflictKind = "import_vs_abort"
	ConflictImportVsClose ConflictKind = "import_vs_close"
	ConflictImportVsExport ConflictKind = "import_vs_export"
)

// ConflictState 冲突证明状态。
type ConflictState string

const (
	ConflictOpen   ConflictState = "open"
	ConflictProven ConflictState = "proven"
	ConflictClosed ConflictState = "closed"
)

// PhaseConflictProof 相位冲突证明。
type PhaseConflictProof struct {
	ConflictID string `json:"conflict_id"` // UUID
	SessionID    string       `json:"session_id"`
	ConflictKind ConflictKind `json:"conflict_kind"`
	State ConflictState `json:"state"`
	// 冲突双方信息
	WinnerOp string `json:"winner_op"` // 胜者操作 (import/abort/close/export)
	WinnerActor string `json:"winner_actor"` // 胜者操作人
	LoserOp     string `json:"loser_op"`     // 败者操作
	LoserActor string `json:"loser_actor"` // 败者操作人
	// 快照
	PhaseAtConflict string `json:"phase_at_conflict"` // 冲突发生时的 Phase
	Note string `json:"note,omitempty"`
	ProvenAt *time.Time `json:"proven_at,omitempty"`
	ClosedAt *time.Time `json:"closed_at,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Copy 深拷贝 PhaseConflictProof。
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
