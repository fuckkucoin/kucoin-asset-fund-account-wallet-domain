// 重点：
// IssueMintLease 在证明集齐全后签发 mint_lease_id。
// AssertLease 是 Journal/ledger 前闸：无租约、过期、世代不符、指纹漂移均 fail-closed。
// ClearAttest 抬升世代并吊销活租约。
// 风险点：
// 签发与复验之间用 Engine.mu 串行；SQL 唯一键为第二道防线。
// 拒绝路径必须先落证明再返回错误。

// Author: Draven
// Date: 2021.11.18

package stablemintgate

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/stablebr"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

// Engine 铸赎租约闸核心。

type Engine struct {
	br *stablebr.Service
	leases LeaseRepo
	halves HalfRepo
	audits AuditRepo
	gens   GenRepo
	proofs ProofRepo
	clock  func() time.Time
	mu     sync.Mutex
}

  func NewEngine(br *stablebr.Service, leases LeaseRepo, halves HalfRepo, audits AuditRepo, gens GenRepo, proofs ProofRepo) *Engine {
	   if br == nil {
		   br = stablebr.New()
	  }
	  if leases == nil {
		   leases = NewMemLeaseRepo()
	   }
	   if halves == nil {
		   halves = NewMemHalfRepo()
	  }
	  if audits == nil {
		  audits = NewMemAuditRepo()
	  }
	  if gens == nil {
		  gens = NewMemGenRepo()
	   }
	   if proofs == nil {
		   proofs = NewMemProofRepo()
	  }
	  e := &Engine{
		  br: br, leases: leases, halves: halves, audits: audits, gens: gens, proofs: proofs,
		   clock: func() time.Time { return time.Now().UTC() },
	   }
	   br.EnableJournalFence(true)
	   br.BindPostGuard(e)
	  br.OnClearAttest(e.onClearAttest)
	  return e
  }

   func (e *Engine) WithClock(c func() time.Time) *Engine {
	   if c != nil {
		  e.clock = c
	  }
	  return e
  }

  func (e *Engine) Bridge() *stablebr.Service { return e.br }

   func (e *Engine) now() time.Time {
	  if e == nil || e.clock == nil {
		  return time.Now().UTC()
	  }
	   return e.clock()
  }

  func (e *Engine) onClearAttest(kind stablebr.Kind, asset, actor string) {
	   if e == nil {
		   return
	  }
	   ctx := context.Background()
	  lk, err := ParseLeaseKind(string(kind))
	   if err != nil {
		   return
	   }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	   now := e.now()
	  scope := ScopeKey(lk, asset)
	  g, _ := e.gens.Get(ctx, scope)
	   if g == nil {
		  g = &MintGeneration{Scope: scope, Kind: string(lk), Asset: asset}
	  }
	  g.MintGen++
	  g.Kind = string(lk)
	   g.Asset = asset
	   g.LastClear = actor
	   g.UpdatedAt = now
	   _ = e.gens.Save(ctx, g)
	   rows, _ := e.leases.ListByScope(ctx, scope, 200)
	  for _, l := range rows {
		   if l == nil || !l.State.Live() {
			  continue
		  }
		  _ = e.revokeLocked(ctx, l, actor, "clear_attest", OutcomeBlockedClear)
	  }
	   recordRevoked("clear")
  }

// IssueMintLease 证明齐全后签发租约。
  func (e *Engine) IssueMintLease(ctx context.Context, req IssueReq) (*MintLease, error) {
	   if e == nil {
		   return nil, ErrUnavailable
	   }
	  kind, err := ParseLeaseKind(req.Kind)
	   if err != nil {
		  return nil, err
	   }
	  actor, err := validate.Actor(req.Actor)
	   if err != nil {
		  return nil, ErrInvalidInput
	  }
	  now := e.now()
	  e.mu.Lock()
	  defer e.mu.Unlock()
	  fp, err := e.br.CurrentFingerprint(string(kind), req.Asset)
	   if err != nil {
		  return nil, fmt.Errorf("%w: %v", ErrBridge, err)
	  }
	  af := FingerprintFrom(fp)
	   if !af.Complete {
		  _ = e.denyLocked(ctx, "", ScopeKey(kind, req.Asset), kind, req.Asset, OutcomeBlockedExpire, "attest incomplete or expired")
		  if !fp.Expiry.IsZero() && !fp.Expiry.After(now) {
			   return nil, ErrAttestExpired
		  }
		   return nil, ErrAttestIncomplete
	  }
	  if !af.Valid(now) {
		   _ = e.denyLocked(ctx, "", ScopeKey(kind, req.Asset), kind, req.Asset, OutcomeBlockedExpire, "fingerprint expired")
		  return nil, ErrAttestExpired
	  }
	  ttl := req.TTL
	   if ttl <= 0 {
		  ttl = DefaultMintLeaseTTL
	   }
	   if ttl > MaxMintLeaseTTL {
		   ttl = MaxMintLeaseTTL
	  }
	   expire := now.Add(ttl)
	  if af.Expiry.Before(expire) {
		   expire = af.Expiry
	   }
	  if !expire.After(now) {
		   return nil, ErrAttestExpired
	  }
	  scope := ScopeKey(kind, af.Asset)
	  g, _ := e.gens.Get(ctx, scope)
	   gen := int64(0)
	  if g != nil {
		  gen = g.MintGen
	  }
	   lease := &MintLease{
		  LeaseID: uuid.NewString(),
		   Kind: kind,
		  Asset:  af.Asset,
		   UserID: strings.TrimSpace(req.UserID),
		  Amount: strings.TrimSpace(req.Amount),
		  Fingerprint: af,
		   MintGen:     gen,
		  State: LeaseIssued,
		   IssuedBy: actor,
		  IssuedAt: now,
		  ExpireAt: expire,
	   }
	  if err := e.leases.Save(ctx, lease); err != nil {
		  return nil, err
	   }
	  _ = e.auditLocked(ctx, lease.LeaseID, scope, actor, "issue", "mint_lease issued", string(OutcomeIssued))
	  recordIssued(string(kind), af.Asset)
	  cp := lease.Copy()
	  return &cp, nil
   }

// AssertLease 落账前复验租约、指纹、世代与证明 Expiry。
  func (e *Engine) AssertLease(ctx context.Context, in AssertInput) (*AssertResult, error) {
	   if e == nil {
		  return nil, ErrUnavailable
	  }
	  if strings.TrimSpace(in.LeaseID) == "" {
		  pr := e.denyUnlocked(ctx, "", ScopeKey(KindMint, in.Asset), KindMint, in.Asset, OutcomeBlockedNoLease, "missing mint_lease_id")
		   recordBlocked("assert", "no_lease")
		   return &AssertResult{Allowed: false, Reason: OutcomeBlockedNoLease, ProofID: pr}, ErrNoLease
	   }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	   return e.assertLocked(ctx, in)
  }

   func (e *Engine) assertLocked(ctx context.Context, in AssertInput) (*AssertResult, error) {
	  now := e.now()
	  lease, err := e.leases.Get(ctx, in.LeaseID)
	   if err != nil {
		  pr := e.denyLocked(ctx, in.LeaseID, ScopeKey(KindMint, in.Asset), KindMint, in.Asset, OutcomeBlockedNoLease, "lease not found")
		  recordBlocked("assert", "not_found")
		  return &AssertResult{Allowed: false, Reason: OutcomeBlockedNoLease, ProofID: pr}, ErrLeaseNotFound
	  }
	   kind := lease.Kind
	   if in.Kind != "" {
		   k, kerr := ParseLeaseKind(in.Kind)
		   if kerr != nil || k != lease.Kind {
			   pr := e.denyLocked(ctx, lease.LeaseID, ScopeKey(lease.Kind, lease.Asset), lease.Kind, lease.Asset, OutcomeBlockedDrift, "kind mismatch")
			   return &AssertResult{Allowed: false,
				  Lease: lease, Reason: OutcomeBlockedDrift, ProofID: pr}, ErrKindMismatch
		   }
		   kind = k
	  }
	   if in.Asset != "" && !strings.EqualFold(in.Asset, lease.Asset) {
		  pr := e.denyLocked(ctx, lease.LeaseID, ScopeKey(lease.Kind, lease.Asset), lease.Kind, lease.Asset, OutcomeBlockedDrift, "asset mismatch")
		  return &AssertResult{Allowed: false, Lease: lease, Reason: OutcomeBlockedDrift, ProofID: pr}, ErrAssetMismatch
	   }
	  if lease.State == LeaseRevoked {
		  pr := e.denyLocked(ctx, lease.LeaseID, ScopeKey(lease.Kind, lease.Asset), lease.Kind, lease.Asset, OutcomeBlockedRevoke, "revoked")
		  recordBlocked("assert", "revoked")
		  return &AssertResult{Allowed: false, Lease: lease, Reason: OutcomeBlockedRevoke, ProofID: pr}, ErrLeaseRevoked
	   }
	   if lease.State == LeaseConsumed || lease.State == LeasePosted {
		   pr := e.denyLocked(ctx, lease.LeaseID, ScopeKey(lease.Kind, lease.Asset), lease.Kind, lease.Asset, OutcomeBlockedReplay, "already consumed")
		   return &AssertResult{Allowed: false, Lease: lease, Reason: OutcomeBlockedReplay, ProofID: pr}, ErrLeaseConsumed
	   }
	  if lease.State == LeaseExpired || !now.Before(lease.ExpireAt) {
		   if lease.State.Live() {
			  _ = TransitionLease(lease, LeaseExpired)
			   _ = e.leases.Save(ctx, lease)
		  }
		   pr := e.denyLocked(ctx, lease.LeaseID, ScopeKey(lease.Kind, lease.Asset), lease.Kind, lease.Asset, OutcomeBlockedLeaseExp, "lease ttl")
		   recordBlocked("assert", "lease_expired")
		   return &AssertResult{Allowed: false, Lease: lease, Reason: OutcomeBlockedLeaseExp, ProofID: pr}, ErrLeaseExpired
	  }
	  scope := ScopeKey(kind, lease.Asset)
	  g, _ := e.gens.Get(ctx, scope)
	  if g != nil && g.MintGen != lease.MintGen {
		  _ = e.revokeLocked(ctx, lease, "system-gen", "generation mismatch", OutcomeBlockedGen)
		  pr := e.denyLocked(ctx, lease.LeaseID, scope, kind, lease.Asset, OutcomeBlockedGen, fmt.Sprintf("lease_gen=%d current=%d", lease.MintGen, g.MintGen))
		  recordBlocked("assert", "gen")
		  return &AssertResult{Allowed: false,
			   Lease: lease, Reason: OutcomeBlockedGen, ProofID: pr}, ErrGenMismatch
	  }
	   if err := e.br.MatchFingerprint(kind.BridgeKind(), lease.Asset, lease.Fingerprint.Digest); err != nil {
		   outcome := OutcomeBlockedDrift
		   ret := ErrFingerprintDrift
		   if err == stablebr.ErrExpired {
			  outcome = OutcomeBlockedExpire
			   ret = ErrAttestExpired
		  } else if err == stablebr.ErrAttest {
			   outcome = OutcomeBlockedExpire
			   ret = ErrAttestIncomplete
		   }
		   _ = e.revokeLocked(ctx, lease, "system-fp", err.Error(), outcome)
		  pr := e.denyLocked(ctx, lease.LeaseID, scope, kind, lease.Asset, outcome, err.Error())
		   recordBlocked("assert", string(outcome))
		  return &AssertResult{Allowed: false, Lease: lease, Reason: outcome, ProofID: pr}, ret
	  }
	   recordAssertOK(string(kind))
	  cp := lease.Copy()
	   return &AssertResult{Allowed: true, Lease: &cp}, nil
  }

// Revoke 运维吊销。
   func (e *Engine) Revoke(ctx context.Context, leaseID, actor, note string) (*MintLease, error) {
	   if e == nil {
		  return nil, ErrUnavailable
	  }
	   if strings.TrimSpace(leaseID) == "" {
		   return nil, ErrInvalidInput
	  }
	  if _, err := validate.Actor(actor); err != nil {
		   return nil, ErrInvalidInput
	  }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	   lease, err := e.leases.Get(ctx, leaseID)
	  if err != nil {
		   return nil, err
	   }
	   if err := e.revokeLocked(ctx, lease, actor, note, OutcomeBlockedRevoke); err != nil {
		   return nil, err
	   }
	  cp := lease.Copy()
	  return &cp, nil
  }

  func (e *Engine) revokeLocked(ctx context.Context, lease *MintLease, actor, note string, outcome ProofOutcome) error {
	  if lease == nil {
		   return ErrLeaseNotFound
	  }
	  if lease.State.Terminal() && lease.State != LeasePosted {
		   return nil
	  }
	  if err := TransitionLease(lease, LeaseRevoked); err != nil {
		  return err
	   }
	  now := e.now()
	  lease.RevokedAt = &now
	   lease.RevokeNote = note
	   lease.RevokeReason = string(outcome)
	   if err := e.leases.Save(ctx, lease); err != nil {
		  return err
	   }
	  _ = e.auditLocked(ctx, lease.LeaseID, ScopeKey(lease.Kind, lease.Asset), actor, "revoke", note, string(outcome))
	   recordRevoked(string(lease.Kind))
	   return nil
  }

// GetLease 只读导出 mint_lease_id。

  func (e *Engine) GetLease(ctx context.Context, leaseID string) (*MintLease, error) {
	   if e == nil {
		   return nil, ErrUnavailable
	   }
	   return e.leases.Get(ctx, leaseID)
  }

   func (e *Engine) ListProofs(ctx context.Context, scope string, limit int) ([]*DenyProof, error) {
	  if e == nil {
		   return nil, ErrUnavailable
	   }
	  return e.proofs.ListByScope(ctx, scope, limit)
  }

  func (e *Engine) GetProof(ctx context.Context, proofID string) (*DenyProof, error) {
	  if e == nil {
		  return nil, ErrUnavailable
	  }
	  return e.proofs.Get(ctx, proofID)
   }

  func (e *Engine) ListHalf(ctx context.Context, leaseID string, limit int) ([]*StableHalfProof, error) {
	   if e == nil {
		  return nil, ErrUnavailable
	   }
	  if leaseID != "" {
		   return e.halves.ListByLease(ctx, leaseID, limit)
	   }
	   return e.halves.ListOpen(ctx, limit)
  }

  func (e *Engine) ListAudit(ctx context.Context, leaseID string, limit int) ([]*MintAudit, error) {
	  if e == nil {
		  return nil, ErrUnavailable
	   }
	  return e.audits.ListByLease(ctx, leaseID, limit)
  }

   func (e *Engine) denyLocked(ctx context.Context, leaseID, scope string, kind LeaseKind, asset string, outcome ProofOutcome, detail string) string {
	   p := &DenyProof{
		  ProofID: uuid.NewString(), LeaseID: leaseID, Scope: scope, Kind: kind, Asset: asset,
		   Outcome: outcome, Detail: detail, CreatedAt: e.now(),
	   }
	   p.Digest = p.ComputeDigest()
	   _ = e.proofs.Save(ctx, p)
	   recordProof(string(outcome))
	  return p.ProofID
   }

   func (e *Engine) denyUnlocked(ctx context.Context, leaseID, scope string, kind LeaseKind, asset string, outcome ProofOutcome, detail string) string {
	  e.mu.Lock()
	  defer e.mu.Unlock()
	   return e.denyLocked(ctx, leaseID, scope, kind, asset, outcome, detail)
   }

  func (e *Engine) auditLocked(ctx context.Context, leaseID, scope, actor, action, detail, outcome string) error {
	   a := &MintAudit{
		   AuditID: uuid.NewString(), LeaseID: leaseID, Scope: scope, Actor: actor,
		   Action: action, Detail: detail, Outcome: outcome, At: e.now(),
	  }
	   return e.audits.Save(ctx, a)
   }

  func parseAmount(s string) error {
	  if strings.TrimSpace(s) == "" {
		  return nil
	  }
	  _, err := money.Parse(s)
	  if err != nil {
		  return ErrAmount
	  }
	  return nil
   }
