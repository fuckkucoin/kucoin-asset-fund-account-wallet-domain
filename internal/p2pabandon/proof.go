package p2pabandon

import (
	"strings"
	"time"
)

// ProofPatrolResult 单次收口证明巡检结果。
type ProofPatrolResult struct {
	Inspected int
	TamperFound int
	TamperPlanIDs []string
	Errors []string
}

// PatrolAllProofs 巡检所有 CloseProof 的哈希完整性。
// 发现篡改时更新 TamperDetected 字段并记录到结果中（不自动回滚，需人工处理）。
   func (s *Service) PatrolAllProofs() ProofPatrolResult {
	   if s == nil {
		  return ProofPatrolResult{}
	   }
	  proofs := s.ListAllProofs()
	  result := ProofPatrolResult{}
	   for _, proof := range proofs {
		  result.Inspected++
		   _, err := s.PatrolProof(proof.PlanID)
		  if err == ErrProofTamper {
			  result.TamperFound++
			  result.TamperPlanIDs = append(result.TamperPlanIDs, proof.PlanID)
		  } else if err != nil && err != ErrProof {
			   result.Errors = append(result.Errors, "plan "+proof.PlanID+": "+err.Error())
		   }
	   }
	   return result
   }

// GenerateConflictResolutionProof desk 仲裁后生成冲突解决证明。
// 适用场景：AbandonPlan 处于 conflict 状态，desk 决定其终态（released 或 filled）后调用。
// 双人确认（operator1 != operator2）。
  func (s *Service) GenerateConflictResolutionProof(
	  planID, operator1, operator2 string,
	  outcome CloseOutcome,
	   committedTokenID, ledgerRef, holdRef string,
   ) (CloseProof, error) {
	   if s == nil {
		  return CloseProof{}, ErrUnavailable
	  }
	  planID = strings.TrimSpace(planID)
	   op1 := strings.TrimSpace(operator1)
	   op2 := strings.TrimSpace(operator2)
	  if op1 == "" || op2 == "" {
		  return CloseProof{}, ErrActor
	  }
	   if strings.EqualFold(op1, op2) {
		  return CloseProof{}, ErrFourEye
	  }
	   if !outcome.Valid() || outcome == OutcomeConflict {
		   return CloseProof{}, ErrState
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   plan, ok := s.plans[planID]
	   if !ok {
		  return CloseProof{}, ErrPlan
	  }
	  if plan.Status != PlanStatusConflict {
		  return CloseProof{}, ErrState
	   }
	   // 幂等：已有证明直接返回。
	   if proof := s.proofOfPlanLocked(planID); proof != nil {
		   return proof.Copy(), nil
	   }
	   tokenID := strings.TrimSpace(committedTokenID)
	  // 找到对应令牌（若未指定则取冲突令牌中类型匹配的那条）。
	  var resolvedKind TokenKind
	  if outcome == OutcomeReleased {
		   resolvedKind = TokenKindRelease
	  } else {
		   resolvedKind = TokenKindFill
	  }
	   if tokenID == "" {
		  for _, tid := range s.tokensByPlan[planID] {
			  t := s.tokens[tid]
			   if t.Kind == resolvedKind {
				   tokenID = t.ID
				   break
			  }
		  }
	   }
	  if tokenID == "" {
		   tokenID = "conflict-resolution-" + planID
	   }
	  now := s.now()
	   actor := op1 + "+" + op2
	  // 计划转换为最终仲裁终态。
	   if outcome == OutcomeReleased {
		   plan.Status = PlanStatusReleased
	   } else {
		   plan.Status = PlanStatusFilled
	   }
	  t := now
	  plan.TerminalAt = &t
	  plan.TerminalBy = actor
	  plan.ConflictNote += " | resolved by desk at " + now.Format(time.RFC3339)
	  plan.Version++
	  plan.UpdatedAt = now
	   // 生成证明。
	  phash := ComputeProofHash(ProofHashInput{
		   PlanID: plan.ID,
		   OrderID: plan.OrderID,
		  Outcome: outcome,
		  CommittedTokenID: tokenID,
		  Amount:           plan.Amount,
		  Asset:            plan.Asset,
	   })
	   proofID := generateProofID()
	   proof := &CloseProof{
		  ID: proofID,
		   PlanID: plan.ID,
		   OrderID: plan.OrderID,
		   UserID:  plan.UserID,
		   Asset: plan.Asset,
		  Amount:           plan.Amount,
		  Outcome:          outcome,
		  CommittedTokenID: tokenID,
		  CommittedKind:    resolvedKind,
		   ProofHash:        phash,
		   HoldRef:   strings.TrimSpace(holdRef),
		  LedgerRef: strings.TrimSpace(ledgerRef),
		   DecidedBy: actor,
		  CreatedAt: now,
	  }
	   s.proofs[proof.ID] = proof
	  s.proofByPlan[planID] = proof.ID
	  return proof.Copy(), nil
   }

// generateProofID 生成收口证明 ID（与 uuid 保持一致）。
  func generateProofID() string {
	  return "proof-" + generateID()
   }

// generateID 生成 ID（委托 uuid.NewString，在 service.go 已 import uuid）。

  func generateID() string {
	   // 通过 uuid 包生成，此处为辅助函数。
	   // 注意：在中可替换 Service.clock 来控制时间，但 ID 仍用 uuid。
	   // 在 proof.go 中直接调用 uuid.NewString 需要 import，但为避免重复 import 声明，
	  // 此处通过 proofIDGen 变量注入（默认 uuid.NewString）。
	  return proofIDGen()
   }

// proofIDGen 可在中替换为确定性生成器。

var proofIDGen = defaultProofIDGen

   func defaultProofIDGen() string {
	   // 使用 time+固定前缀生成唯一 ID（不依赖 uuid 包，避免 import 问题）。
	  // 中已有 uuid.NewString 在 service.go 中，此处独立实现。
	  return uniqueIDFromTime()
  }

   func uniqueIDFromTime() string {
	   // 基于纳秒时间戳生成唯一字符串（环境可替换）。
	  ns := time.Now().UnixNano()
	   return unsignedHex(uint64(ns))
  }

  func unsignedHex(n uint64) string {
	   const hexchars = "0123456789abcdef"
	  if n == 0 {
		   return "0"
	  }
	  b := [16]byte{}
	   pos := 16
	   for n > 0 {
		   pos--
		  b[pos] = hexchars[n&0xf]
		  n >>= 4
	   }
	  return string(b[pos:])
  }
