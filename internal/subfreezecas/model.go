package subfreezecas

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"
)

// CASState 冻结世代状态机。
type CASState string

const (
	CASOpen   CASState = "open"
	CASFrozen CASState = "frozen"
	CASThawed CASState = "thawed"
)

  func ParseCASState(s string) (CASState, error) {
	   switch CASState(strings.ToLower(strings.TrimSpace(s))) {
	  case CASOpen, CASFrozen, CASThawed:
		  return CASState(strings.ToLower(strings.TrimSpace(s))), nil
	  default:
		   return "", ErrInvalidInput
	   }
   }

   func (s CASState) LiveDebit() bool { return s == CASOpen }

  func (s CASState) Frozen() bool { return s == CASFrozen }

// ProofState 冻后借记证明状态机。
type ProofState string

const (
	ProofOpen ProofState = "open"
	ProofAcked ProofState = "acked"
)

  func ParseProofState(s string) (ProofState, error) {
	   switch ProofState(strings.ToLower(strings.TrimSpace(s))) {
	  case ProofOpen, ProofAcked:
		  return ProofState(strings.ToLower(strings.TrimSpace(s))), nil
	   default:
		   return "", ErrInvalidInput
	   }
  }

   func (s ProofState) Open() bool { return s == ProofOpen || s == "" }

// FreezeCAS 母户冻结世代单据。
type FreezeCAS struct {
	FreezeCASID string `json:"freeze_cas_id"`
	ParentID            string `json:"parent_id"`
	ChildSetFingerprint string `json:"child_set_fingerprint"`
	ChildIDs   []string `json:"child_ids"`
	Reason     string   `json:"reason"`
	Generation int64    `json:"generation"`
	State CASState `json:"state"`
	Actor    string    `json:"actor"`
	OpenedAt time.Time `json:"opened_at"`
	FrozenAt time.Time `json:"frozen_at,omitempty"`
	ThawedAt time.Time `json:"thawed_at,omitempty"`
	SupersededBy   string `json:"superseded_by,omitempty"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
	Note string `json:"note,omitempty"`
}

  func (c FreezeCAS) Copy() FreezeCAS {
	  cp := c
	  if c.ChildIDs != nil {
		   cp.ChildIDs = append([]string(nil), c.ChildIDs...)
	  }
	   return cp
   }

   func (c FreezeCAS) DebitAllowed(now time.Time) error {
	   if c.FreezeCASID == "" {
		  return ErrCASMissing
	  }
	  if c.State == CASFrozen {
		   return ErrCASFrozen
	   }
	   if c.State == CASThawed {
		  return ErrCASThawed
	  }
	   if c.State != CASOpen {
		   return ErrCASFrozen
	   }
	  if c.Generation < 0 {
		   return ErrCASStale
	  }
	  _ = now
	  return nil
   }

// FrozenSpendProof 冻后仍成功借记的击穿证明。
type FrozenSpendProof struct {
	ProofID string `json:"proof_id"`
	FreezeCASID string `json:"freeze_cas_id"`
	ParentID string `json:"parent_id"`
	ChildID  string `json:"child_id"`
	OwnerID  string `json:"owner_id"`
	Asset    string `json:"asset"`
	Amount   string `json:"amount"`
	DebitIdem string `json:"debit_idempotency_key"`
	DetectedAt time.Time `json:"detected_at"`
	Compensated         bool       `json:"compensated"`
	CompensateCreditKey string     `json:"compensate_credit_key,omitempty"`
	State               ProofState `json:"state"`
	AckedBy             string     `json:"acked_by,omitempty"`
	AckedAt             time.Time  `json:"acked_at,omitempty"`
	Note            string `json:"note,omitempty"`
	RolledBackAvail string `json:"rolled_back_available,omitempty"`
}

   func (p FrozenSpendProof) Copy() FrozenSpendProof { return p }

// VelocityRevocation 速度窗吊销记录。
type VelocityRevocation struct {
	RevokeID    string `json:"revoke_id"`
	FreezeCASID string `json:"freeze_cas_id"`
	ParentID    string `json:"parent_id"`
	OwnerID string `json:"owner_id"`
	Generation int64     `json:"generation"`
	FrozenAt   time.Time `json:"frozen_at"`
	WindowFrom time.Time `json:"window_from"`
	WindowTo   time.Time `json:"window_to"`
	HitCount   int       `json:"hit_count"`
	Active bool `json:"active"`
	Note string `json:"note,omitempty"`
}

   func (v VelocityRevocation) Copy() VelocityRevocation { return v }

  func (v VelocityRevocation) Covers(owner string, at time.Time) bool {
	  if !v.Active {
		   return false
	   }
	  if strings.TrimSpace(owner) != v.OwnerID {
		  return false
	   }
	  if at.Before(v.WindowFrom) || at.After(v.WindowTo) {
		  return false
	   }
	   return true
  }

// FreezeAudit 冻结/解冻/借记闸审计。
type FreezeAudit struct {
	AuditID string `json:"audit_id"`
	FreezeCASID string `json:"freeze_cas_id"`
	ParentID    string `json:"parent_id"`
	ChildID     string `json:"child_id,omitempty"`
	Actor  string `json:"actor"`
	Action string `json:"action"`
	Outcome    string    `json:"outcome"`
	Detail     string    `json:"detail"`
	Generation int64     `json:"generation"`
	At         time.Time `json:"at"`
}

  func (a FreezeAudit) Copy() FreezeAudit { return a }

// DebitJournal 引擎侧借记轨迹，供击穿巡检冻点。
type DebitJournal struct {
	JournalID   string `json:"journal_id"`
	FreezeCASID string `json:"freeze_cas_id"`
	OwnerID  string `json:"owner_id"`
	ParentID string `json:"parent_id"`
	ChildID string `json:"child_id,omitempty"`
	Asset   string `json:"asset"`
	Amount  string `json:"amount"`
	Idem string `json:"idempotency_key"`
	OK      bool      `json:"ok"`
	Refused string    `json:"refused,omitempty"`
	At      time.Time `json:"at"`
	AfterFrozen bool `json:"after_frozen"`
}

   func (j DebitJournal) Copy() DebitJournal { return j }

// ChildFingerprint 子户集核验范化指纹。

  func ChildFingerprint(childIDs []string) string {
	   norm := normalizeChildIDs(childIDs)
	   h := sha256.New()
	   _, _ = h.Write([]byte("subacct.childset.v1\n"))
	  for _, id := range norm {
		   _, _ = h.Write([]byte(id))
		  _, _ = h.Write([]byte{'\n'})
	   }
	   return hex.EncodeToString(h.Sum(nil))
  }

   func normalizeChildIDs(in []string) []string {
	  seen := map[string]struct{}{}
	   out := make([]string, 0, len(in))
	   for _, id := range in {
		  id = strings.TrimSpace(id)
		   if id == "" {
			   continue
		   }
		  if _, ok := seen[id]; ok {
			  continue
		  }
		  seen[id] = struct{}{}
		  out = append(out, id)
	  }
	   sort.Strings(out)
	   return out
  }

// AlignChildren 母冻时子闸是否与指纹集合一致。
  func AlignChildren(cas FreezeCAS, frozenChildIDs []string) error {
	  if cas.State != CASFrozen {
		   return nil
	   }
	  want := normalizeChildIDs(cas.ChildIDs)
	  got := normalizeChildIDs(frozenChildIDs)
	  if ChildFingerprint(want) != ChildFingerprint(got) {
		  return ErrGateMisaligned
	  }
	   if cas.ChildSetFingerprint != "" && cas.ChildSetFingerprint != ChildFingerprint(want) {
		   return ErrFingerprint
	   }
	   return nil
  }

// CurrentLineage 同一母户当前有效 CAS：先 frozen，其次最新 open，忽略已被替代的 thawed。

  func CurrentLineage(rows []FreezeCAS) *FreezeCAS {
	  var frozen *FreezeCAS
	  var open *FreezeCAS
	  var latest *FreezeCAS
	   for i := range rows {
		   r := rows[i]
		   if latest == nil || r.Generation > latest.Generation || (r.Generation == latest.Generation && r.OpenedAt.After(latest.OpenedAt)) {
			   cp := r.Copy()
			  latest = &cp
		   }
		  if r.State == CASFrozen {
			   cp := r.Copy()
			  frozen = &cp
		   }
		  if r.State == CASOpen {
			  if open == nil || r.Generation >= open.Generation {
				  cp := r.Copy()
				  open = &cp
			   }
		  }
	   }
	   if frozen != nil {
		   return frozen
	  }
	   if open != nil {
		  return open
	  }
	   return latest
  }

// ScopeKey 母户 CAS 作用域。
   func ScopeKey(parentID string) string {
	   return strings.TrimSpace(parentID)
   }

const (
	ActionFreezeParent = "freeze_parent"
	ActionFreezeChild  = "freeze_child"
	ActionUnfreezeParent = "unfreeze_parent"
	ActionUnfreezeChild = "unfreeze_child"
	ActionDebitAssert = "debit_assert"
	ActionPunchProof  = "punch_proof"
	ActionProofAck = "proof_ack"
	ActionVelRevoke = "velocity_revoke"
	ActionCompensate = "compensate_credit"
	OutcomeOK     = "ok"
	OutcomeDenied = "denied"
	OutcomePunch  = "punch"
	OutcomeMisaligned  = "misaligned"
	OutcomeStale       = "stale"
	OutcomeRevoked     = "revoked"
	OutcomeIdem        = "idempotent"
	OutcomeCompensated = "compensated"
)
