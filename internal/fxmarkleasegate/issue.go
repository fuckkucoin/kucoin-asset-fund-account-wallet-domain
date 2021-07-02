package fxmarkleasegate

import (
	"context"
	"strings"
)

  func (e *Engine) IssueMarkLease(ctx context.Context, req IssueMarkLeaseReq) (IssueMarkLeaseResp, error) {
	  if err := e.require(); err != nil {
		  return IssueMarkLeaseResp{}, err
	   }
	   from,
		   to, err := NormalizePair(req.From, req.To)
	  if err != nil {
		  return IssueMarkLeaseResp{}, err
	  }
	  purpose,
		   err := NormalizePurpose(req.Purpose)
	   if err != nil {
		  return IssueMarkLeaseResp{}, err
	  }
	   ttl, err := parseTTL(req.TTL, e.ttl)
	   if err != nil {
		  return IssueMarkLeaseResp{}, err
	   }
	  if idem := strings.TrimSpace(req.Idem); idem != "" {
		   if old, err := e.store.GetLeaseByIdem(ctx, idem); err == nil {
			  if old.From != from || old.To != to {
				  return IssueMarkLeaseResp{}, ErrIdemConflict
			  }
			  return toIssueResp(old), nil
		   }
	   }
	  unlock := e.keys.lock(PairKey(from, to))
	   defer unlock()
	   snap, err := e.readLive(ctx, from, to, req.PreferLive)
	   if err != nil {
		   return IssueMarkLeaseResp{}, ErrRateMissing
	  }
	   if _, err := ParsePositiveRate(snap.Rate); err != nil {
		   return IssueMarkLeaseResp{},
			   ErrRateMissing
	  }
	  now := e.now()
	  thresh := normalizeThreshold(req.ThresholdBPS)
	   if req.ThresholdBPS == 0 {
		  thresh = e.thresh
	  }
	  l := MarkLease{
		  ID: newLeaseID(),
		   From: from,
		  To: to,
		   FromUSD: snap.FromUSD,
		  ToUSD:   snap.ToUSD,
		  Rate: snap.Rate,
		   Source: snap.Source,
		  SourcePrint: ComputeSourcePrint(from, to, snap.Rate, snap.Source, snap.Generation),
		  PairPrint:   ComputePairPrint(snap),
		   Generation: snap.Generation,
		   FromGen:      snap.FromGen,
		   ToGen:        snap.ToGen,
		   ThresholdBPS: thresh,
		   Purpose: purpose,
		  State: LeaseIssued,
		  TTL:       Duration{ttl},
		  IssuedAt:  now,
		   ExpiresAt: now.Add(ttl),
		   Idem:      strings.TrimSpace(req.Idem),
		  Actor: strings.TrimSpace(req.Actor),
		  CreatedAt: now,
		   UpdatedAt: now,
	   }
	  if _, err := e.store.SaveLease(ctx, l); err != nil {
		   return IssueMarkLeaseResp{}, err
	   }
	   e.writeAudit(ctx, l.ID, LeaseEvIssue, "", LeaseIssued, firstNonEmpty(req.Actor, "system"), "issue mark lease")
	   if e.autoBnd {
		  if _, err := e.bindLocked(ctx, l, BindAssertReq{LeaseID: l.ID, From: from, To: to, Purpose: purpose, Actor: req.Actor}); err != nil {
			  got, _ := e.store.GetLease(ctx, l.ID)
			   return toIssueResp(got), err
		  }
		  got, _ := e.store.GetLease(ctx, l.ID)
		   return toIssueResp(got), nil
	  }
	   return toIssueResp(l), nil
  }

   func toIssueResp(l MarkLease) IssueMarkLeaseResp {
	  return IssueMarkLeaseResp{
		  LeaseID: l.ID,
		   From:    l.From, To: l.To, Rate: l.Rate,
		  FromUSD: l.FromUSD, ToUSD: l.ToUSD, Source: l.Source,
		   SourcePrint: l.SourcePrint, Generation: l.Generation,
		  ThresholdBPS: l.ThresholdBPS, State: l.State,
		   ExpiresAt: l.ExpiresAt, Purpose: l.Purpose,
	  }
  }

  func firstNonEmpty(ss ...string) string {
	   for _, s := range ss {
		  if strings.TrimSpace(s) != "" {
			   return strings.TrimSpace(s)
		   }
	   }
	   return ""
   }

   func (e *Engine) BindBeforePost(ctx context.Context, req BindAssertReq) (BindAssertResp, error) {
	  if err := e.require(); err != nil {
		  return BindAssertResp{}, err
	  }
	  id, err := requireLeaseID(req.LeaseID)
	  if err != nil {
		  return BindAssertResp{}, err
	   }
	   l, err := e.store.GetLease(ctx, id)
	   if err != nil {
		  return BindAssertResp{}, err
	   }
	   unlock := e.keys.lock(PairKey(l.From, l.To))
	  defer unlock()
	  l, err = e.store.GetLease(ctx, id)
	  if err != nil {
		   return BindAssertResp{}, err
	  }
	   return e.bindLocked(ctx, l, req)
  }

   func (e *Engine) bindLocked(ctx context.Context, l MarkLease, req BindAssertReq) (BindAssertResp, error) {
	  if proof, err := e.recheck(ctx, &l, req.From, req.To, req.Purpose, false); err != nil {
		   return BindAssertResp{LeaseID: l.ID, State: l.State}, err
	  } else if proof != nil {
		  return BindAssertResp{LeaseID: l.ID, State: l.State}, wrapDrift(proof.Kind, ErrFingerprintDrift)
	  }
	  if l.State == LeaseBound {
		  return BindAssertResp{LeaseID: l.ID, State: l.State, Bound: true, Rate: l.Rate, ExpiresAt: l.ExpiresAt}, nil
	  }
	  next, err := MustApply(l.State, LeaseEvBind)
	  if err != nil {
		   return BindAssertResp{}, err
	  }
	  from := l.State
	   l.State = next
	  l.BoundAt = stamp(e.now())
	   l.UpdatedAt = e.now()
	  l.PostRef = firstNonEmpty(req.PostRef, l.PostRef)
	   if _, err := e.store.SaveLease(ctx, l); err != nil {
		   return BindAssertResp{}, err
	   }
	   e.writeAudit(ctx, l.ID, LeaseEvBind, from, next, firstNonEmpty(req.Actor, "system"), "bind before post")
	   return BindAssertResp{LeaseID: l.ID, State: l.State, Bound: true, Rate: l.Rate, ExpiresAt: l.ExpiresAt}, nil
   }
