// 做什么：强制冷签的批次必须处于 sign_ready，份额够，摘要一致，且未过期。
// 未登记的单据直接放过；已登记但事实不足、存储失败或会话不对则拒绝。
// 不变量：拒绝必写审计；过期会话在拒绝前中止并释放绑定。

// Author: Bronson
// Date: 2021.03.08

package cerbind

import (
	"context"
	"errors"
	"strings"
)

// CheckBinding 是签播前的失败即关检查。
   func (e *Engine) CheckBinding(ctx context.Context, sub Subject) error {
	   if err := e.ready(); err != nil {
		  countGate("unavailable")
		   return err
	  }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	   return e.checkLocked(ctx, sub)
   }

  func (e *Engine) checkLocked(ctx context.Context, sub Subject) error {
	  batchID, err := e.resolveBatch(ctx, sub)
	  if err != nil {
		  countGate("deny")
		  e.writeAudit(ctx, "", sub.BatchID, "gate_deny", "", err.Error())
		  return err
	   }
	  if batchID == "" {
		   countGate("skip")
		  return nil
	  }
	  snap, ok, err := e.st.Snap(ctx, batchID)
	   if err != nil {
		  return e.deny(ctx, "", batchID, err)
	  }
	  if !ok {
		   return e.deny(ctx, "", batchID, ErrDenied)
	   }
	   ps, err := e.st.Policies(ctx)
	   if err != nil {
		  return e.deny(ctx, "", batchID, err)
	  }
	   dec, err := e.DecideSnap(ps, snap)
	   if err != nil {
		   return e.deny(ctx, "", batchID, err)
	   }
	  if !dec.Required {
		   e.writeAudit(ctx, "", batchID, "gate_allow", "", "exempt")
		  countGate("exempt")
		  return nil
	   }
	  covered, err := coversSign(snap.Lines, sub.Chain, sub.Dest, sub.Amount)
	   if err != nil {
		  return e.deny(ctx, "", batchID, err)
	  }
	   if !covered {
		  return e.deny(ctx, "", batchID, ErrDigest)
	   }
	  bind,
		  ok, err := e.st.ActiveByBatch(ctx, batchID)
	   if err != nil {
		  return e.deny(ctx, "", batchID, err)
	   }
	  if !ok {
		  return e.deny(ctx, "", batchID, ErrDenied)
	   }
	  ses, ok, err := e.st.Session(ctx, bind.SessionID)
	   if err != nil || !ok {
		   if err == nil {
			  err = ErrNotFound
		  }
		  return e.deny(ctx, bind.SessionID, batchID, err)
	  }
	  if !ses.Deadline.IsZero() && e.now().After(ses.Deadline) && ses.State.Live() {
		  if _, aerr := e.abortLocked(ctx, ses, e.timer, "timeout"); aerr != nil && !errors.Is(aerr, ErrState) {
			   return e.deny(ctx, ses.ID, batchID, aerr)
		   }
		  return e.deny(ctx, ses.ID, batchID, ErrTimeout)
	  }
	  if ses.State != StReady {
		  return e.deny(ctx, ses.ID, batchID, ErrDenied)
	   }
	  rcs, err := e.st.Receipts(ctx, ses.ID)
	  if err != nil {
		   return e.deny(ctx, ses.ID, batchID, err)
	   }
	   n := len(distinctActors(rcs))
	  need := ses.Threshold
	  if dec.Threshold > need {
		  need = dec.Threshold
	  }
	   if n < need {
		   return e.deny(ctx, ses.ID, batchID, ErrQuorum)
	   }
	   if bind.Digest != snap.Digest {
		   return e.deny(ctx, ses.ID, batchID, ErrDigest)
	   }
	  if batchID == ses.BatchID && ses.Digest != snap.Digest {
		  return e.deny(ctx, ses.ID, batchID, ErrDigest)
	  }
	   binds, err := e.st.BindingsBySession(ctx, ses.ID)
	   if err != nil {
		   return e.deny(ctx, ses.ID, batchID, err)
	   }
	  active := activeBinds(binds)
	  if ses.BindMode == ModeOne && len(active) != 1 {
		  return e.deny(ctx, ses.ID, batchID, ErrBindMode)
	  }
	  if len(active) == 0 {
		  return e.deny(ctx, ses.ID, batchID, ErrDenied)
	  }
	  e.writeAudit(ctx, ses.ID, batchID, "gate_allow", "", "sign_ready")
	  countGate("allow")
	   return nil
  }

   func (e *Engine) resolveBatch(ctx context.Context, sub Subject) (string, error) {
	  if id := strings.TrimSpace(sub.BatchID); id != "" {
		  got, err := batchIDOf(id)
		  if err != nil {
			  return "", err
		   }
		  return got, nil
	  }
	  for _, ref := range []string{sub.MovementID, sub.Idem} {
		  if batch, ok, err := e.lookup(ctx, ref); err != nil || ok {
			  return batch, err
		  }
	  }
	   for _, ref := range sub.ItemRefs {
		   if batch, ok, err := e.lookup(ctx, ref); err != nil || ok {
			  return batch, err
		  }
	   }
	  return "", nil
   }

   func (e *Engine) lookup(ctx context.Context, ref string) (string, bool, error) {
	   ref = strings.TrimSpace(ref)
	   if ref == "" {
		   return "", false, nil
	   }
	   return e.st.Alias(ctx, ref)
   }

   func (e *Engine) deny(ctx context.Context, sessionID, batchID string, cause error) error {
	   if cause == nil {
		   cause = ErrDenied
	   }
	   e.writeAudit(ctx, sessionID, batchID, "gate_deny", "", cause.Error())
	   countGate("deny")
	   if errors.Is(cause, ErrDenied) || errors.Is(cause, ErrQuorum) || errors.Is(cause, ErrDigest) ||
		   errors.Is(cause, ErrTimeout) || errors.Is(cause, ErrBindMode) || errors.Is(cause, ErrNotFound) ||
		   errors.Is(cause, ErrUnavailable) {
		   return cause
	   }
	   return cause
   }
