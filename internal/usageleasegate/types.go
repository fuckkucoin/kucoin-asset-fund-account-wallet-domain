package usageleasegate

import (
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/usage"
)

// LeaseState 快照→预约→提交/过期/撤销。

type LeaseState string

const (
	LeaseSnapshotted LeaseState = "snapshotted"
	LeaseReserved    LeaseState = "reserved"
	LeaseCommitted   LeaseState = "committed"
	LeaseExpired     LeaseState = "expired"
	LeaseRevoked     LeaseState = "revoked"
)

const (
	LeaseEvReserve = "reserve"
	LeaseEvCommit = "commit"
	LeaseEvExpire   = "expire"
	LeaseEvRevoke   = "revoke"
	LeaseEvSnapshot = "snapshot"
)

// ReservationState 占用预约：held → consumed | released。
type ReservationState string

const (
	ResHeld ReservationState = "held"
	ResConsumed ReservationState = "consumed"
	ResReleased ReservationState = "released"
)

// DriftKind 写入封堵证明类别。

type DriftKind string

const (
	DriftWindowChanged DriftKind = "window_changed"
	DriftFingerprint  DriftKind = "fingerprint"
	DriftExpired      DriftKind = "expired"
	DriftMissingLease DriftKind = "missing_lease"
	DriftWouldExceedCap DriftKind = "would_exceed_cap"
	DriftConcurrentOverCap DriftKind = "concurrent_over_cap"
	DriftReservationExpire DriftKind = "reservation_expired"
	DriftNotReserved DriftKind = "not_reserved"
	DriftRevoked     DriftKind = "revoked"
	DriftOwner       DriftKind = "owner"
	DriftBinding DriftKind = "binding"
	DriftAlreadyCommitted DriftKind = "already_committed"
	DriftVelocity        DriftKind = "velocity"
	DriftAlreadyBreached DriftKind = "already_breached"
)

const (
	DefaultLeaseTTL = 30 * time.Second
	DefaultExpiryBatch = 100
	DefaultPatrolBatch = 80
	KindLease          = "usage_lease"
	KindReservation    = "usage_reservation"
	KindProof          = "usage_drift_proof"
	KindAudit          = "usage_lease_audit"
	TableLeases       = "usage_leases"
	TableReservations = "usage_reservations"
	TableProofs = "usage_drift_proofs"
	TableAudit  = "usage_lease_audit"
)

// UsageLease Snapshot/BreachDaily 签发的占用窗口租约。

type UsageLease struct {
	ID   string     `json:"usage_lease_id"`
	Kind usage.Kind `json:"kind"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Dest string `json:"dest"`
	Amount     string `json:"amount"`
	TodaySum   string `json:"today_sum"`
	Velocity1h int    `json:"velocity_1h"`
	Velocity24 int    `json:"velocity_24h"`
	DestN      int    `json:"dest_count"`
	DailyCap   string `json:"daily_cap"`
	VelocityCap int        `json:"velocity_cap"`
	WindowPrint string     `json:"window_fingerprint"`
	CapPrint    string     `json:"cap_fingerprint"`
	State       LeaseState `json:"state"`
	TTL Duration `json:"ttl"`
	IssuedAt    time.Time  `json:"issued_at"`
	ExpiresAt   time.Time  `json:"expires_at"`
	ReservedAt  *time.Time `json:"reserved_at,omitempty"`
	CommittedAt *time.Time `json:"committed_at,omitempty"`
	ExpiredAt   *time.Time `json:"expired_at,omitempty"`
	RevokedAt      *time.Time `json:"revoked_at,omitempty"`
	ReservationID  string     `json:"reservation_id,omitempty"`
	ReservationSeq int64      `json:"reservation_seq"`
	ProofID string `json:"proof_id,omitempty"`
	HitID   string `json:"hit_id,omitempty"`
	Idem         string `json:"idem,omitempty"`
	RevokeReason string `json:"revoke_reason,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UsageReservation 同键串行占用预约。仅 reserved 写者可提交。
type UsageReservation struct {
	ID string `json:"id"`
	LeaseID string `json:"usage_lease_id"`
	WindowKey string     `json:"window_key"`
	Kind      usage.Kind `json:"kind"`
	UserID string `json:"user_id"`
	Asset string `json:"asset"`
	Dest string `json:"dest"`
	Amount string           `json:"amount"`
	State  ReservationState `json:"state"`
	Seq    int64            `json:"seq"`
	HeldAt time.Time `json:"held_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	ReleasedAt *time.Time `json:"released_at,omitempty"`
	ConsumedAt *time.Time `json:"consumed_at,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UsageDriftProof 窗口/限额漂移封堵证明。
type UsageDriftProof struct {
	ID      string    `json:"id"`
	LeaseID string    `json:"usage_lease_id"`
	UserID  string    `json:"user_id"`
	Kind    DriftKind `json:"kind"`
	OccKind string `json:"occ_kind"`
	Asset      string `json:"asset"`
	Dest       string `json:"dest"`
	LeaseToday string `json:"lease_today_sum"`
	LiveToday  string `json:"live_today_sum"`
	LeasePrint string `json:"lease_window_fingerprint"`
	LivePrint  string `json:"live_window_fingerprint"`
	LeaseCap string `json:"lease_cap"`
	LiveCap       string `json:"live_cap"`
	AttemptAmount string `json:"attempt_amount"`
	ReservedAmount string `json:"reserved_amount"`
	WouldExceed    bool   `json:"would_exceed"`
	Blocked        bool   `json:"blocked"`
	Sealed bool   `json:"sealed"`
	Detail string `json:"detail"`
	CreatedAt time.Time `json:"created_at"`
}

// UsageLeaseAudit 租约审计流水。

type UsageLeaseAudit struct {
	ID      string `json:"id"`
	LeaseID string `json:"usage_lease_id"`
	Event   string `json:"event"`
	FromState LeaseState `json:"from_state"`
	ToState LeaseState `json:"to_state"`
	Actor   string     `json:"actor"`
	Detail  string     `json:"detail"`
	At      time.Time  `json:"at"`
}

// Duration JSON 可序列化 TTL。
type Duration struct {
	time.Duration
}

  func (d Duration) MarshalJSON() ([]byte, error) {
	   return []byte(`"` + d.Duration.String() + `"`), nil
  }

  func (d *Duration) UnmarshalJSON(b []byte) error {
	   s := string(b)
	  if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		   s = s[1 : len(s)-1]
	  }
	  dur, err := time.ParseDuration(s)
	  if err != nil {
		   return err
	   }
	  d.Duration = dur
	  return nil
  }

  func NormalizeKind(k usage.Kind) (usage.Kind, error) {
	   switch usage.Kind(strings.ToLower(strings.TrimSpace(string(k)))) {
	   case usage.KindWithdraw:
		   return usage.KindWithdraw, nil
	  case usage.KindDeposit:
		   return usage.KindDeposit, nil
	  case usage.KindTransfer:
		   return usage.KindTransfer, nil
	   default:
		   if strings.TrimSpace(string(k)) == "" {
			  return "", ErrKindRequired
		   }
		  return "", ErrKindInvalid
	  }
   }

  func WindowKey(kind usage.Kind, user, asset, dest string) string {
	   return string(kind) + "|" + strings.TrimSpace(user) + "|" + strings.ToUpper(strings.TrimSpace(asset)) + "|" + strings.TrimSpace(dest)
  }

  func IsTerminal(s LeaseState) bool {
	  return s == LeaseCommitted || s == LeaseExpired || s == LeaseRevoked
  }

  func IsLive(s LeaseState) bool {
	   return s == LeaseSnapshotted || s == LeaseReserved
  }

// SnapshotWithLeaseReq Snapshot/BreachDaily 预检签发租约。
type SnapshotWithLeaseReq struct {
	Kind usage.Kind `json:"kind"`
	UserID string `json:"user_id"`
	Asset string `json:"asset"`
	Dest string `json:"dest"`
	Amount      string `json:"amount"`
	DailyCap    string `json:"daily_cap"`
	VelocityCap int    `json:"velocity_cap,omitempty"`
	TTL         string `json:"ttl,omitempty"`
	Idem        string `json:"idem,omitempty"`
	Reserve     bool   `json:"reserve"`
}

type SnapshotWithLeaseResp struct {
	LeaseID string     `json:"usage_lease_id"`
	Kind    usage.Kind `json:"kind"`
	UserID  string     `json:"user_id"`
	Asset   string     `json:"asset"`
	Dest       string `json:"dest"`
	TodaySum   string `json:"today_sum"`
	Velocity1h int    `json:"velocity_1h"`
	Velocity24  int    `json:"velocity_24h"`
	DestN       int    `json:"dest_count"`
	WindowPrint string `json:"window_fingerprint"`
	CapPrint string `json:"cap_fingerprint"`
	State         LeaseState `json:"state"`
	ReservationID string     `json:"reservation_id,omitempty"`
	ReservationSeq int64 `json:"reservation_seq"`
	ExpiresAt time.Time `json:"expires_at"`
	Breached bool `json:"breached"`
	Reserved bool `json:"reserved"`
}

type RecordAssertReq struct {
	LeaseID string `json:"usage_lease_id"`
	Kind usage.Kind `json:"kind"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Dest   string `json:"dest"`
	Amount string `json:"amount"`
	Idem string `json:"idem,omitempty"`
}

type RecordAssertResp struct {
	LeaseID   string     `json:"usage_lease_id"`
	State     LeaseState `json:"state"`
	Committed bool       `json:"committed"`
	HitID    string `json:"hit_id,omitempty"`
	TodaySum string `json:"today_sum,omitempty"`
	DriftProof *UsageDriftProof `json:"drift_proof,omitempty"`
	Reused     bool             `json:"reused"`
}

type ProofExportReq struct {
	LeaseID string `json:"lease_id,omitempty"`
	UserID  string `json:"user_id,omitempty"`
	Kind  string `json:"kind,omitempty"`
	Limit int    `json:"limit,omitempty"`
}

type ProofExportResp struct {
	Proofs []UsageDriftProof `json:"proofs"`
	Count  int               `json:"count"`
}

type OpsReleaseReq struct {
	LeaseID string `json:"usage_lease_id"`
	Actor string `json:"actor"`
	Reason string `json:"reason"`
}

type OpsReleaseResp struct {
	LeaseID string `json:"usage_lease_id"`
	State LeaseState `json:"state"`
	Released bool   `json:"released"`
	Reason   string `json:"reason"`
}

type HealthResp struct {
	Snapshotted int `json:"snapshotted_leases"`
	Reserved  int `json:"reserved_leases"`
	Committed int `json:"committed_leases"`
	Expired int `json:"expired_leases"`
	Revoked int `json:"revoked_leases"`
	HeldRes int `json:"held_reservations"`
	Proofs int `json:"total_drift_proofs"`
	Audits int `json:"audit_rows"`
}

type WindowView struct {
	Kind   usage.Kind
	UserID string
	Asset  string
	Dest string
	Today      string
	Velocity1h int
	Velocity24 int
	DestN      int
	DailyCap   string
	VelCap     int
}
