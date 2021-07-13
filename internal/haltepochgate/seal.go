package haltepochgate

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

   func (s *Service) openSlip(ctx context.Context, ep HaltEpoch, kind SlipKind, lane FundLane, carried int64, actor, summary, intentID string) *SlipProof {
	  if s == nil || s.store == nil {
		   return nil
	   }
	  if !kind.Valid() {
		  kind = SlipLaneMismatch
	  }
	   now := s.now()
	  p := SlipProof{
		   ProofID:    "sp-" + uuid.NewString(),
		   EpochID:    ep.EpochID,
		   Scope:      ep.Scope,
		   Generation: ep.Generation,
		  CarriedGen: carried,
		  Kind:       kind,
		  State: SlipOpen,
		   Lane: lane,
		   IntentID: intentID,
		   CaseID:   ep.WindowCase,
		  Actor:      strings.TrimSpace(actor),
		   Summary:    strings.TrimSpace(summary),
		  FailClosed: true,
		   Disposition: "reject",
		  OpenedAt: now,
	   }
	  p.Digest = ProofDigest(p)
	  if err := s.store.PutProof(ctx, p); err != nil {
		   return nil
	  }
	  if ep.WindowCase != "" {
		  if cs, err := s.store.GetCase(ctx, ep.WindowCase); err == nil && cs != nil {
			  cs.SlipCount++
			  _ = s.store.PutCase(ctx, *cs)
		   }
	   }
	  _ = s.store.AppendAudit(ctx, AuditEvent{
		   EventID: "ae-" + uuid.NewString(),
		   Kind:  "slip_open",
		  Scope: ep.Scope.String(),
		  Actor:  actor,
		   Detail: fmt.Sprintf("%s %s", kind, summary),
		  At:     now,
		   Generation: ep.Generation,
	   })
	  return &p
   }

  func (s *Service) revokeInflight(ctx context.Context, scope ScopeKey, newGen int64, caseID, actor string) (int, error) {
	  list, err := s.store.ListIntentsByScope(ctx, scope, []IntentState{IntentAllowed, IntentBroadcast}, 500)
	   if err != nil {
		  return 0, err
	   }
	   n := 0
	  now := s.now()
	  ep, _ := s.store.GetEpoch(ctx, scope)
	   if ep == nil {
		   ep = &HaltEpoch{Scope: scope, Generation: newGen, WindowCase: caseID}
	  }
	  for _, in := range list {
		   if in.Generation == newGen && in.State == IntentAllowed {
			   continue
		   }
		  if err := TransitionIntent(in.State, IntentRevoked); err != nil {
			  continue
		   }
		   kind := SlipStaleEpochBroadcast
		  if in.Lane == LaneDeposit {
			   kind = SlipStaleEpochCredit
		  }
		  p := s.openSlip(ctx, *ep, kind, in.Lane, in.Generation, actor,
			   fmt.Sprintf("revoke intent %s after epoch %d→%d", in.IntentID, in.Generation, newGen), in.IntentID)
		  in.State = IntentRevoked
		   if p != nil {
			   in.ProofID = p.ProofID
		  }
		   in.UpdatedAt = now
		   if err := s.store.PutIntent(ctx, in); err != nil {
			   return n, err
		  }
		  n++
	  }
	  return n, nil
  }

// ScanSlipWindow 巡检未封窗案与陈旧允许意图。
  func (s *Service) ScanSlipWindow(ctx context.Context, batch int) (revoked int, proofs int, err error) {
	  if s == nil || s.store == nil {
		  return 0, 0, ErrStoreUnavailable
	   }
	  if batch <= 0 {
		   batch = 100
	   }
	   cases, err := s.store.ListOpenCases(ctx, batch)
	  if err != nil {
		   return 0, 0, err
	   }
	   for _, cs := range cases {
		  n, e2 := s.revokeInflight(ctx, cs.Scope, cs.ClearGen, cs.CaseID, "window-scanner")
		   if e2 != nil {
			   return revoked, proofs, e2
		   }
		   revoked += n
		  cs.RevokedIntents += n
		   if cs.State == WindowOpen {
			  cs.State = WindowSealing
		   }
		  _ = s.store.PutCase(ctx, cs)
	  }
	   allowed, err := s.store.ListAllowedIntents(ctx, batch)
	  if err != nil {
		  return revoked, proofs, err
	  }
	  for _, in := range allowed {
		   ep, e2 := s.store.GetEpoch(ctx, in.Scope)
		   if e2 != nil {
			  continue
		   }
		   if ep.State.WindowSealed() || in.StaleVersus(ep.Generation) {
			   n, e3 := s.revokeInflight(ctx, in.Scope, ep.Generation, ep.WindowCase, "inflight-pump")
			  if e3 != nil {
				  return revoked, proofs, e3
			  }
			  revoked += n
		  }
	  }
	  open, err := s.store.ListProofs(ctx, SlipOpen, batch)
	  if err != nil {
		  return revoked, proofs, err
	  }
	  proofs = len(open)
	   return revoked, proofs, nil
   }

// ProveSlip open → proven。

   func (s *Service) ProveSlip(ctx context.Context, proofID string) (*SlipProof, error) {
	  p, err := s.store.GetProof(ctx, proofID)
	  if err != nil {
		  return nil, err
	  }
	  if err := TransitionSlip(p.State, SlipProven); err != nil {
		  return nil, err
	   }
	  p.State = SlipProven
	  p.ProvenAt = s.now()
	  p.Digest = ProofDigest(*p)
	  if err := s.store.PutProof(ctx, *p); err != nil {
		   return nil, err
	   }
	   return p, nil
  }

// CloseSlip proven/open → closed。

  func (s *Service) CloseSlip(ctx context.Context, proofID string) (*SlipProof, error) {
	  p, err := s.store.GetProof(ctx, proofID)
	   if err != nil {
		   return nil, err
	   }
	   if err := TransitionSlip(p.State, SlipClosed); err != nil {
		  return nil, err
	  }
	   p.State = SlipClosed
	   if p.ProvenAt.IsZero() {
		   p.ProvenAt = s.now()
	   }
	   p.ClosedAt = s.now()
	  p.Digest = ProofDigest(*p)
	  if err := s.store.PutProof(ctx, *p); err != nil {
		   return nil, err
	   }
	   return p, nil
   }

   func (s *Service) DrainOpenProofs(ctx context.Context, limit int) ([]SlipProof, error) {
	  list, err := s.store.ListProofs(ctx, SlipOpen, limit)
	   if err != nil {
		   return nil, err
	   }
	  out := make([]SlipProof, 0, len(list))
	  for _, p := range list {
		   np, err := s.ProveSlip(ctx, p.ProofID)
		  if err != nil {
			   continue
		  }
		   out = append(out, *np)
	  }
	  return out, nil
  }

// WindowStillOpen 清窗案未封。

  func WindowStillOpen(cs ClearWindowCase, now time.Time) bool {
	   _ = now
	   return cs.State == WindowOpen || cs.State == WindowSealing || cs.State == WindowConflict
  }
