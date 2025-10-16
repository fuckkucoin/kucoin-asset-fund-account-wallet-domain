package stmtexpiry

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

// ──────────────────────────── 导出租约状态 ────────────────────────────

type LeaseState string

const (
	LeaseBuilt   LeaseState = "built"
	LeaseIssued  LeaseState = "issued"
	LeaseExpired LeaseState = "expired"
	LeaseRevoked LeaseState = "revoked"
)

  func (s LeaseState) Terminal() bool { return s == LeaseExpired || s == LeaseRevoked }

  func (s LeaseState) Valid() bool {
	  switch s {
	  case LeaseBuilt, LeaseIssued, LeaseExpired, LeaseRevoked:
		  return true
	  }
	  return false
   }

// ──────────────────────────── 漂移案例状态 ────────────────────────────

type DriftState string

const (
	DriftOpen DriftState = "open"
	DriftProven DriftState = "proven"
	DriftClosed DriftState = "closed"
)

  func (s DriftState) Terminal() bool { return s == DriftClosed }

  func (s DriftState) Valid() bool {
	  switch s {
	  case DriftOpen, DriftProven, DriftClosed:
		  return true
	  }
	   return false
   }

// ──────────────────────────── ExportLease ────────────────────────────
// ExportLease 导出租约：绑定用户、窗口、过期时间和账单高水位。
// TTL 到期自动撤销；高水位之后的新账单会触发漂移检测。
type ExportLease struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
	Format string `json:"format"`
	Root      string     `json:"root"`
	HighWater int        `json:"high_water"`
	State     LeaseState `json:"state"`
	TTLSec   int       `json:"ttl_sec"`
	IssuedAt time.Time `json:"issued_at,omitempty"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	WindowStart time.Time `json:"window_start,omitempty"`
	WindowEnd time.Time `json:"window_end,omitempty"`
	RevokedBy string `json:"revoked_by,omitempty"`
	ProofID   string `json:"proof_id,omitempty"`
}

  func NewExportLease(userID, format, root string, highWater, ttlSec int, wStart, wEnd time.Time) ExportLease {
	  now := time.Now().UTC()
	   return ExportLease{
		   ID:     uuid.New().String(),
		  UserID: userID,
		   Format:    format,
		  Root:      root,
		  HighWater: highWater,
		   State:     LeaseBuilt,
		  TTLSec:      ttlSec,
		  CreatedAt:   now,
		   UpdatedAt:   now,
		   WindowStart: wStart,
		   WindowEnd:   wEnd,
	   }
  }

  func (l *ExportLease) Issue(proofID string) error {
	  if l.State != LeaseBuilt {
		   return fmt.Errorf("cannot issue lease in state %q", l.State)
	   }
	  now := time.Now().UTC()
	   l.State = LeaseIssued
	  l.IssuedAt = now
	  l.ExpiresAt = now.Add(time.Duration(l.TTLSec) * time.Second)
	   l.ProofID = proofID
	   l.UpdatedAt = now
	   return nil
   }

  func (l *ExportLease) Expire() error {
	   if l.State != LeaseIssued {
		  return fmt.Errorf("cannot expire lease in state %q", l.State)
	   }
	  l.State = LeaseExpired
	   l.UpdatedAt = time.Now().UTC()
	   return nil
   }

  func (l *ExportLease) Revoke(reason string) error {
	   if l.State != LeaseIssued && l.State != LeaseBuilt {
		  return fmt.Errorf("cannot revoke lease in state %q", l.State)
	   }
	  l.State = LeaseRevoked
	   l.RevokedBy = reason
	  l.UpdatedAt = time.Now().UTC()
	   return nil
  }

  func (l *ExportLease) IsExpired() bool {
	   if l.State != LeaseIssued {
		  return false
	   }
	   return time.Now().UTC().After(l.ExpiresAt)
  }

// ──────────────────────────── RootDriftCase ────────────────────────────
// RootDriftCase 校验根漂移案例：live-book 新追加 Bill 导致 Manifest Root 相对
// 导出快照产生漂移，需记录、证明并关闭。
type RootDriftCase struct {
	ID      string `json:"id"`
	LeaseID string `json:"lease_id"`
	UserID  string `json:"user_id"`
	OriginalRoot string `json:"original_root"`
	CurrentRoot  string `json:"current_root"`
	HighWaterOld int    `json:"high_water_old"`
	HighWaterNew  int        `json:"high_water_new"`
	DriftBillSeqs []int      `json:"drift_bill_seqs,omitempty"`
	State         DriftState `json:"state"`
	CreatedAt     time.Time  `json:"created_at"`
	ProvenAt   time.Time `json:"proven_at,omitempty"`
	ClosedAt   time.Time `json:"closed_at,omitempty"`
	Resolution string    `json:"resolution,omitempty"`
}

  func NewRootDriftCase(leaseID, userID, origRoot, curRoot string, hwOld, hwNew int, seqs []int) RootDriftCase {
	  return RootDriftCase{
		   ID:      uuid.New().String(),
		   LeaseID: leaseID,
		  UserID:  userID,
		   OriginalRoot: origRoot,
		  CurrentRoot:  curRoot,
		  HighWaterOld: hwOld,
		  HighWaterNew:  hwNew,
		  DriftBillSeqs: seqs,
		   State:     DriftOpen,
		  CreatedAt: time.Now().UTC(),
	  }
   }

  func (d *RootDriftCase) Prove() error {
	  if d.State != DriftOpen {
		  return fmt.Errorf("cannot prove drift in state %q", d.State)
	  }
	   d.State = DriftProven
	  d.ProvenAt = time.Now().UTC()
	   return nil
  }

  func (d *RootDriftCase) Close(resolution string) error {
	  if d.State != DriftProven {
		   return fmt.Errorf("cannot close drift in state %q", d.State)
	   }
	  d.State = DriftClosed
	  d.ClosedAt = time.Now().UTC()
	  d.Resolution = resolution
	  return nil
  }

// ──────────────────────────── Revocation ────────────────────────────
// RevocationKind 撤销原因分类。

type RevocationKind string

const (
	RevokeTTLExpiry RevocationKind = "ttl_expiry"
	RevokeRootDrift RevocationKind = "root_drift"
	RevokeManual     RevocationKind = "manual"
	RevokeConcurrent RevocationKind = "concurrent_reexport"
)

// Revocation 撤销记录：过期或漂移触发后标记导出无效。
type Revocation struct {
	ID string `json:"id"`
	LeaseID string         `json:"lease_id"`
	UserID  string         `json:"user_id"`
	Kind    RevocationKind `json:"kind"`
	Reason string `json:"reason"`
	DriftID   string    `json:"drift_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Actor     string    `json:"actor"`
}

   func NewRevocation(leaseID, userID string, kind RevocationKind, reason, actor string) Revocation {
	  return Revocation{
		  ID: uuid.New().String(),
		   LeaseID: leaseID,
		   UserID:  userID,
		  Kind:    kind,
		  Reason: reason,
		  CreatedAt: time.Now().UTC(),
		  Actor: actor,
	   }
  }

// ──────────────────────────── IssueProof ────────────────────────────
// IssueProof 签发证明：user⊕window⊕Root⊕TTL 四元组。
// 缺失证明的导出不得标为已签发。
type IssueProof struct {
	ID      string `json:"id"`
	LeaseID string `json:"lease_id"`
	UserID  string `json:"user_id"`
	Root   string `json:"root"`
	TTLSec int    `json:"ttl_sec"`
	Digest string `json:"digest"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time `json:"window_end"`
}

   func NewIssueProof(leaseID, userID, root string, ttl int, wStart, wEnd time.Time) IssueProof {
	  now := time.Now().UTC()
	  p := IssueProof{
		  ID: uuid.New().String(),
		   LeaseID: leaseID,
		   UserID:  userID,
		  Root:    root,
		  TTLSec:  ttl,
		  IssuedAt: now,
		  ExpiresAt:   now.Add(time.Duration(ttl) * time.Second),
		  WindowStart: wStart,
		  WindowEnd:   wEnd,
	   }
	  p.Digest = p.computeDigest()
	   return p
  }

   func (p *IssueProof) computeDigest() string {
	  h := sha256.New()
	   h.Write([]byte(p.UserID))
	   h.Write([]byte{0})
	  h.Write([]byte(p.Root))
	   h.Write([]byte{0})
	  h.Write([]byte(fmt.Sprintf("%d", p.TTLSec)))
	  h.Write([]byte{0})
	   h.Write([]byte(p.WindowStart.Format(time.RFC3339)))
	  h.Write([]byte{0})
	  h.Write([]byte(p.WindowEnd.Format(time.RFC3339)))
	   return hex.EncodeToString(h.Sum(nil))
   }

   func (p *IssueProof) Verify() error {
	   if strings.TrimSpace(p.Digest) == "" {
		   return fmt.Errorf("proof digest empty")
	  }
	  want := p.computeDigest()
	  if p.Digest != want {
		   return fmt.Errorf("proof digest mismatch")
	  }
	   return nil
  }

   func (p *IssueProof) Expired() bool {
	  return time.Now().UTC().After(p.ExpiresAt)
   }

// ──────────────────────────── 辅助 ────────────────────────────
// VerifyExportIntegrity 验证导出是否仍有效：租约非终态、未过期、证明完整。
   func VerifyExportIntegrity(lease ExportLease, proof *IssueProof) error {
	   if lease.State.Terminal() {
		   return fmt.Errorf("lease %s in terminal state %s", lease.ID, lease.State)
	  }
	  if lease.State != LeaseIssued {
		   return fmt.Errorf("lease %s not yet issued (state=%s)", lease.ID, lease.State)
	  }
	   if lease.IsExpired() {
		  return fmt.Errorf("lease %s expired at %s", lease.ID, lease.ExpiresAt.Format(time.RFC3339))
	  }
	   if proof == nil {
		   return fmt.Errorf("lease %s missing issue proof", lease.ID)
	   }
	   if err := proof.Verify(); err != nil {
		   return fmt.Errorf("lease %s proof invalid: %w", lease.ID, err)
	   }
	   if proof.Expired() {
		  return fmt.Errorf("lease %s proof expired", lease.ID)
	   }
	  return nil
  }
