package p2pabandon

import (
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

// TokenSummary 令牌汇总（用于 API 响应与日志）。
type TokenSummary struct {
	PlanID       string      `json:"plan_id"`
	OrderID      string      `json:"order_id"`
	ReleaseToken *TokenBrief `json:"release_token,omitempty"`
	FillToken    *TokenBrief `json:"fill_token,omitempty"`
	ConflictPresent bool `json:"conflict_present"`
	// MutexResult 互斥结果：release_won | fill_won | conflict | pending。
	MutexResult string `json:"mutex_result"`
}

// TokenBrief 令牌摘要（不含内部字段）。
type TokenBrief struct {
	ID string `json:"id"`
	Kind TokenKind `json:"kind"`
	Status TokenStatus `json:"status"`
	ReservedBy       string `json:"reserved_by"`
	CommittedBy      string `json:"committed_by,omitempty"`
	ConflictDetected bool   `json:"conflict_detected"`
	ReservedAt time.Time `json:"reserved_at"`
	TerminalAt *time.Time `json:"terminal_at,omitempty"`
}

   func tokenToBrief(t MutexToken) *TokenBrief {
	  return &TokenBrief{
		  ID:     t.ID,
		  Kind:   t.Kind,
		  Status: t.Status,
		  ReservedBy:  t.ReservedBy,
		   CommittedBy: t.CommittedBy,
		   ConflictDetected: t.ConflictDetected,
		  ReservedAt: t.ReservedAt,
		  TerminalAt: t.TerminalAt,
	  }
   }

// GetTokenSummary 获取计划的互斥令牌汇总（用于 API 响应）。

  func (s *Service) GetTokenSummary(planID string) (TokenSummary, error) {
	   if s == nil {
		  return TokenSummary{}, ErrUnavailable
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   plan, ok := s.plans[strings.TrimSpace(planID)]
	  if !ok {
		  return TokenSummary{}, ErrPlan
	  }
	  sum := TokenSummary{
		   PlanID:      plan.ID,
		   OrderID:     plan.OrderID,
		   MutexResult: "pending",
	  }
	   for _, tid := range s.tokensByPlan[planID] {
		  t := s.tokens[tid]
		  switch t.Kind {
		  case TokenKindRelease:
			  sum.ReleaseToken = tokenToBrief(t.Copy())
			  if t.ConflictDetected {
				  sum.ConflictPresent = true
			   }
		  case TokenKindFill:
			  sum.FillToken = tokenToBrief(t.Copy())
			   if t.ConflictDetected {
				   sum.ConflictPresent = true
			  }
		   }
	   }
	   if sum.ConflictPresent {
		  sum.MutexResult = "conflict"
	   } else if sum.ReleaseToken != nil && sum.ReleaseToken.Status == TokenStatusCommitted {
		  sum.MutexResult = "release_won"
	  } else if sum.FillToken != nil && sum.FillToken.Status == TokenStatusCommitted {
		  sum.MutexResult = "fill_won"
	  }
	   return sum, nil
  }

// ReserveExternalFillToken 由外部成交 worker 调用，预先预留 fill 令牌。
// 设计意图：P2P 成交贷记流程在贷记完成前调用此方法占位；
// 若此时 release 令牌已 committed，立刻返回 ErrTokenConflict，
// 让 fill 流程知晓竞态已发生并走 conflict 处理流程。

   func (s *Service) ReserveExternalFillToken(planID, actor string) (MutexToken, error) {
	  if s == nil {
		   return MutexToken{},
			  ErrUnavailable
	  }
	  planID = strings.TrimSpace(planID)
	   actor = strings.TrimSpace(actor)
	  if actor == "" {
		  actor = "fill_worker"
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   plan, ok := s.plans[planID]
	   if !ok {
		  return MutexToken{}, ErrPlan
	  }
	  if plan.Status.Terminal() {
		  return MutexToken{}, ErrPlanTerminal
	  }
	  // 若 release 令牌已 committed，直接返回冲突错误。
	  if s.hasReleaseCommittedLocked(planID) {
		  return MutexToken{}, fmt.Errorf("%w: release already committed for plan %s", ErrTokenConflict, planID)
	   }
	   // 若已存在 fill 令牌，幂等返回。
	   for _, tid := range s.tokensByPlan[planID] {
		  t := s.tokens[tid]
		  if t.Kind == TokenKindFill {
			  return t.Copy(), nil
		  }
	  }
	  now := s.now()
	   fillToken := &MutexToken{
		  ID:     uuid.NewString(),
		  PlanID: planID,
		  OrderID: plan.OrderID,
		   Kind: TokenKindFill,
		   Status:     TokenStatusReserved,
		  ReservedBy: actor,
		  ReservedAt: now,
		   Version:    1,
		  CreatedAt:  now,
		  UpdatedAt: now,
	   }
	   s.tokens[fillToken.ID] = fillToken
	  s.tokensByPlan[planID] = append(s.tokensByPlan[planID], fillToken.ID)
	   return fillToken.Copy(), nil
  }

// AbortExternalFillToken 外部成交 worker 在 fill 贷记失败时主动 abort fill 令牌。
   func (s *Service) AbortExternalFillToken(planID, tokenID, reason string) error {
	   if s == nil {
		   return ErrUnavailable
	  }
	   planID = strings.TrimSpace(planID)
	   tokenID = strings.TrimSpace(tokenID)
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  t, ok := s.tokens[tokenID]
	   if !ok {
		  return ErrToken
	  }
	   if t.PlanID != planID || t.Kind != TokenKindFill {
		   return ErrToken
	   }
	  if t.Status.Terminal() {
		  return nil // 幂等
	  }
	   now := s.now()
	  t.Status = TokenStatusAborted
	   t.AbortedBy = "fill_worker"
	   t.AbortReason = strings.TrimSpace(reason)
	  tt := now
	   t.TerminalAt = &tt
	  t.Version++
	   t.UpdatedAt = now
	   return nil
   }
