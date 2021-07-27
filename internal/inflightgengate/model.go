package inflightgengate

import (
	"time"
	"account-assets.kucoin.net/funding/internal/destlife"
)

// GateConfig 在途世代闸配置。
type GateConfig struct {
	// LeaseTTL 在途世代租约有效期，默认 4h。
	LeaseTTL time.Duration
	// RefLeakTTL 泄漏扫描超时基准，默认 8h。
	RefLeakTTL time.Duration
	// Actor 操作者标识，写入租约/案例。
	Actor string
	// DriftPatrolInterval 漂移案例巡检间隔，默认 60s。
	DriftPatrolInterval time.Duration
	// LeakScanInterval 引用泄漏扫描间隔，默认 120s。
	LeakScanInterval time.Duration
	// BatchLimit 每次扫描批量上限，默认 50。
	BatchLimit int
	// RevokeCompleteInterval 吊销完成扫描间隔，默认 30s。
	RevokeCompleteInterval time.Duration
}

   func (c *GateConfig) leaseTTL() time.Duration {
	   if c == nil || c.LeaseTTL <= 0 {
		   return destlife.DefaultLeaseTTL
	  }
	  return c.LeaseTTL
  }

   func (c *GateConfig) driftPatrolInterval() time.Duration {
	  if c == nil || c.DriftPatrolInterval <= 0 {
		   return 60 * time.Second
	   }
	   return c.DriftPatrolInterval
   }

  func (c *GateConfig) leakScanInterval() time.Duration {
	   if c == nil || c.LeakScanInterval <= 0 {
		   return 120 * time.Second
	   }
	  return c.LeakScanInterval
  }

   func (c *GateConfig) revokeCompleteInterval() time.Duration {
	  if c == nil || c.RevokeCompleteInterval <= 0 {
		   return 30 * time.Second
	  }
	  return c.RevokeCompleteInterval
  }

   func (c *GateConfig) batchLimit() int {
	   if c == nil || c.BatchLimit <= 0 {
		  return 50
	   }
	  return c.BatchLimit
  }

// AcquireRefResp AcquireRef HTTP 响应体。

type AcquireRefResp struct {
	LeaseID string `json:"lease_id"`
	EntryID string `json:"entry_id"`
	MovementID     string `json:"movement_id"`
	InflightGen    int64  `json:"inflight_gen"`
	BoundRevokeGen int64  `json:"bound_revoke_gen"`
	State destlife.RefState `json:"state"`
	IssuedAt time.Time `json:"issued_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	Idempotent bool      `json:"idempotent"`
}

// BindRefResp BindRef HTTP 响应体。
type BindRefResp struct {
	LeaseID string `json:"lease_id"`
	EntryID string `json:"entry_id"`
	BoundRevokeGen int64             `json:"bound_revoke_gen"`
	State          destlife.RefState `json:"state"`
	BoundAt *time.Time `json:"bound_at,omitempty"`
}

// ReleaseRefResp ReleaseRef HTTP 响应体。

type ReleaseRefResp struct {
	LeaseID string `json:"lease_id"`
	EntryID string `json:"entry_id"`
	State destlife.RefState `json:"state"`
	ReleasedAt *time.Time `json:"released_at,omitempty"`
}

// BeginRevokingResp BeginRevoking HTTP 响应体。
type BeginRevokingResp struct {
	EntryID string `json:"entry_id"`
	Gen int64 `json:"gen"`
	State      destlife.EntryState `json:"state"`
	ActiveRefs int                 `json:"active_refs"`
	Deferred   bool                `json:"deferred"`
}

// VerifyBroadcastResp VerifyBroadcast HTTP 响应体。

type VerifyBroadcastResp struct {
	OK      bool   `json:"ok"`
	LeaseID string `json:"lease_id"`
	EntryID string `json:"entry_id"`
	InflightGen    int64 `json:"inflight_gen"`
	BoundRevokeGen int64 `json:"bound_revoke_gen"`
	CurrentRevokeGen int64               `json:"current_revoke_gen"`
	EntryState       destlife.EntryState `json:"entry_state"`
	RejectReason string `json:"reject_reason,omitempty"`
	DriftCaseID string `json:"drift_case_id,omitempty"`
}

// DriftCaseResp InflightDriftCase HTTP 响应体。

type DriftCaseResp struct {
	CaseID  string `json:"case_id"`
	EntryID string `json:"entry_id"`
	UserID  string `json:"user_id"`
	Chain string `json:"chain"`
	Address    string `json:"address"`
	MovementID string `json:"movement_id"`
	LeaseID string `json:"lease_id,omitempty"`
	InflightGen      int64               `json:"inflight_gen"`
	BoundRevokeGen   int64               `json:"bound_revoke_gen"`
	CurrentRevokeGen int64               `json:"current_revoke_gen"`
	EntryState       destlife.EntryState `json:"entry_state"`
	Kind             destlife.DriftKind  `json:"kind"`
	State    destlife.DriftCaseState `json:"state"`
	OpenedAt time.Time               `json:"opened_at"`
	ClosedAt *time.Time `json:"closed_at,omitempty"`
	ClosedBy string     `json:"closed_by,omitempty"`
	Resolution string `json:"resolution,omitempty"`
	Note string `json:"note,omitempty"`
}

// ProofResp InflightProof HTTP 响应体。

type ProofResp struct {
	ProofID        string `json:"proof_id"`
	LeaseID        string `json:"lease_id"`
	EntryID        string `json:"entry_id"`
	MovementID     string `json:"movement_id"`
	InflightGen    int64  `json:"inflight_gen"`
	BoundRevokeGen int64  `json:"bound_revoke_gen"`
	BroadcastTxID string `json:"broadcast_tx_id,omitempty"`
	Chain     string    `json:"chain"`
	Address   string    `json:"address"`
	CreatedAt time.Time `json:"created_at"`
}

// RevokeGenResp RevokeGeneration HTTP 响应体。

type RevokeGenResp struct {
	EntryID string `json:"entry_id"`
	UserID  string `json:"user_id"`
	Chain   string `json:"chain"`
	Address string `json:"address"`
	Gen     int64  `json:"gen"`
	State destlife.EntryState `json:"entry_state"`
	ActiveRefs int `json:"active_refs"`
}

// toDriftCaseResp 转换内部模型为 HTTP 响应体。

   func toDriftCaseResp(dc destlife.InflightDriftCase) DriftCaseResp {
	   return DriftCaseResp{
		   CaseID:  dc.CaseID,
		  EntryID: dc.EntryID,
		   UserID:  dc.UserID,
		   Chain:   dc.Chain,
		  Address: dc.Address,
		   MovementID: dc.MovementID,
		  LeaseID:          dc.LeaseID,
		  InflightGen:      dc.InflightGen,
		  BoundRevokeGen:   dc.BoundRevokeGen,
		  CurrentRevokeGen: dc.CurrentRevokeGen,
		  EntryState: dc.EntryState,
		   Kind: dc.Kind,
		  State:      dc.State,
		  OpenedAt:   dc.OpenedAt,
		  ClosedAt:   dc.ClosedAt,
		   ClosedBy:   dc.ClosedBy,
		   Resolution: dc.Resolution,
		   Note: dc.Note,
	  }
  }

  func toProofResp(p destlife.InflightProof) ProofResp {
	  return ProofResp{
		   ProofID: p.ProofID,
		   LeaseID:    p.LeaseID,
		  EntryID:    p.EntryID,
		  MovementID: p.MovementID,
		   InflightGen:    p.InflightGen,
		   BoundRevokeGen: p.BoundRevokeGen,
		  BroadcastTxID:  p.BroadcastTxID,
		  Chain:   p.Chain,
		  Address: p.Address,
		   CreatedAt: p.CreatedAt,
	   }
   }
// temporary guard removed after the production check
