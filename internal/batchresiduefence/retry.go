package batchresiduefence

import (
	"context"
	"strings"
	"github.com/google/uuid"
)

  func (e *Engine) RetryWithFence(ctx context.Context, req RetryWithFenceReq) (RetryWithFenceResp, error) {
	   if err := e.require(); err != nil {
		   return RetryWithFenceResp{}, err
	   }
	   batchID, err := requireBatchID(req.BatchID)
	   if err != nil {
		  return RetryWithFenceResp{}, err
	   }
	  itemIdem, err := requireItemIdem(req.ItemIdem)
	  if err != nil {
		  return RetryWithFenceResp{}, err
	  }
	   actor, err := requireActor(req.Actor)
	   if err != nil {
		  return RetryWithFenceResp{}, err
	  }
	   if strings.TrimSpace(req.FenceID) == "" {
		   e.audit(ctx, batchID, "", AuditRetryReject, "", "", actor, ReasonNoFence)
		   return RetryWithFenceResp{Allowed: false,
			  Reason: ReasonNoFence}, ErrFailClosed
	   }
	   view, verr := e.viewer.ViewItem(ctx, batchID, itemIdem)
	  if verr != nil {
		  return RetryWithFenceResp{}, verr
	   }
	  if err := forbidRetryPhase(view.Phase); err != nil {
		  reason := ReasonItemNotFailed
		   if view.Phase == PhaseSubmitted {
			   reason = ReasonItemSubmitted
		  }
		   if view.Phase == PhaseSkipped {
			  reason = ReasonItemSkipped
		   }
		  e.audit(ctx, batchID, req.FenceID, AuditRetryReject, string(view.Phase), "", actor, reason)
		  return RetryWithFenceResp{Allowed: false, Reason: reason}, err
	   }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	   if e.mutexAlive(ctx, batchID, MutexSubmit) {
		   e.audit(ctx, batchID, req.FenceID, AuditRetryReject, "", "", actor, ReasonSubmitRetryMutex)
		  return RetryWithFenceResp{Allowed: false, Reason: ReasonSubmitRetryMutex}, ErrMutexHeld
	  }
	   if err := e.AcquireMutex(ctx, batchID, MutexRetry, actor); err != nil {
		   return RetryWithFenceResp{Allowed: false, Reason: ReasonSubmitRetryMutex}, err
	  }
	  defer func() { _ = e.ReleaseMutex(ctx, batchID, actor) }()
	   f, ok, err := e.store.GetFence(ctx, strings.TrimSpace(req.FenceID))
	  if err != nil {
		   return RetryWithFenceResp{}, err
	   }
	   if !ok {
		   return RetryWithFenceResp{Allowed: false,
			   Reason: ReasonNoFence}, ErrFailClosed
	  }
	  if f.BatchID != batchID || f.ItemIdem != itemIdem {
		  return RetryWithFenceResp{Allowed: false, Reason: ReasonBatchMismatch}, ErrFence
	   }
	  if req.Generation != 0 && req.Generation != f.Generation {
		  return RetryWithFenceResp{Allowed: false, Reason: ReasonGenMismatch}, ErrGeneration
	   }
	  if f.State == FenceExpired || e.now().After(f.ExpireAt) {
		  if f.State != FenceExpired {
			  next, aerr := MustApplyFence(f.State, FenceEvExpire)
			  if aerr == nil {
				  f.State = next
				   _ = e.store.SaveFence(ctx, f)
			  }
		   }
		   return RetryWithFenceResp{Allowed: false, Fence: f, Reason: ReasonFenceExpired}, ErrFenceExpired
	  }
	   if f.State == FenceReleased {
		   return RetryWithFenceResp{Allowed: false,
			  Fence: f, Reason: ReasonFenceReleased}, ErrFenceReleased
	  }
	   if !CanRetryHold(f.State) {
		  return RetryWithFenceResp{Allowed: false, Fence: f, Reason: ReasonFailClosed}, ErrFenceState
	   }
	  half, halfOpen, herr := e.store.GetOpenHalf(ctx, batchID, itemIdem)
	   if herr != nil {
		   return RetryWithFenceResp{}, herr
	  }
	  resumeOnly := req.ResumeOnly || halfOpen
	  if halfOpen && !resumeOnly {
		  resumeOnly = true
	   }
	  attempt := strings.TrimSpace(req.AttemptID)
	  if attempt == "" {
		  attempt = uuid.NewString()
	   }
	  expectFP := FreezeFingerprint(batchID, itemIdem, view.UserID, view.Asset, view.Amount, f.OriginalFreezeRef)
	  if !MatchFingerprint(f.FreezeFingerprint, expectFP) {
		  return RetryWithFenceResp{Allowed: false, Fence: f, Reason: ReasonFingerprintDrift}, ErrFingerprint
	   }
	  slot,
		   hasSlot, serr := e.store.GetSlot(ctx, itemIdem)
	  if serr != nil {
		  return RetryWithFenceResp{},
			   serr
	   }
	   now := e.now()
	  if hasSlot && slot.Status == FreezeSlotActive {
		  if slot.FenceID != f.ID {
			   proof := BatchDupFreezeProof{
				   ID: e.newID(), BatchID: batchID, ItemIdem: itemIdem,
				   WinnerFenceID: slot.FenceID, LoserAttemptID: attempt,
				  LoserActor: actor, Reason: ReasonDupFreeze,
				   Fingerprint: f.FreezeFingerprint, CreatedAt: now,
			  }
			   if err := e.store.SaveProof(ctx, proof); err != nil {
				   return RetryWithFenceResp{}, err
			   }
			  e.audit(ctx, batchID, f.ID, AuditDupProof, "", "", actor, ReasonDupFreeze)
			  return RetryWithFenceResp{Allowed: false, Fence: f, DupProof: &proof, Reason: ReasonDupFreeze}, ErrDupFreeze
		   }
		  if !resumeOnly && halfOpen {
			  return RetryWithFenceResp{Allowed: false, Fence: f, Half: &half, Reason: ReasonSecondFullFreeze}, ErrSecondFullFreeze
		   }
		  if f.State == FenceIssued {
			   next, aerr := MustApplyFence(f.State, FenceEvFreeze)
			   if aerr != nil {
				   return RetryWithFenceResp{}, aerr
			   }
			   f.State = next
			   f.FrozenAt = &now
			  f.ResumeOnly = resumeOnly
			  if err := e.store.SaveFence(ctx, f); err != nil {
				  return RetryWithFenceResp{}, err
			  }
		  }
		   var halfPtr *HalfSubmitCase
		  if halfOpen {
			  nextH, aerr := MustApplyHalf(half.State, HalfEvResume)
			  if aerr != nil {
				  return RetryWithFenceResp{}, aerr
			  }
			   half.State = nextH
			   half.ResumeAt = &now
			  if err := e.store.SaveHalf(ctx, half); err != nil {
				  return RetryWithFenceResp{}, err
			   }
			   e.audit(ctx, batchID, f.ID, AuditHalfResume, string(HalfOpen), string(nextH), actor, "slot_held")
			  cp := half.Copy()
			  halfPtr = &cp
		  }
		  return RetryWithFenceResp{Allowed: true, Fence: f, Half: halfPtr, ResumeOnly: resumeOnly}, nil
	  }
	   if halfOpen && !resumeOnly {
		   return RetryWithFenceResp{Allowed: false, Fence: f, Half: &half, Reason: ReasonHalfCutNeedsResume}, ErrSecondFullFreeze
	   }
	  if f.State == FenceIssued {
		   next, aerr := MustApplyFence(f.State, FenceEvFreeze)
		   if aerr != nil {
			   return RetryWithFenceResp{}, aerr
		   }
		   from := f.State
		   f.State = next
		  f.FrozenAt = &now
		   f.ResumeOnly = resumeOnly
		   if err := e.store.SaveFence(ctx, f); err != nil {
			   return RetryWithFenceResp{}, err
		  }
		  e.audit(ctx, batchID, f.ID, AuditFreezeFence, string(from), string(next), actor, "")
	  }
	   if err := e.store.SaveSlot(ctx, FreezeSlot{
		  ItemIdem: itemIdem, BatchID: batchID, FenceID: f.ID,
		  Fingerprint: f.FreezeFingerprint,
		  Status:      FreezeSlotActive, HeldAt: now,
	  }); err != nil {
		   return RetryWithFenceResp{}, err
	  }
	  var halfPtr *HalfSubmitCase
	  if halfOpen {
		   nextH, aerr := MustApplyHalf(half.State, HalfEvResume)
		   if aerr != nil {
			  return RetryWithFenceResp{}, aerr
		  }
		  half.State = nextH
		   half.ResumeAt = &now
		   if err := e.store.SaveHalf(ctx, half); err != nil {
			   return RetryWithFenceResp{}, err
		  }
		   e.audit(ctx, batchID, f.ID, AuditHalfResume, string(HalfOpen), string(nextH), actor, "")
		   cp := half.Copy()
		   halfPtr = &cp
	   }
	   return RetryWithFenceResp{Allowed: true, Fence: f, Half: halfPtr, ResumeOnly: resumeOnly}, nil
  }
