package cerdecidegate

import (
	"context"
	"strings"
	"github.com/google/uuid"
)

// RecordPolicyFlip 记录策略包翻转并推进世代。
  func (e *Engine) RecordPolicyFlip(ctx context.Context, ev FlipEvent) (FlipEvent, error) {
	   if err := e.ok(); err != nil {
		   return FlipEvent{}, err
	  }
	  if ev.EventID == "" {
		  ev.EventID = uuid.NewString()
	  }
	  if ev.Scope == "" {
		  if ev.PolicyID != "" {
			   ev.Scope = PolicyScope(ev.PolicyID)
		  } else {
			  ev.Scope = "*"
		   }
	  }
	  if ev.At.IsZero() {
		   ev.At = e.now()
	  }
	  epoch, err := e.store.NextGeneration(ctx, ev.Scope)
	  if err != nil {
		  return FlipEvent{}, err
	  }
	   ev.Epoch = epoch
	   if err := e.store.SaveFlip(ctx, ev); err != nil {
		   return FlipEvent{}, err
	  }
	  return ev, nil
  }

// OnFlip 消费一条翻转：吊销匹配的在途租约。
   func (e *Engine) OnFlip(ctx context.Context, ev FlipEvent) (int, error) {
	  if err := e.ok(); err != nil {
		  return 0, err
	   }
	   if ev.EventID == "" {
		  var err error
		   ev, err = e.RecordPolicyFlip(ctx, ev)
		   if err != nil {
			   return 0, err
		   }
	  }
	  n, err := e.revokeMatching(ctx, ev)
	   if err != nil {
		   return n, err
	  }
	  if ev.EventID != "" {
		   _ = e.store.MarkFlipDone(ctx, ev.EventID)
	   }
	  if e.metric != nil {
		  e.metric.AddRevoked(n)
		  e.metric.IncFlip()
	   }
	  return n, nil
   }

   func (e *Engine) revokeMatching(ctx context.Context, ev FlipEvent) (int, error) {
	  var rows []DecideLease
	   var err error
	   switch {
	   case ev.PolicyID != "":
		   rows, err = e.store.ListOpenByPolicy(ctx, ev.PolicyID)
	  case strings.HasPrefix(ev.Scope, "batch:"):
		   rows, err = e.store.ListOpenByBatch(ctx, strings.TrimPrefix(ev.Scope, "batch:"))
	   default:
		   rows, err = e.store.ListOpenByScope(ctx, ev.Scope)
	  }
	  if err != nil {
		   return 0, err
	   }
	   now := e.now()
	   n := 0
	  why := flipWhy(ev)
	   kind := flipProofKind(ev)
	  for _, row := range rows {
		  if !row.State.Open() {
			  continue
		  }
		   if ev.Kind == FlipDestRisk && RiskRank(ev.NewRisk) <= RiskRank(row.DestRisk) {
			  continue
		  }
		  if err := e.store.CASState(ctx, row.LeaseID, row.State, LeaseRevoked, DecideLease{
			  RevokedAt: now,
			  UpdatedAt: now, RevokeWhy: why,
		  }); err != nil {
			   continue
		  }
		   _ = e.dropProof(ctx, CerDriftProof{
			  ProofID: uuid.NewString(), LeaseID: row.LeaseID, BatchID: row.BatchID,
			   Kind: kind, QuotedPolicy: row.PolicyID, LivePolicy: ev.PolicyID,
			   QuotedThresh: row.Threshold, LiveThresh: ev.NewThresh,
			   QuotedShares: row.Shares, LiveShares: ev.NewShares,
			  QuotedRisk: row.DestRisk,
			  LiveRisk:   ev.NewRisk,
			   QuotedForce: row.Required,
			  LiveForce:   ev.NewForce,
			  QuotedGen:   row.Generation, LiveGen: ev.Epoch,
			  Detail: why, Actor: "cer-decide-flip", At: now, CreatedAt: now,
		   })
		  _ = e.store.AppendAudit(ctx, DecideAudit{
			   AuditID: uuid.NewString(), LeaseID: row.LeaseID, BatchID: row.BatchID,
			  Action: "flip_revoke", From: string(row.State), To: string(LeaseRevoked),
			  Actor: "cer-decide-flip", Detail: why, At: now,
		  })
		   n++
	  }
	   return n, nil
  }

   func flipWhy(ev FlipEvent) string {
	   switch ev.Kind {
	   case FlipThreshold:
		  return "policy_threshold_flip"
	  case FlipShares:
		   return "policy_shares_flip"
	   case FlipForceCold:
		   return "policy_force_cold_flip"
	  case FlipDestRisk:
		  return "dest_risk_raise"
	  case FlipGeneration:
		  return "generation_bump"
	  default:
		   return "policy_pack_flip"
	   }
   }

  func flipProofKind(ev FlipEvent) ProofKind {
	   switch ev.Kind {
	   case FlipThreshold:
		   return ProofThreshDrift
	   case FlipShares:
		   return ProofSharesDrift
	   case FlipForceCold:
		  return ProofForceFlip
	   case FlipDestRisk:
		  return ProofRiskRaise
	   case FlipGeneration:
		   return ProofGenStale
	  default:
		   return ProofPolicyDrift
	   }
   }

// RaiseDestRisk 目的地风险档抬升：世代+1 并吊销旧租约。

   func (e *Engine) RaiseDestRisk(ctx context.Context, batchID, oldRisk, newRisk, policyID string) (int, error) {
	  if RiskRank(newRisk) <= RiskRank(oldRisk) {
		  return 0, nil
	   }
	   return e.OnFlip(ctx, FlipEvent{
		  Scope: "batch:" + strings.TrimSpace(batchID), Kind: FlipDestRisk,
		   PolicyID: policyID,
		  OldRisk:  oldRisk, NewRisk: newRisk, At: e.now(),
	   })
  }

// FlipPolicyFields Decide 后改 Threshold/Shares/ForceCold。
   func (e *Engine) FlipPolicyFields(ctx context.Context, policyID string, kind FlipKind, oldT, newT, oldS, newS int, oldF, newF bool) (int, error) {
	  return e.OnFlip(ctx, FlipEvent{
		  Kind: kind, PolicyID: policyID, Scope: PolicyScope(policyID),
		   OldThresh: oldT, NewThresh: newT, OldShares: oldS, NewShares: newS,
		   OldForce: oldF, NewForce: newF, At: e.now(),
	  })
  }
