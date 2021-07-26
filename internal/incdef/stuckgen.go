package incdef

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/stucksettle"
)

// bindStuckGate 为 Service 绑定 stucksettle Gate（可在 New 后调用）。

   func (s *Service) bindStuckGate(gate *stucksettle.Gate) *Service {
	   if s != nil {
		   s.mu.Lock()
		  s.stuckGate = gate
		   s.mu.Unlock()
	  }
	  return s
  }

// BindStuckGate 公开挂载接口（由 register/main 调用）。
  func (s *Service) BindStuckGate(gate *stucksettle.Gate) *Service {
	   return s.bindStuckGate(gate)
   }

// GuardedRecoverStuck 带世代闸保护的回收执行。
// 取代裸 s.RecoverStuck(id)；通过 Gate 完成互斥检查、世代获取、证明固化。
// isHoldState 由调用方（AdvanceStuck）传入当前 StuckCase 是否为 Hold 态。
   func (s *Service) GuardedRecoverStuck(trID string, isHoldState bool) (StuckRec, error) {
	  if s == nil {
		  return StuckRec{}, ErrUnavailable
	  }
	  if s.stuckGate == nil {
		   // 无 Gate 时退化为原有路径
		   return s.RecoverStuck(trID)
	   }
	   id := strings.TrimSpace(trID)
	   if id == "" {
		   return StuckRec{}, ErrStuck
	  }
	  nodeID := "incdef:" + id
	   proof, err := s.stuckGate.GuardedRecover(id, nodeID, isHoldState)
	   if err != nil {
		   switch err {
		  case stucksettle.ErrDupBlocked:
			  s.Raise(Alert{Level: "warn", Kind: "stuck_dup_blocked", Subject: id,
				  Body: "duplicate recover blocked by stucksettle gate"})
			  return StuckRec{TransferID: id, Reason: "dup_blocked"}, ErrStuck
		  case stucksettle.ErrAdvanceLocked:
			  return StuckRec{TransferID: id, Reason: "advance_locked"}, ErrStuck
		  case stucksettle.ErrHoldBypass:
			  return StuckRec{TransferID: id, Reason: "hold_bypass"}, ErrStuck
		   default:
			   return StuckRec{TransferID: id, Reason: err.Error()}, ErrStuck
		  }
	   }
	   action := ""
	   if proof != nil {
		  action = proof.Action
	   }
	   rec := StuckRec{TransferID: id, Action: action}
	  s.mu.Lock()
	   s.appendLocked("stuck_recover_gated", "ops-inc", id, action)
	  s.mu.Unlock()
	  return rec, nil
  }

// AdvanceStuckGated AdvanceStuck 的世代闸增强版。
// 在进入 Retry/Recover 路径前先持 AdvanceLock；完成后释放。
// isHoldState 从当前 StuckCase 读取。
   func (s *Service) AdvanceStuckGated(trID string) (StuckCase, error) {
	   if s == nil {
		   return StuckCase{}, ErrUnavailable
	   }
	  if s.HaltedKilled() {
		  return StuckCase{}, ErrKilled
	  }
	   id := strings.TrimSpace(trID)
	  s.mu.Lock()
	   cur, ok := s.stuck[id]
	  if !ok {
		  s.mu.Unlock()
		   return StuckCase{}, ErrStuck
	   }
	  if cur.State == StuckRecovered || cur.State == StuckFailed {
		  s.mu.Unlock()
		  return cur, nil
	  }
	  isHold := cur.State == StuckHold
	   s.mu.Unlock()
	  // 持 AdvanceLock
	  if s.stuckGate != nil {
		  if err := s.stuckGate.HoldAdvance(id); err != nil {
			  return StuckCase{}, ErrStuck
		   }
		   defer s.stuckGate.ReleaseAdvance(id)
	  }
	   s.mu.Lock()
	   cur = s.stuck[id]
	   if cur.State == StuckRecovered || cur.State == StuckFailed {
		   s.mu.Unlock()
		   return cur, nil
	  }
	   cur.Tries++
	  cur.At = s.now()
	  if cur.Tries >= maxStuckTries {
		   cur.State = StuckHold
		   s.stuck[id] = cur
		   s.appendLocked("stuck_hold", "ops-inc", id, cur.Reason)
		   s.mu.Unlock()
		  return cur, ErrStuck
	   }
	   cur.State = StuckRetry
	  s.stuck[id] = cur
	   s.mu.Unlock()
	   // 带世代闸的回收
	  rec, err := s.GuardedRecoverStuck(id, isHold)
	  s.mu.Lock()
	  cur = s.stuck[id]
	   if err != nil {
		  cur.Reason = rec.Reason
		  if cur.Tries >= maxStuckTries {
			   cur.State = StuckFailed
		  }
		   s.stuck[id] = cur
		  s.mu.Unlock()
		   return cur, err
	   }
	   // 验证 EffectProof 存在后才标记 Recovered
	  if s.stuckGate != nil {
		   if _, pErr := s.stuckGate.GetEffectProof(id); pErr != nil {
			  // 证明缺失：不标 Recovered
			   cur.Reason = "proof_missing"
			  s.stuck[id] = cur
			   s.mu.Unlock()
			  return cur, ErrStuck
		   }
	   }
	   cur.State = StuckRecovered
	   cur.Action = rec.Action
	   s.stuck[id] = cur
	   s.mu.Unlock()
	  return cur, nil
  }

// StuckGenStatus 查询 transfer 的回收世代状态（供运维 API 只读）。

  func (s *Service) StuckGenStatus(trID string) (*stucksettle.StuckGeneration, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	   if s.stuckGate == nil {
		  return nil, ErrUnavailable
	  }
	   return s.stuckGate.GenStatus(strings.TrimSpace(trID))
   }

// StuckDupCases 查询 transfer 的双结案件列表（供运维 API 只读）。

  func (s *Service) StuckDupCases(trID string) ([]stucksettle.DupRecoverCase, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	  }
	  if s.stuckGate == nil {
		   return nil, ErrUnavailable
	   }
	  return s.stuckGate.DupCasesByTransfer(strings.TrimSpace(trID))
   }

// recoveryNodeKey 生成节点幂等键（防止同进程内 re-entrancy）。
// 形态：incdef:{transfer_id}:try{NN}，供 Gate.AcquireLease holderKey 使用。

  func recoveryNodeKey(trID string, tries int) string {
	   pad := func(n int) string {
		   if n < 10 {
			   return "0" + string(rune('0'+n))
		  }
		   return string(rune('0'+n/10)) + string(rune('0'+n%10))
	  }
	  return "incdef:" + trID + ":try" + pad(tries)
  }
