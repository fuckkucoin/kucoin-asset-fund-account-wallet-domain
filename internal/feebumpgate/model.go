package feebumpgate

import "time"

// GenState 抬费世代状态。

type GenState string

const (
	GenSigned    GenState = "signed"
	GenBroadcast GenState = "broadcast"
	GenSuperseded GenState = "superseded"
	GenFinal GenState = "final"
	GenBlocked GenState = "blocked"
)

// Terminal 终态不可逆。
  func (s GenState) Terminal() bool {
	  return s == GenSuperseded || s == GenFinal || s == GenBlocked
   }

// CanBroadcast signed 才可进入替换广播。
  func (s GenState) CanBroadcast() bool { return s == GenSigned }

// CanSupersede signed 或 broadcast 可被新世代退役。
  func (s GenState) CanSupersede() bool {
	  return s == GenSigned || s == GenBroadcast
  }

// Active 仍可能占用热库。
   func (s GenState) Active() bool {
	   return s == GenSigned || s == GenBroadcast
   }

// OccState 广播占用状态。
type OccState string

const (
	OccHeld    OccState = "held"
	OccRetired OccState = "retired"
	OccOrphaned OccState = "orphaned"
)

// Terminal 占用终态。
  func (s OccState) Terminal() bool {
	   return s == OccRetired || s == OccOrphaned
   }

// Held 仍计热占。
   func (s OccState) Held() bool { return s == OccHeld }

// ProofKind 孤儿/拒广播证明类型。
type ProofKind string

const (
	ProofStaleGen ProofKind = "stale_gen"
	ProofNoGen            ProofKind = "no_gen"
	ProofPrevHashMismatch ProofKind = "prev_hash_mismatch"
	ProofDualOccupancy    ProofKind = "dual_occupancy"
	ProofRetireFail ProofKind = "retire_fail"
	ProofOldBroadcast ProofKind = "old_gen_broadcast"
	ProofOrphanOpen ProofKind = "orphan_open"
	ProofOrphanClosed ProofKind = "orphan_closed"
	ProofForceRetire ProofKind = "force_retire"
	ProofOccTimeout  ProofKind = "occ_timeout"
	ProofSignBlocked ProofKind = "sign_blocked_orphan"
	ProofBroadcastOK ProofKind = "broadcast_ok"
	ProofRetireOK     ProofKind = "retire_ok"
	ProofFeeNotRaised ProofKind = "fee_not_raised"
	ProofIdemConflict ProofKind = "idem_conflict"
)

// OrphanState 孤儿证明生命周期。

type OrphanState string

const (
	OrphanOpen   OrphanState = "open"
	OrphanClosed OrphanState = "closed"
)

// Open 挡后续抬费/广播。

  func (s OrphanState) Open() bool { return s == OrphanOpen }

// FeeBumpGen 一次抬费签章世代。
type FeeBumpGen struct {
	GenID      string `json:"gen_id"`
	TransferID string `json:"transfer_id"`
	Generation int64  `json:"generation"`
	PrevBroadcastHash string `json:"prev_broadcast_hash"`
	NewBroadcastHash  string `json:"new_broadcast_hash"`
	NewFee            string `json:"new_fee"`
	PrevFee           string `json:"prev_fee"`
	DestinationTag    string `json:"destination_tag"`
	OccupancyFP string `json:"occupancy_fp"`
	Chain       string `json:"chain"`
	To          string `json:"to"`
	Amount      string `json:"amount"`
	Asset       string `json:"asset"`
	CanonDigest string `json:"canon_digest"`
	SigDigest string `json:"sig_digest"`
	State          GenState `json:"state"`
	OccID          string   `json:"occ_id"`
	IdempotencyKey string   `json:"idempotency_key"`
	Actor string `json:"actor"`
	SignedAt time.Time `json:"signed_at"`
	BroadcastAt *time.Time `json:"broadcast_at,omitempty"`
	SupersededAt *time.Time `json:"superseded_at,omitempty"`
	FinalAt      *time.Time `json:"final_at,omitempty"`
	BlockedAt    *time.Time `json:"blocked_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// Copy 值拷贝。
  func (g FeeBumpGen) Copy() FeeBumpGen {
	   out := g
	   if g.BroadcastAt != nil {
		   t := *g.BroadcastAt
		   out.BroadcastAt = &t
	  }
	   if g.SupersededAt != nil {
		  t := *g.SupersededAt
		  out.SupersededAt = &t
	   }
	  if g.FinalAt != nil {
		   t := *g.FinalAt
		  out.FinalAt = &t
	   }
	  if g.BlockedAt != nil {
		  t := *g.BlockedAt
		  out.BlockedAt = &t
	  }
	   return out
   }

// BroadcastOccupancy 热库/广播占用行。
type BroadcastOccupancy struct {
	OccID string `json:"occ_id"`
	GenID string `json:"gen_id"`
	TransferID    string `json:"transfer_id"`
	Generation    int64  `json:"generation"`
	BroadcastHash string `json:"broadcast_hash"`
	OccupancyFP   string `json:"occupancy_fp"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Fee    string `json:"fee"`
	State  OccState  `json:"state"`
	HeldAt time.Time `json:"held_at"`
	RetiredAt  *time.Time `json:"retired_at,omitempty"`
	OrphanedAt *time.Time `json:"orphaned_at,omitempty"`
	ExpireAt   time.Time  `json:"expire_at"`
	Actor     string    `json:"actor"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Copy 值拷贝。
  func (o BroadcastOccupancy) Copy() BroadcastOccupancy {
	  out := o
	  if o.RetiredAt != nil {
		   t := *o.RetiredAt
		  out.RetiredAt = &t
	  }
	  if o.OrphanedAt != nil {
		  t := *o.OrphanedAt
		   out.OrphanedAt = &t
	  }
	   return out
  }

// RbfOrphanProof 孤儿/拒路径不可变证明。

type RbfOrphanProof struct {
	ProofID    string `json:"proof_id"`
	TransferID string `json:"transfer_id"`
	GenID      string `json:"gen_id"`
	PrevGenID  string `json:"prev_gen_id"`
	OccID string `json:"occ_id"`
	Kind       ProofKind   `json:"kind"`
	State      OrphanState `json:"state"`
	QuotedHash string      `json:"quoted_hash"`
	LiveHash  string `json:"live_hash"`
	QuotedGen int64  `json:"quoted_gen"`
	LiveGen int64 `json:"live_gen"`
	HeldOccCount int `json:"held_occ_count"`
	Detail string `json:"detail"`
	Actor    string     `json:"actor"`
	At       time.Time  `json:"at"`
	ClosedAt *time.Time `json:"closed_at,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Copy 值拷贝。
  func (p RbfOrphanProof) Copy() RbfOrphanProof {
	  out := p
	  if p.ClosedAt != nil {
		   t := *p.ClosedAt
		  out.ClosedAt = &t
	  }
	   return out
  }

// BlocksNewSign open 孤儿挡新抬费。

   func (p RbfOrphanProof) BlocksNewSign() bool {
	   return p.State.Open() && (p.Kind == ProofDualOccupancy ||
		   p.Kind == ProofRetireFail ||
		   p.Kind == ProofOrphanOpen ||
		   p.Kind == ProofSignBlocked)
  }

// BlocksBroadcast open 孤儿挡广播。
  func (p RbfOrphanProof) BlocksBroadcast() bool {
	  return p.State.Open()
  }

// GenAudit 世代审计行。
type GenAudit struct {
	AuditID    string `json:"audit_id"`
	GenID      string `json:"gen_id"`
	TransferID string `json:"transfer_id"`
	Action     string `json:"action"`
	FromState  string `json:"from_state"`
	ToState string `json:"to_state"`
	Actor string `json:"actor"`
	Detail string    `json:"detail"`
	At     time.Time `json:"at"`
}

// Copy 值拷贝。
   func (a GenAudit) Copy() GenAudit { return a }

// SignWithGenReq 带世代抬费签章请求。
type SignWithGenReq struct {
	TransferID string `json:"transfer_id"`
	Chain      string `json:"chain"`
	To         string `json:"to"`
	Amount     string `json:"amount"`
	Asset  string `json:"asset"`
	NewFee string `json:"new_fee"`
	DestinationTag string `json:"destination_tag"`
	PrevBroadcastHash string `json:"prev_broadcast_hash"`
	IdempotencyKey    string `json:"idempotency_key"`
	Actor string `json:"actor"`
	Canon string `json:"canon"`
	SigHex string `json:"sig_hex"`
}

// BroadcastReq 替换广播断言。
type BroadcastReq struct {
	GenID         string `json:"gen_id"`
	TransferID    string `json:"transfer_id"`
	BroadcastHash string `json:"broadcast_hash"`
	Actor         string `json:"actor"`
}

// AssertOccReq 占用复验。
type AssertOccReq struct {
	GenID      string `json:"gen_id"`
	TransferID string `json:"transfer_id"`
	BroadcastHash string `json:"broadcast_hash"`
	Actor string `json:"actor"`
}

// RetireReq 退役占用。

type RetireReq struct {
	GenID string `json:"gen_id"`
	TransferID string `json:"transfer_id"`
	Actor string `json:"actor"`
	Force bool   `json:"force"`
	Why string `json:"why"`
}

// SignWithGenResp 签章+世代回包。
type SignWithGenResp struct {
	Gen FeeBumpGen         `json:"gen"`
	Occ BroadcastOccupancy `json:"occupancy"`
	Proof *RbfOrphanProof `json:"proof,omitempty"`
}

// BroadcastResp 广播回包。

type BroadcastResp struct {
	Gen   FeeBumpGen         `json:"gen"`
	Occ   BroadcastOccupancy `json:"occupancy"`
	Proof *RbfOrphanProof    `json:"proof,omitempty"`
}

// GateConfig 闸配置。

type GateConfig struct {
	OccTTL time.Duration
	MaxOpenOrphans  int
	RequirePrevHash bool
	MinFeeBumpUnits int64
}

// Fill 默认值。

   func (c *GateConfig) Fill() {
	   if c.OccTTL <= 0 {
		  c.OccTTL = 15 * time.Minute
	  }
	  if c.MaxOpenOrphans <= 0 {
		   c.MaxOpenOrphans = 1
	   }
  }

// WorkerConfig 巡检间隔。
type WorkerConfig struct {
	WorkerID string
	OrphanScanInterval  time.Duration
	RetireScanInterval  time.Duration
	ProofExportInterval time.Duration
	BatchSize int
}

// Fill 默认值。
  func (c *WorkerConfig) Fill() {
	   if c.WorkerID == "" {
		   c.WorkerID = "feebump-gate-worker"
	  }
	  if c.OrphanScanInterval <= 0 {
		  c.OrphanScanInterval = 20 * time.Second
	  }
	  if c.RetireScanInterval <= 0 {
		  c.RetireScanInterval = 30 * time.Second
	  }
	   if c.ProofExportInterval <= 0 {
		  c.ProofExportInterval = time.Minute
	  }
	   if c.BatchSize <= 0 {
		  c.BatchSize = 64
	   }
   }
