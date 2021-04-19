// 重点：
// ResolveWithPayLease 在四眼结案后签发 dispute_pay_lease_id 或 claw_lease。
// AssertPayLease 是账本贷记前闸：无租约、过期、世代不符、证据根漂移均 fail-closed。
// 同案二次 Resolve 与幂等键重放不得二次放款，并落 DisputePayProof。
// 风险点：
// 结案与签发之间用 Engine.mu 串行；SQL 唯一键为第二道防线。
// 拒绝路径必须先落证明再返回错误。

// Author: Blaise
// Date: 2021.04.13

package disputepaygate

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/disputedesk"
	"account-assets.kucoin.net/funding/pkg/money"
)

// Engine 结案租约闸核心。
type Engine struct {
	desk *disputedesk.Service
	pays  PayLeaseRepo
	claws ClawLeaseRepo
	proofs ProofRepo
	audits AuditRepo
	gens   GenRepo
	clock func() time.Time
	mu    sync.Mutex
}

   func NewEngine(desk *disputedesk.Service, pays PayLeaseRepo, claws ClawLeaseRepo, proofs ProofRepo, audits AuditRepo, gens GenRepo) *Engine {
	   if desk == nil {
		   desk = disputedesk.New()
	  }
	   if pays == nil {
		   pays = NewMemPayLeaseRepo()
	   }
	  if claws == nil {
		  claws = NewMemClawLeaseRepo()
	  }
	   if proofs == nil {
		  proofs = NewMemProofRepo()
	  }
	  if audits == nil {
		  audits = NewMemAuditRepo()
	  }
	   if gens == nil {
		  gens = NewMemGenRepo()
	   }
	  return &Engine{
		   desk: desk,
		   pays: pays, claws: claws, proofs: proofs, audits: audits, gens: gens,
		   clock: func() time.Time { return time.Now().UTC() },
	  }
  }

  func (e *Engine) WithClock(c func() time.Time) *Engine {
	   if c != nil {
		   e.clock = c
	   }
	   return e
   }

   func (e *Engine) Desk() *disputedesk.Service { return e.desk }

   func (e *Engine) now() time.Time {
	  if e == nil || e.clock == nil {
		  return time.Now().UTC()
	   }
	  return e.clock()
   }

// ResolveReq 结案并签发租约入参。
type ResolveReq struct {
	CaseID string
	Actor    string
	Decision string
	Note     string
	TTL        time.Duration
	ReleaseRef string
}

// ResolveResult 结案签发结果。
type ResolveResult struct {
	Case disputedesk.Case
	Kind LeaseKind
	PayLease *DisputePayLease
	ClawLease  *ClawLease
	ProofID    string
	PayLeaseID string
}

// ResolveWithPayLease 四眼结案并签发放款或扣回租约。

  func (e *Engine) ResolveWithPayLease(ctx context.Context, req ResolveReq) (*ResolveResult, error) {
	   if e == nil {
		   return nil, ErrUnavailable
	   }
	  if strings.TrimSpace(req.CaseID) == "" || strings.TrimSpace(req.Actor) == "" {
		  return nil, ErrInvalidInput
	  }
	  dec, kind, err := NormalizeDecision(req.Decision)
	   if err != nil {
		   return nil, err
	  }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	   now := e.now()
	  cur, err := e.desk.Get(req.CaseID)
	  if err != nil {
		  return nil, fmt.Errorf("%w: %v", ErrCaseNotFound, err)
	  }
	  if cur.Status.Terminal() {
		   return e.rejectDoubleResolve(ctx, cur, kind, now)
	   }
	   if existing, gerr := e.pays.GetByCase(ctx, cur.ID); gerr == nil && existing != nil {
		  return e.rejectDoubleResolve(ctx, cur, KindPay, now)
	   }
	  if existing, gerr := e.claws.GetByCase(ctx, cur.ID); gerr == nil && existing != nil {
		   return e.rejectDoubleResolve(ctx, cur, KindClaw, now)
	   }
	  closed, rerr := e.desk.ResolveFourEye(req.CaseID, req.Actor, dec, req.Note)
	  if rerr != nil {
		   if rerr == disputedesk.ErrFourEye {
			   return nil, ErrFourEye
		   }
		  return nil, fmt.Errorf("%w: %v", ErrDesk, rerr)
	   }
	  dg, _ := e.desk.GenerationOf(closed.ID)
	   root := dg.EvidenceRoot
	  if root == "" {
		  root, _ = e.desk.EvidenceRoot(closed.ID)
	   }
	   _ = e.gens.Save(ctx, &CaseGeneration{
		   CaseID: closed.ID, PayGen: dg.PayGen, EvidenceRoot: root, UpdatedAt: now,
	   })
	   ttl := req.TTL
	  out := &ResolveResult{Case: closed, Kind: kind}
	   if kind == KindPay {
		  if ttl <= 0 {
			  ttl = DefaultPayLeaseTTL
		   }
		  lease := &DisputePayLease{
			   LeaseID: uuid.NewString(),
			  CaseID:      closed.ID,
			  Idempotency: closed.IdempotencyKey,
			   UserID: closed.UserID,
			  Amount: closed.Amount,
			  Asset:  closed.Asset,
			   EvidenceRoot: root,
			  Decision: dec,
			  PayGen: dg.PayGen,
			  State:    CaseResolved,
			   IssuedBy: req.Actor,
			   IssuedAt: now,
			   ExpireAt: now.Add(ttl),
		  }
		  if err := TransitionPay(lease, PayIssued); err != nil {
			   return nil, err
		   }
		  if err := e.pays.Save(ctx, lease); err != nil {
			  return nil, err
		  }
		  out.PayLease = lease
		   out.PayLeaseID = lease.LeaseID
		  recordPayIssued(closed.Asset)
	   } else {
		  if ttl <= 0 {
			   ttl = DefaultClawLeaseTTL
		  }
		  lease := &ClawLease{
			   LeaseID: uuid.NewString(),
			   CaseID:  closed.ID,
			   Idempotency: closed.IdempotencyKey,
			  UserID:       closed.UserID,
			   Amount:       closed.Amount,
			   Asset:        closed.Asset,
			  EvidenceRoot: root,
			  Decision: dec,
			   PayGen:     dg.PayGen,
			  State:      CaseResolved,
			   ReleaseRef: strings.TrimSpace(req.ReleaseRef),
			  IssuedBy: req.Actor,
			   IssuedAt: now,
			   ExpireAt: now.Add(ttl),
		   }
		  if err := TransitionClaw(lease, ClawIssued); err != nil {
			  return nil, err
		  }
		   if err := e.claws.Save(ctx, lease); err != nil {
			   return nil, err
		   }
		  out.ClawLease = lease
		  out.PayLeaseID = lease.LeaseID
		  recordClawIssued(closed.Asset)
	   }
	   pid, _ := e.saveProof(ctx, closed.ID, dg.PayGen, out.PayLeaseID, "", kind, OutcomePaid, "lease_issued", now)
	   if kind == KindClaw {
		  pid, _ = e.saveProof(ctx, closed.ID, dg.PayGen, out.PayLeaseID, "", kind, OutcomeClawed, "claw_issued", now)
	   }
	  out.ProofID = pid
	  _ = e.audits.Save(ctx, &AuditRow{
		   AuditID: uuid.NewString(), CaseID: closed.ID, LeaseID: out.PayLeaseID,
		   Actor: req.Actor, Action: "resolve_issue", Detail: dec, At: now,
	  })
	   return out, nil
  }

  func (e *Engine) rejectDoubleResolve(ctx context.Context, cur disputedesk.Case, kind LeaseKind, now time.Time) (*ResolveResult, error) {
	  dg, _ := e.desk.GenerationOf(cur.ID)
	  leaseID := ""
	   if kind == KindPay {
		   if l, err := e.pays.GetByCase(ctx, cur.ID); err == nil {
			   leaseID = l.LeaseID
		  }
	   } else {
		  if l, err := e.claws.GetByCase(ctx, cur.ID); err == nil {
			  leaseID = l.LeaseID
		   }
	   }
	   pid, _ := e.saveProof(ctx, cur.ID, dg.PayGen, leaseID, "", kind, OutcomeBlockedDouble, "second_resolve", now)
	   recordBlocked("resolve", string(OutcomeBlockedDouble))
	   return &ResolveResult{Case: cur, Kind: kind, ProofID: pid, PayLeaseID: leaseID}, ErrDoubleResolve
   }

// OpenAfterResolve 结案后再 Open 同幂等键：返回原案且不得签发新租约。
   func (e *Engine) OpenAfterResolve(ctx context.Context, req disputedesk.OpenReq) (disputedesk.Case, error) {
	   if e == nil {
		   return disputedesk.Case{}, ErrUnavailable
	  }
	   got, err := e.desk.Open(req)
	  if err != nil {
		   return disputedesk.Case{}, err
	   }
	  if !got.Status.Terminal() {
		   return got, nil
	  }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	  now := e.now()
	   if _, perr := e.pays.GetByIdem(ctx, got.IdempotencyKey); perr == nil {
		   _, _ = e.saveProof(ctx, got.ID, 0, "", "", KindPay, OutcomeBlockedReplay, "open_after_resolve", now)
		  recordBlocked("open", string(OutcomeBlockedReplay))
		   return got, ErrIdemReplayPay
	   }
	  if _, cerr := e.claws.GetByCase(ctx, got.ID); cerr == nil {
		   _, _ = e.saveProof(ctx, got.ID, 0, "", "", KindClaw, OutcomeBlockedReplay, "open_after_resolve", now)
		   return got, ErrIdemReplayPay
	   }
	  return got, ErrIdemReplayPay
  }

// AssertPayInput 账本贷记前复验入参。

type AssertPayInput struct {
	LeaseID string
	CaseID string
	Amount string
	Asset  string
	LedgerID  string
	WriteKind string
}

// AssertResult 复验输出。

type AssertResult struct {
	Allowed bool
	Lease   *DisputePayLease
	Claw *ClawLease
	ProofID string
	Reason  ProofOutcome
	Detail     string
	PayLeaseID string
}

// AssertPayLease 账本贷记前复验放款租约。

   func (e *Engine) AssertPayLease(ctx context.Context, in AssertPayInput) (*AssertResult, error) {
	  if e == nil {
		   return nil, ErrUnavailable
	   }
	  now := e.now()
	   if strings.TrimSpace(in.LeaseID) == "" {
		  pid, _ := e.saveProof(ctx, in.CaseID, 0, "", in.LedgerID, KindPay, OutcomeBlockedNoLease, "missing_pay_lease", now)
		   recordBlocked("credit", string(OutcomeBlockedNoLease))
		   return &AssertResult{Allowed: false, ProofID: pid, Reason: OutcomeBlockedNoLease, Detail: "missing dispute_pay_lease_id"}, ErrNoLeaseForCredit
	   }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	  lease, err := e.pays.Get(ctx, strings.TrimSpace(in.LeaseID))
	   if err != nil {
		  pid, _ := e.saveProof(ctx, in.CaseID, 0, in.LeaseID, in.LedgerID, KindPay, OutcomeBlockedNoLease, "lease_not_found", now)
		   recordBlocked("credit", string(OutcomeBlockedNoLease))
		  return &AssertResult{Allowed: false, ProofID: pid, Reason: OutcomeBlockedNoLease}, ErrNoLeaseForCredit
	   }
	   if in.CaseID != "" && lease.CaseID != in.CaseID {
		   pid, _ := e.saveProof(ctx, in.CaseID, lease.PayGen, lease.LeaseID, in.LedgerID, KindPay, OutcomeBlockedReplay, "case_mismatch", now)
		  return &AssertResult{Allowed: false, ProofID: pid, Reason: OutcomeBlockedReplay, PayLeaseID: lease.LeaseID}, ErrDecisionMismatch
	   }
	   if lease.State == PayConsumed {
		  pid, _ := e.saveProof(ctx, lease.CaseID, lease.PayGen, lease.LeaseID, in.LedgerID, KindPay, OutcomeBlockedReplay, "already_consumed", now)
		  return &AssertResult{Allowed: false, ProofID: pid, Reason: OutcomeBlockedReplay, PayLeaseID: lease.LeaseID}, ErrLeaseConsumed
	  }
	   if lease.State == LeaseRevoked {
		  pid, _ := e.saveProof(ctx, lease.CaseID, lease.PayGen, lease.LeaseID, in.LedgerID, KindPay, OutcomeBlockedRevoke, "revoked", now)
		   return &AssertResult{Allowed: false, ProofID: pid, Reason: OutcomeBlockedRevoke, PayLeaseID: lease.LeaseID}, ErrLeaseRevoked
	   }
	   if lease.State == LeaseExpired || !now.Before(lease.ExpireAt) {
		  if lease.State == PayIssued {
			  _ = TransitionPay(lease, LeaseExpired)
			   _ = e.pays.Save(ctx, lease)
		  }
		   pid, _ := e.saveProof(ctx, lease.CaseID, lease.PayGen, lease.LeaseID, in.LedgerID, KindPay, OutcomeBlockedExpire, "expired", now)
		   return &AssertResult{Allowed: false, ProofID: pid, Reason: OutcomeBlockedExpire, PayLeaseID: lease.LeaseID}, ErrLeaseExpired
	   }
	  if err := e.recheckDesk(lease.CaseID, lease.PayGen, lease.EvidenceRoot, now, lease.LeaseID, in.LedgerID, KindPay); err != nil {
		   reason := OutcomeBlockedDrift
		   if err == ErrGenMismatch {
			  reason = OutcomeBlockedGen
		  }
		   pid, _ := e.saveProof(ctx, lease.CaseID, lease.PayGen, lease.LeaseID, in.LedgerID, KindPay, reason, err.Error(), now)
		  recordBlocked("credit", string(reason))
		   return &AssertResult{Allowed: false, ProofID: pid, Reason: reason, PayLeaseID: lease.LeaseID}, err
	  }
	  if in.Asset != "" && !strings.EqualFold(in.Asset, lease.Asset) {
		  return &AssertResult{Allowed: false, PayLeaseID: lease.LeaseID}, ErrAssetMismatch
	   }
	  if in.Amount != "" {
		   want, e1 := money.Parse(lease.Amount)
		   got, e2 := money.Parse(in.Amount)
		   if e1 != nil || e2 != nil || want.String() != got.String() {
			  return &AssertResult{Allowed: false, PayLeaseID: lease.LeaseID}, ErrAmountMismatch
		  }
	   }
	   lease.BoundLedger = strings.TrimSpace(in.LedgerID)
	  if err := e.pays.Save(ctx, lease); err != nil {
		   return nil, err
	   }
	   recordAssertOK("credit")
	   return &AssertResult{Allowed: true, Lease: lease, PayLeaseID: lease.LeaseID}, nil
  }

// AssertClawLease 账本借记/释放前复验扣回租约。

  func (e *Engine) AssertClawLease(ctx context.Context, in AssertPayInput) (*AssertResult, error) {
	   if e == nil {
		  return nil, ErrUnavailable
	  }
	  now := e.now()
	   if strings.TrimSpace(in.LeaseID) == "" {
		   pid, _ := e.saveProof(ctx, in.CaseID, 0, "", in.LedgerID, KindClaw, OutcomeBlockedNoLease, "missing_claw_lease", now)
		   return &AssertResult{Allowed: false, ProofID: pid, Reason: OutcomeBlockedNoLease}, ErrNoLeaseForDebit
	  }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	   lease, err := e.claws.Get(ctx, strings.TrimSpace(in.LeaseID))
	  if err != nil {
		   pid, _ := e.saveProof(ctx, in.CaseID, 0, in.LeaseID, in.LedgerID, KindClaw, OutcomeBlockedNoLease, "claw_not_found", now)
		   return &AssertResult{Allowed: false, ProofID: pid, Reason: OutcomeBlockedNoLease}, ErrNoLeaseForDebit
	  }
	   if lease.State == ClawConsumed {
		   return &AssertResult{Allowed: false, PayLeaseID: lease.LeaseID}, ErrLeaseConsumed
	  }
	  if lease.State == LeaseRevoked {
		  return &AssertResult{Allowed: false, PayLeaseID: lease.LeaseID}, ErrLeaseRevoked
	  }
	  if !now.Before(lease.ExpireAt) {
		   _ = TransitionClaw(lease, LeaseExpired)
		   _ = e.claws.Save(ctx, lease)
		  pid, _ := e.saveProof(ctx, lease.CaseID, lease.PayGen, lease.LeaseID, in.LedgerID, KindClaw, OutcomeBlockedExpire, "expired", now)
		  return &AssertResult{Allowed: false, ProofID: pid, Reason: OutcomeBlockedExpire, PayLeaseID: lease.LeaseID}, ErrLeaseExpired
	   }
	  if err := e.recheckDesk(lease.CaseID, lease.PayGen, lease.EvidenceRoot, now, lease.LeaseID, in.LedgerID, KindClaw); err != nil {
		  reason := OutcomeBlockedDrift
		   if err == ErrGenMismatch {
			   reason = OutcomeBlockedGen
		  }
		   pid, _ := e.saveProof(ctx, lease.CaseID, lease.PayGen, lease.LeaseID, in.LedgerID, KindClaw, reason, err.Error(), now)
		   return &AssertResult{Allowed: false, ProofID: pid, Reason: reason, PayLeaseID: lease.LeaseID}, err
	  }
	   lease.BoundLedger = strings.TrimSpace(in.LedgerID)
	  if lease.ReleaseRef == "" && in.WriteKind != "" {
		   lease.ReleaseRef = in.WriteKind
	  }
	  _ = e.claws.Save(ctx, lease)
	   return &AssertResult{Allowed: true, Claw: lease, PayLeaseID: lease.LeaseID}, nil
   }

  func (e *Engine) recheckDesk(caseID string, payGen int64, root string, now time.Time, leaseID, ledgerID string, kind LeaseKind) error {
	   _ = now
	  _ = leaseID
	  _ = ledgerID
	   _ = kind
	   dg, err := e.desk.GenerationOf(caseID)
	   if err != nil {
		   return ErrCaseNotFound
	   }
	  curRoot, err := e.desk.EvidenceRoot(caseID)
	   if err != nil {
		  return err
	   }
	   if !strings.EqualFold(curRoot, root) || !strings.EqualFold(dg.EvidenceRoot, root) {
		   return ErrEvidenceDrift
	  }
	  if dg.PayGen != payGen {
		  return ErrGenMismatch
	   }
	  return nil
  }

// ConsumePayLease 账本贷记成功后消费放款租约。

  func (e *Engine) ConsumePayLease(ctx context.Context, leaseID, ledgerID string) (*DisputePayLease, error) {
	  if e == nil {
		  return nil, ErrUnavailable
	  }
	   e.mu.Lock()
	   defer e.mu.Unlock()
	   lease,
		  err := e.pays.Get(ctx, strings.TrimSpace(leaseID))
	   if err != nil {
		   return nil, err
	   }
	   if err := TransitionPay(lease, PayConsumed); err != nil {
		   return nil, err
	  }
	   now := e.now()
	  lease.BoundLedger = strings.TrimSpace(ledgerID)
	   t := now
	  lease.ConsumedAt = &t
	   if err := e.pays.Save(ctx, lease); err != nil {
		   return nil, err
	   }
	  _, _ = e.saveProof(ctx, lease.CaseID, lease.PayGen, lease.LeaseID, ledgerID, KindPay, OutcomePaid, "consumed", now)
	  return lease, nil
  }

// ConsumeClawLease 扣回提交成功后消费 claw 租约。

   func (e *Engine) ConsumeClawLease(ctx context.Context, leaseID, ledgerID string) (*ClawLease, error) {
	   if e == nil {
		   return nil, ErrUnavailable
	   }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	  lease, err := e.claws.Get(ctx, strings.TrimSpace(leaseID))
	   if err != nil {
		  return nil, err
	   }
	  if err := TransitionClaw(lease, ClawConsumed); err != nil {
		   return nil, err
	   }
	   now := e.now()
	   lease.BoundLedger = strings.TrimSpace(ledgerID)
	   t := now
	  lease.ConsumedAt = &t
	  if err := e.claws.Save(ctx, lease); err != nil {
		  return nil, err
	  }
	   _, _ = e.saveProof(ctx, lease.CaseID, lease.PayGen, lease.LeaseID, ledgerID, KindClaw, OutcomeClawed, "consumed", now)
	   return lease, nil
   }

// RevokeLease 运维吊销未消费租约。

   func (e *Engine) RevokeLease(ctx context.Context, leaseID, actor, note string) error {
	   if e == nil {
		  return ErrUnavailable
	   }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	   now := e.now()
	   if l, err := e.pays.Get(ctx, leaseID); err == nil {
		  if err := TransitionPay(l, LeaseRevoked); err != nil {
			  return err
		  }
		   t := now
		  l.RevokedAt = &t
		  l.RevokeNote = strings.TrimSpace(note)
		  if err := e.pays.Save(ctx, l); err != nil {
			   return err
		  }
		  _, _ = e.saveProof(ctx, l.CaseID, l.PayGen, l.LeaseID, "", KindPay, OutcomeBlockedRevoke, note, now)
		  _ = e.audits.Save(ctx, &AuditRow{AuditID: uuid.NewString(), CaseID: l.CaseID, LeaseID: l.LeaseID, Actor: actor, Action: "revoke_pay", Detail: note, At: now})
		   recordRevoked("pay")
		  return nil
	  }
	  if l, err := e.claws.Get(ctx, leaseID); err == nil {
		   if err := TransitionClaw(l, LeaseRevoked); err != nil {
			   return err
		   }
		  t := now
		   l.RevokedAt = &t
		   l.RevokeNote = strings.TrimSpace(note)
		  if err := e.claws.Save(ctx, l); err != nil {
			   return err
		   }
		   _, _ = e.saveProof(ctx, l.CaseID, l.PayGen, l.LeaseID, "", KindClaw, OutcomeBlockedRevoke, note, now)
		  _ = e.audits.Save(ctx, &AuditRow{AuditID: uuid.NewString(), CaseID: l.CaseID, LeaseID: l.LeaseID, Actor: actor, Action: "revoke_claw", Detail: note, At: now})
		  recordRevoked("claw")
		   return nil
	   }
	   return ErrLeaseNotFound
  }

   func (e *Engine) GetPayLease(ctx context.Context, id string) (*DisputePayLease, error) {
	  if e == nil {
		  return nil, ErrUnavailable
	  }
	   return e.pays.Get(ctx, strings.TrimSpace(id))
   }

  func (e *Engine) GetClawLease(ctx context.Context, id string) (*ClawLease, error) {
	  if e == nil {
		   return nil, ErrUnavailable
	  }
	   return e.claws.Get(ctx,
		   strings.TrimSpace(id))
  }

   func (e *Engine) ExportProof(ctx context.Context, proofID string) (*DisputePayProof, error) {
	   if e == nil {
		   return nil, ErrUnavailable
	   }
	   return e.proofs.Get(ctx, strings.TrimSpace(proofID))
   }

  func (e *Engine) ListProofs(ctx context.Context, caseID string, limit int) ([]*DisputePayProof, error) {
	   if e == nil {
		   return nil, ErrUnavailable
	   }
	  if limit <= 0 {
		  limit = 50
	  }
	  return e.proofs.ListByCase(ctx, strings.TrimSpace(caseID), limit)
   }

   func (e *Engine) ListAudit(ctx context.Context, caseID string, limit int) ([]*AuditRow, error) {
	   if e == nil {
		  return nil, ErrUnavailable
	  }
	  if limit <= 0 {
		  limit = 50
	  }
	   return e.audits.ListByCase(ctx, strings.TrimSpace(caseID), limit)
  }

   func (e *Engine) saveProof(ctx context.Context, caseID string, payGen int64, leaseID, ledgerID string, kind LeaseKind, outcome ProofOutcome, detail string, now time.Time) (string, error) {
	  p := &DisputePayProof{
		   ProofID: ProofIDFor(caseID, payGen, outcome, leaseID+detail+now.Format(time.RFC3339Nano)),
		   CaseID:  caseID,
		   PayGen:   payGen,
		   LedgerID: ledgerID,
		  LeaseID: leaseID,
		  Kind:    kind,
		  Outcome: outcome,
		  Detail:    detail,
		   CreatedAt: now,
	   }
	   p.Digest = p.ComputeDigest()
	  if err := e.proofs.Save(ctx, p); err != nil {
		  return "", err
	   }
	   recordProof(string(outcome))
	   return p.ProofID, nil
   }
