package fiataddgate

import (
	"context"
	"errors"
	"fmt"
	"account-assets.kucoin.net/funding/internal/fiatadd"
)

// Reconciler 只读对账器。

type Reconciler struct {
	book    *fiatadd.Book
	forks   ForkCaseStore
	effects EffectStore
	ledger TransferReader
}

// NewReconciler 构造只读对账器。

  func NewReconciler(book *fiatadd.Book, forks ForkCaseStore, effects EffectStore, ledger TransferReader) *Reconciler {
	   return &Reconciler{book: book, forks: forks, effects: effects, ledger: ledger}
  }

// Reconcile 对单个 Session 执行三方核对，返回一致性结果。
// 仅读，不改写任何行。
   func (r *Reconciler) Reconcile(ctx context.Context, sessionID string) (ReconcileResult, error) {
	  if r == nil || r.book == nil {
		   return ReconcileResult{}, fmt.Errorf("reconciler: nil")
	   }
	   sess, err := r.book.Get(ctx, "", sessionID)
	  if err != nil {
		   return ReconcileResult{}, fmt.Errorf("reconciler: load session: %w", err)
	   }
	   res := ReconcileResult{
		   SessionID: sess.ID,
		  SessionStatus: sess.Status,
		  SessionCreditID: sess.CreditID,
		   ConfirmGen: sess.ConfirmGen,
	   }
	  // 检查分叉案例
	   if fc, err := r.forks.GetOpenCase(ctx, sessionID); err == nil {
		  res.ForkCaseOpen = true
		  res.ForkCaseID = fc.CaseID
	   }
	   // 检查 CreditEffect
	   if effect, err := r.effects.GetBySessionGen(ctx, sessionID, sess.ConfirmGen); err == nil {
		   res.EffectFound = true
		   res.EffectID = effect.EffectID
		   if res.SessionCreditID == "" {
			   res.SessionCreditID = effect.CreditID
		  }
	   }
	  // 检查 ledger Transfer
	   if r.ledger != nil && sess.CreditID != "" {
		  snap, err := r.ledger.GetByID(ctx, sess.CreditID)
		   if err == nil && snap.ID != "" {
			   res.LedgerCreditFound = true
			  res.LedgerCreditID = snap.ID
			  res.LedgerCreditAmount = snap.Amount
		  }
	  }
	   // 一致性判断
	  res.Consistent, res.Reason = r.checkConsistency(res, sess)
	   return res, nil
  }

  func (r *Reconciler) checkConsistency(res ReconcileResult, sess fiatadd.Session) (bool, string) {
	  if res.ForkCaseOpen {
		   return false, fmt.Sprintf("open fork case %s exists", res.ForkCaseID)
	  }
	   switch sess.Status {
	   case "credited":
		  if sess.CreditID == "" {
			   return false, "session credited but credit_id empty"
		  }
		   if r.ledger != nil && !res.LedgerCreditFound {
			   return false, fmt.Sprintf("session credited credit_id=%s but ledger transfer not found", sess.CreditID)
		   }
		   return true, ""
	  case "pending":
		   if res.LedgerCreditFound {
			   return false, fmt.Sprintf("session pending but ledger credit %s found", res.LedgerCreditID)
		   }
		  return true, ""
	  case "confirming":
		   return true, "session in confirming; transient state"
	  case "forked":
		  return false, "session in forked state; requires ops intervention"
	   default:
		  return true, ""
	   }
   }

// ReconcileBatch 批量对账，返回所有不一致的 Session 结果。

   func (r *Reconciler) ReconcileBatch(ctx context.Context, sessionIDs []string) ([]ReconcileResult, error) {
	  var inconsistent []ReconcileResult
	   for _, id := range sessionIDs {
		   res, err := r.Reconcile(ctx, id)
		  if err != nil {
			  if !errors.Is(err, fiatadd.ErrGone) {
				   inconsistent = append(inconsistent,
					  ReconcileResult{SessionID: id, Reason: err.Error()})
			  }
			  continue
		   }
		  if !res.Consistent {
			  inconsistent = append(inconsistent, res)
		   }
	   }
	   return inconsistent, nil
  }
