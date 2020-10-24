// 广播前把单据交给 KYT。
// 作者: Blaise
// 日期: 2020.10.03
package ledger

import (
	"context"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/kyt"
	"account-assets.kucoin.net/funding/internal/metrics"
)

   func kytReview(tr *Transfer) bool {
	  if tr == nil {
		   return false
	   }
	  if hasFlag(tr.RiskFlags, "kyt_released") {
		   return false
	   }
	   if tr.KYTDecision == string(kyt.Review) {
		   return true
	   }
	  return hasFlag(tr.RiskFlags, "kyt_review")
   }

   func kytReleased(tr *Transfer) bool {
	   return tr != nil && hasFlag(tr.RiskFlags, "kyt_released")
   }

// screenWithdraw 在冻结之后、链上提交之前筛查出金。
// 未挂筛查器时直接拒绝。筛查出错同样拒绝。严禁在结果未知时接着签名。
// 放行则记下结论并接着。复核则打标、保持冻结，并返回 ErrKYTReview。执行体不得把复核当成失败去解冻。
// 拒绝则由调用方回滚冻结，不得提交。
// 已经人工放行的单据不再过筛，避免同一张单被再次卡住。
  func (s *Service) screenWithdraw(ctx context.Context, cur *Transfer) error {
	  if cur == nil || cur.Direction != WalletWithdraw {
		  return nil
	   }
	   if s.kyt == nil {
		   return ErrKYTUnavailable
	   }
	   if kytReleased(cur) {
		   return nil
	   }
	  to := s.DecryptAddress(cur)
	  d, reason, err := s.kyt.Screen(ctx, kyt.Request{
		   UserID: cur.UserID, MovementID: cur.ID, Chain: cur.Chain,
		  ToAddress: to, Asset: cur.Asset, Amount: cur.Amount.String(),
	   })
	   if err != nil {
		  metrics.KYTDecisions.WithLabelValues("error").Inc()
		   return fmt.Errorf("%w: %v", ErrKYTUnavailable, err)
	  }
	  cur.KYTDecision = string(d)
	   switch d {
	  case kyt.Allow:
		   metrics.KYTDecisions.WithLabelValues("allow").Inc()
		  return s.putFenced(ctx, cur)
	  case kyt.Review:
		  metrics.KYTDecisions.WithLabelValues("review").Inc()
		  cur.RiskFlags = appendFlag(cur.RiskFlags, "kyt_review")
		  if err := s.putFenced(ctx, cur); err != nil {
			  return err
		  }
		   _ = s.audit(ctx, cur, "kyt_review", map[string]any{"reason": reason})
		   return ErrKYTReview
	   default:
		  metrics.KYTDecisions.WithLabelValues("deny").Inc()
		   return fmt.Errorf("%w: %s", ErrKYTDenied, reason)
	  }
   }

// DecideKYT 由另一名操作员对复核中的出金作出放行或拒绝。
// 用户本人、空操作员、system 不得作此决定。
// 放行后打上已放行标记再进入签名与广播。拒绝则在同一把用户锁里回滚冻结。
// 单据上已有链上哈希时，严禁在这里解冻。
  func (s *Service) DecideKYT(ctx context.Context, id, actor, decision, reason string) (*Transfer, error) {
	   actor = strings.TrimSpace(actor)
	   decision = strings.ToLower(strings.TrimSpace(decision))
	   if actor == "" || strings.EqualFold(actor, "system") {
		  return nil, fmt.Errorf("%w: actor required", ErrMakerChecker)
	   }
	   if decision != "allow" && decision != "deny" {
		   return nil, fmt.Errorf("%w: decision must be allow or deny", ErrKYTNotPending)
	  }
	  head,
		  err := s.store.GetTransfer(ctx, id)
	  if err != nil {
		   return nil, err
	  }
	  if strings.EqualFold(actor, head.UserID) {
		  return nil, fmt.Errorf("%w: actor cannot be the user", ErrMakerChecker)
	   }
	  lease,
		  err := s.lock.Lock(ctx, UserLockKey(head.UserID), s.lockTTL)
	  if err != nil {
		  return nil, err
	   }
	  defer lease.Unlock()
	   ctx = withLease(ctx, lease)
	   ctx,
		  err = s.beginFence(ctx, id, lease)
	   if err != nil {
		  return nil, err
	  }
	  cur, err := s.getFenced(ctx, id)
	   if err != nil {
		   return nil, err
	  }
	  if cur.State == StateRolledBack || cur.State == StateCredited {
		  return cur, fmt.Errorf("%w", ErrKYTNotPending)
	   }
	  if cur.TxHash != "" && !IsInternalTxHash(cur.TxHash) {
		   return cur, ErrSubmittedLive
	   }
	   if kytReleased(cur) && decision == "allow" {
		   return s.progressLocked(ctx,
			   id, false, lease)
	   }
	  if !kytReview(cur) {
		   return cur, ErrKYTNotPending
	  }
	   if decision == "deny" {
		   if reason == "" {
			   reason = "kyt_ops_deny"
		  }
		   if err := s.store.InTx(ctx, func(ctx context.Context) error {
			   got, e2 := s.getFenced(ctx, id)
			   if e2 != nil {
				   return e2
			   }
			  _ = s.audit(ctx, got, "kyt_ops_deny", map[string]any{"actor": actor, "reason": reason})
			  return s.rollbackLocked(ctx, got, reason)
		  }); err != nil {
			   return cur, err
		   }
		  if s.OnBooks != nil {
			   s.OnBooks(ctx, cur.UserID)
		   }
		   return s.store.GetTransfer(ctx, id)
	  }
	  cur.KYTDecision = string(kyt.Allow)
	   cur.RiskFlags = dropFlag(cur.RiskFlags, "kyt_review")
	  cur.RiskFlags = appendFlag(cur.RiskFlags, "kyt_released")
	   if err := s.putFenced(ctx, cur); err != nil {
		   return cur, err
	   }
	  _ = s.audit(ctx, cur, "kyt_ops_allow", map[string]any{"actor": actor, "reason": reason})
	   metrics.KYTDecisions.WithLabelValues("ops_allow").Inc()
	  return s.progressLocked(ctx, id, false, lease)
  }
