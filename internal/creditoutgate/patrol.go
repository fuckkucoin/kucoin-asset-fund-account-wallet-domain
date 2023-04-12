package creditoutgate

import (
	"fmt"
)

// Patrol 执行一次双贷记巡检，返回巡检结果。
// limit <= 0 时默认扫描 64 条。
  func (s *Service) Patrol(limit int) PatrolResult {
	  if s == nil {
		  return PatrolResult{}
	  }
	   if limit <= 0 {
		   limit = 64
	  }
	   // 持锁收集 delivered 快照。
	   s.mu.Lock()
	  var deliveredLeases []CreditDeliverLease
	  for _, l := range s.leases {
		  if l.State == LeaseDelivered {
			  deliveredLeases = append(deliveredLeases,
				   *l)
		  }
	   }
	  proofSnapshot := make(map[string]CreditProof, len(s.proofs))
	  for k, p := range s.proofs {
		   proofSnapshot[k] = *p
	  }
	  s.mu.Unlock()
	   if len(deliveredLeases) > limit {
		  deliveredLeases = deliveredLeases[:limit]
	   }
	   result := PatrolResult{Scanned: len(deliveredLeases)}
	   // 检测逻辑：外部 CreditOutbox。
	  if s.outbox == nil {
		  return result
	   }
	   var foundCases []CreditFenceCase
	   for _, lease := range deliveredLeases {
		   extEpoch, extToken, _, extDelivered, extOK := s.outbox.CreditByKey(lease.CreditKey)
		   if !extOK {
			   continue
		   }
		  proof, hasProof := proofSnapshot[lease.CreditKey]
		  if !hasProof {
			  continue
		  }
		  // 双贷记场景：外部已贷记但 epoch/token 与本包投递证明不一致。
		   if extDelivered && (extEpoch != proof.Epoch || extToken != lease.Token) {
			  s.mu.Lock()
			  // 检查是否已有相同 key 的 dual_credit 证明（避免重复追加）。
			   alreadyReported := false
			  for _, c := range s.casesByKey[lease.CreditKey] {
				   if c.Kind == CaseDualCredit {
					  alreadyReported = true
					   break
				  }
			   }
			   if !alreadyReported {
				   bg,
					   _ := s.gens[lease.BoardKey]
				   var currentGen int64
				  if bg != nil {
					  currentGen = bg.WorldGen
				   }
				  now := s.now()
				   caseID := s.genID("cfc")
				  c := &CreditFenceCase{
					  ID:        caseID,
					  Kind:      CaseDualCredit,
					   CreditKey: lease.CreditKey,
					   LeaseID:         lease.ID,
					   BoardKey:        lease.BoardKey,
					   BindWorldGen:    lease.BindWorldGen,
					  CurrentWorldGen: currentGen,
					  Epoch:           proof.Epoch,
					   Token:           lease.Token,
					  Evidence: truncateEvidence(fmt.Sprintf(
						  `{"local_epoch":%d,"ext_epoch":%d,"ext_token_match":%v,"amount":%q}`,
						   proof.Epoch, extEpoch, extToken == lease.Token, lease.Amount,
					   )),
					  DetectedAt: now,
				   }
				   s.appendCaseLocked(c)
				   foundCases = append(foundCases, *c)
			  }
			  s.mu.Unlock()
			  result.Detected++
		  }
	   }
	   result.Cases = foundCases
	  return result
  }

// PatrolAllBoards 对每个已知板卡执行一次护栏状态健康检查：
// 若板卡关闭但仍有 enqueued 租约（世代漂移漏吊销），追加 CaseGenDrift 证明。
// 这是一个安全网巡检，正常情况下 FenceFlip 应已吊销全部在途租约。

  func (s *Service) PatrolAllBoards() []CreditFenceCase {
	  if s == nil {
		   return nil
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  now := s.now()
	  var found []CreditFenceCase
	   for boardKey, bg := range s.gens {
		   if !bg.Closed {
			  continue
		   }
		   for _, lease := range s.leases {
			  if lease.BoardKey != boardKey {
				  continue
			   }
			  if lease.State != LeaseEnqueued && lease.State != LeaseDelivering {
				   continue
			  }
			   // 发现漏网租约。
			   caseID := s.genID("cfc")
			  c := &CreditFenceCase{
				  ID:   caseID,
				   Kind: CaseGenDrift,
				   CreditKey: lease.CreditKey,
				  LeaseID:  lease.ID,
				  BoardKey: boardKey,
				   BindWorldGen:    lease.BindWorldGen,
				   CurrentWorldGen: bg.WorldGen,
				   Epoch: lease.Epoch,
				   Token: lease.Token,
				   Evidence: truncateEvidence(fmt.Sprintf(
					   `{"board_closed":true,"lease_state":%q,"bind_world_gen":%d,"current_world_gen":%d}`,
					  string(lease.State), lease.BindWorldGen, bg.WorldGen,
				  )),
				   DetectedAt: now,
			   }
			  s.appendCaseLocked(c)
			   // 补充吊销（安全网）。
			   revokedAt := now
			  lease.State = LeaseRevoked
			   lease.RevokedAt = &revokedAt
			  lease.RevokeReason = "patrol_safety_net_revoke"
			   found = append(found, *c)
		  }
	   }
	   return found
   }
