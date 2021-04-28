package creditoutgate

import (
	"fmt"
)

// Deliver 复验板卡世代并完成投递。
// creditKey 须与 Enqueue 时一致。
// epoch/token 来自调用方持有的 fundsafety CreditOutbox 令牌。
// 返回值：
// - DeliverResult.Replay=true：幂等成功，不产生新 CreditProof；
// - DeliverResult.Proof 非空：本次新 Deliver 成功，已写入证明；
// - error 非 nil：拒绝（世代漂移 / 租约吊销 / epoch/token 不符 / 板卡关闭）。
  func (s *Service) Deliver(creditKey string, epoch int64, token string) (DeliverResult, error) {
	  if s == nil {
		  return DeliverResult{},
			   ErrUnavailable
	  }
	   creditKey = normKey(creditKey)
	   if creditKey == "" {
		  return DeliverResult{}, ErrCreditKey
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   lease, ok := s.leases[creditKey]
	  if !ok {
		   return DeliverResult{}, ErrLeaseNotFound
	  }
	   now := s.now()
	   // 幂等路径：已完成投递。
	   if lease.State == LeaseDelivered {
		  proof := s.proofs[creditKey]
		  if proof != nil {
			  p := *proof
			  return DeliverResult{Proof: &p, Replay: true}, nil
		  }
		  return DeliverResult{Replay: true}, nil
	  }
	   // 已吊销路径：追加证明并拒绝。
	   if lease.State == LeaseRevoked {
		   bg, _ := s.gens[lease.BoardKey]
		  var currentGen int64
		  if bg != nil {
			  currentGen = bg.WorldGen
		  }
		   caseID := s.genID("cfc")
		   c := &CreditFenceCase{
			  ID:        caseID,
			   Kind:      CaseFenceClose,
			   CreditKey: creditKey,
			  LeaseID:      lease.ID,
			   BoardKey:     lease.BoardKey,
			  BindWorldGen: lease.BindWorldGen,
			  CurrentWorldGen: currentGen,
			   Epoch:           epoch,
			   Token: token,
			  Evidence: truncateEvidence(fmt.Sprintf(
				   `{"revoke_reason":%q,"bind_world_gen":%d,"current_world_gen":%d,"epoch":%d}`,
				   lease.RevokeReason, lease.BindWorldGen, currentGen, epoch,
			  )),
			   DetectedAt: now,
		  }
		  s.appendCaseLocked(c)
		   return DeliverResult{Revoked: true}, ErrLeaseRevoked
	  }
	   // 校验 epoch/token。
	   if lease.Epoch != epoch || lease.Token == "" || lease.Token != token {
		   bg, _ := s.gens[lease.BoardKey]
		  var currentGen int64
		  if bg != nil {
			  currentGen = bg.WorldGen
		   }
		   caseID := s.genID("cfc")
		  c := &CreditFenceCase{
			   ID:   caseID,
			  Kind: CaseStaleReplay,
			  CreditKey: creditKey,
			  LeaseID:         lease.ID,
			   BoardKey:        lease.BoardKey,
			  BindWorldGen:    lease.BindWorldGen,
			   CurrentWorldGen: currentGen,
			   Epoch:           epoch,
			   Token:           token,
			  Evidence: truncateEvidence(fmt.Sprintf(
				  `{"expected_epoch":%d,"got_epoch":%d,"token_match":%v}`,
				  lease.Epoch, epoch, lease.Token == token,
			  )),
			   DetectedAt: now,
		  }
		  s.appendCaseLocked(c)
		  return DeliverResult{}, ErrEpochToken
	  }
	   // 复验板卡世代。
	  bg, bgOK := s.gens[lease.BoardKey]
	   if !bgOK {
		  // 板卡世代不存在（理论上不应发生，Enqueue 时已初始化）。
		   return DeliverResult{}, ErrBoardKey
	   }
	   // 板卡已关闭或世代已漂移：拒绝并追加证明。
	   genDrifted := (bg.WorldGen != lease.BindWorldGen || bg.LaneGen != lease.BindLaneGen)
	  if bg.Closed || genDrifted {
		   kind := CaseGenDrift
		   if bg.Closed {
			  kind = CaseFenceClose
		   }
		  caseID := s.genID("cfc")
		  c := &CreditFenceCase{
			  ID:   caseID,
			   Kind: kind,
			   CreditKey: creditKey,
			   LeaseID:         lease.ID,
			   BoardKey:        lease.BoardKey,
			   BindWorldGen:    lease.BindWorldGen,
			  CurrentWorldGen: bg.WorldGen,
			   Epoch:           epoch,
			  Token: token,
			  Evidence: truncateEvidence(fmt.Sprintf(
				   `{"bind_world_gen":%d,"current_world_gen":%d,"bind_lane_gen":%d,"current_lane_gen":%d,"closed":%v}`,
				   lease.BindWorldGen,
				  bg.WorldGen, lease.BindLaneGen, bg.LaneGen, bg.Closed,
			  )),
			  DetectedAt: now,
		   }
		   s.appendCaseLocked(c)
		  return DeliverResult{}, ErrGenDrift
	  }
	   // 检查外部 CreditOutbox 是否已投递（sentAt 非空）但 epoch 不符（护栏外漏）。
	  if s.outbox != nil {
		  _,
			  extToken, _, extDelivered, extOK := s.outbox.CreditByKey(creditKey)
		  if extOK && extDelivered && extToken != token {
			  // 护栏关闭后外部已贷记，但本包租约尚未完成 —— 双贷记夹缝。
			   caseID := s.genID("cfc")
			  c := &CreditFenceCase{
				   ID: caseID,
				   Kind: CaseDualCredit,
				  CreditKey: creditKey,
				  LeaseID: lease.ID,
				  BoardKey: lease.BoardKey,
				  BindWorldGen: lease.BindWorldGen,
				   CurrentWorldGen: bg.WorldGen,
				   Epoch:           epoch,
				   Token:           token,
				  Evidence: truncateEvidence(fmt.Sprintf(
					   `{"ext_delivered":true,"ext_token_match":false,"local_epoch":%d}`,
					  epoch,
				   )),
				   DetectedAt: now,
			   }
			  s.appendCaseLocked(c)
			  return DeliverResult{}, ErrDualCredit
		  }
	   }
	  // 全部校验通过：执行 Deliver。
	  delivering := now
	  lease.State = LeaseDelivering
	  lease.DeliveringAt = &delivering
	  // 写投递证明。
	  proofID := s.genID("cpf")
	  proof := &CreditProof{
		  ID: proofID,
		  CreditKey: creditKey,
		  LeaseID: lease.ID,
		  BoardKey: lease.BoardKey,
		   WorldGen: lease.BindWorldGen,
		   LaneGen:  lease.BindLaneGen,
		   Epoch:    epoch,
		   Amount:   lease.Amount,
		   ProvedAt: now,
	   }
	  s.proofs[creditKey] = proof
	   // 租约转 delivered。
	   delivered := now
	   lease.State = LeaseDelivered
	  lease.DeliveredAt = &delivered
	   p := *proof
	   return DeliverResult{Proof: &p, Replay: false}, nil
  }

// DeliverPending 批量投递所有 enqueued 状态的租约。
// limit <= 0 时默认处理 32 条。
// 返回批量结果摘要。

  func (s *Service) DeliverPending(limit int) BatchDeliverResult {
	  if s == nil {
		  return BatchDeliverResult{}
	   }
	  if limit <= 0 {
		  limit = 32
	   }
	  // 不持锁收集 pending 列表（快照）。
	  s.mu.Lock()
	   var pending []CreditDeliverLease
	   for _, l := range s.leases {
		  if l.State == LeaseEnqueued {
			  pending = append(pending, *l)
		  }
	  }
	  s.mu.Unlock()
	  if len(pending) > limit {
		  pending = pending[:limit]
	  }
	  out := BatchDeliverResult{Requested: len(pending)}
	  for _, l := range pending {
		  res, err := s.Deliver(l.CreditKey, l.Epoch, l.Token)
		   if err != nil {
			  switch err {
			  case ErrLeaseRevoked:
				  out.Revoked++
			   case ErrGenDrift:
				   out.GenDrift++
			   case ErrEpochToken:
				  out.EpochMismatch++
			   default:
				   out.Failed++
			   }
			   continue
		  }
		   if res.Replay {
			  out.Replay++
		   } else {
			   out.Delivered++
		   }
	   }
	  return out
   }

// BatchDeliverResult 批量投递结果摘要。
type BatchDeliverResult struct {
	Requested int `json:"requested"`
	Delivered int `json:"delivered"`
	Replay  int `json:"replay"`
	Revoked int `json:"revoked"`
	GenDrift int `json:"gen_drift"`
	EpochMismatch int `json:"epoch_mismatch"`
	Failed        int `json:"failed"`
}
