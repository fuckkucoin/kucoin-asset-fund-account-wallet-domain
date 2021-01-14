package withdraw

import (
	"context"
	"errors"
	"fmt"
	"time"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/ledger"
)

type StuckAction string

const (
	StuckWait StuckAction = "wait"
	StuckBump    StuckAction = "fee_bump"
	StuckRecover StuckAction = "recover"
	StuckFail    StuckAction = "fail"
)

type Stuck struct {
	TransferID string          `json:"transfer_id"`
	Status     BroadcastStatus `json:"status"`
	Age time.Duration `json:"age"`
	Action StuckAction `json:"action"`
	Reason string `json:"reason"`
}

// ClassifyStuck 对广播跟踪行分类动作。已确认只返回 wait；过期审查不等于放行。

   func ClassifyStuck(row Broadcast, now time.Time, ttl time.Duration) Stuck {
	  if ttl <= 0 {
		   ttl = 30 * time.Minute
	   }
	  age := now.Sub(row.UpdatedAt)
	  out := Stuck{TransferID: row.TransferID, Status: row.Status, Age: age, Action: StuckWait}
	  switch row.Status {
	   case BcConfirmed:
		   out.Reason = "already confirmed"
		  return out
	   case BcFailed:
		  out.Action = StuckFail
		  out.Reason = "already failed"
		   return out
	   case BcSigned:
		   if age > ttl {
			   out.Action = StuckRecover
			  out.Reason = "signed but not broadcast"
		  }
		  return out
	  case BcBroadcast, BcConfirming, BcReorged:
		   if age > ttl*2 {
			   out.Action = StuckRecover
			  out.Reason = "stale broadcast"
			  return out
		   }
		   if age > ttl {
			   out.Action = StuckBump
			   out.Reason = "pending too long"
			  return out
		   }
	  }
	  return out
  }

  func (s *Service) ScanStuck(now time.Time, ttl time.Duration) []Stuck {
	  if s == nil {
		  return nil
	   }
	   out := make([]Stuck, 0)
	  for _, row := range s.Broadcasts().All() {
		  st := ClassifyStuck(row, now, ttl)
		  if st.Action != StuckWait {
			   out = append(out, st)
		  }
	  }
	  return out
   }

// RecoverStuck 对广播跟踪的卡住单采取动作。
// 无 hash（signed / 从未广播）走 ledger Cancel 解冻并标记广播 Failed；
// 已有 live hash 拒绝自动解冻，留给 Observe/Confirm。
// fee_bump 只分类返回，不造链上替换交易。
   func (s *Service) RecoverStuck(trID string, now time.Time) (Stuck, error) {
	  if s == nil {
		   return Stuck{}, fmt.Errorf("withdraw service unavailable")
	   }
	  row, ok := s.Broadcasts().Get(trID)
	   if !ok {
		  return Stuck{}, fmt.Errorf("broadcast not tracked")
	  }
	  st := ClassifyStuck(row, now, 0)
	  if st.Action == StuckWait {
		  return st, fmt.Errorf("not stuck")
	   }
	  if row.Status == BcConfirmed {
		  return st, fmt.Errorf("already confirmed")
	  }
	   if st.Action == StuckBump {
		   return st, nil
	   }
	   if row.Status == BcFailed {
		  return st, nil
	  }
	  if err := RefuseAutoUnfreeze(row); err != nil {
		   s.noteLiveDeny(trID, row.TxHash, "recover")
		  return st, err
	   }
	   var fr FreezeState
	   user, asset := "", row.Asset
	  run, hasRun := s.runFor(trID)
	  if hasRun {
		   user = run.UserID
		   if run.Asset != "" {
			  asset = run.Asset
		   }
		  fr = run.Freeze
		  if fr == FreezeNone {
			  fr = freezeFromStep(run.Step)
		   }
	  }
	   if s.abuseHoldsUnfreeze(user, asset, fr, row) {
		  return st, ErrAbuseInFlight
	  }
	  // 相位已离开可取消窗口时，即使跟踪仍是 signed 空哈希，也不自动解冻。
	  if hasRun && !CanCancel(fr) {
		  return st, ErrFreezeUnlockDenied
	  }
	   if st.Action != StuckRecover && st.Action != StuckFail {
		  return st, nil
	  }
	  reason := st.Reason
	  if reason == "" {
		   reason = "stuck recover"
	  }
	  if err := s.Cancel(context.Background(), trID, reason); err != nil && !errors.Is(err, ledger.ErrNotFound) {
		  return st, err
	   }
	  // Cancel 已按 transfer 释 pending_withdraw；此处按 ref 再幂等释放，避免取消竞态残留标签。
	   if s.holds != nil {
		   if tr, err := s.led.Get(context.Background(), trID); err == nil && tr != nil {
			  _, _ = s.holds.ReleaseByRef(context.Background(), tr.UserID, holdcat.ReasonPendingWithdraw, trID)
		   }
	  }
	  if _, err := s.Broadcasts().Fail(trID, reason); err != nil {
		   return st, err
	   }
	  return st, nil
   }

// SweepStuck 扫描卡住广播并执行 RecoverStuck。
// 仅统计成功解冻/失败收尾（StuckRecover）；bump-only 与 live hash 拒绝不计。
// limit≤0 默认 200；硬上限 500。
   func (s *Service) SweepStuck(now time.Time, ttl time.Duration, limit int) (int, error) {
	   if s == nil {
		  return 0, nil
	   }
	   if limit <= 0 {
		  limit = 200
	  }
	   if limit > 500 {
		  limit = 500
	   }
	  stuck := s.ScanStuck(now, ttl)
	  n := 0
	  for i, st := range stuck {
		  if i >= limit {
			   break
		   }
		  got,
			   err := s.RecoverStuck(st.TransferID, now)
		  if err != nil {
			  continue
		  }
		  if got.Action == StuckRecover {
			  n++
		  }
	  }
	  return n, nil
   }
