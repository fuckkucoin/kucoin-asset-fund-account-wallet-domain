package disputedesk

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/validate"
)

  func (s *Service) MarkReview(id, actor string) (Case, error) {
	  if s == nil {
		  return Case{}, ErrUnavailable
	   }
	  actor,
		  err := validate.Actor(actor)
	  if err != nil {
		  return Case{}, ErrActor
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   cur, ok := s.cases[strings.TrimSpace(id)]
	  if !ok {
		  return Case{}, ErrCase
	   }
	  if cur.Status != StatusEvidence && cur.Status != StatusOpen {
		  return cur.Copy(), ErrState
	   }
	   if len(s.ev[cur.ID]) == 0 {
		   return cur.Copy(), ErrEvidence
	   }
	   cur.Status = StatusReview
	  cur.UpdatedAt = s.now()
	  s.noteLocked(cur.ID, actor, "review", "")
	  return cur.Copy(), nil
  }

// ResolveFourEye 开案人和用户本人都不能结。证据为空失败即关。
// 若注入了 GateChecker，还会检查资金收口闸状态：blocking 时拒绝结案。

  func (s *Service) ResolveFourEye(id, actor, decision, note string) (Case, error) {
	   if s == nil {
		  return Case{}, ErrUnavailable
	  }
	  actor, err := validate.Actor(actor)
	   if err != nil {
		   return Case{}, ErrActor
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   cur, ok := s.cases[strings.TrimSpace(id)]
	  if !ok {
		  return Case{}, ErrCase
	   }
	   if cur.Status.Terminal() {
		  return cur.Copy(), ErrState
	  }
	  if strings.EqualFold(actor, cur.OpenedBy) {
		  return cur.Copy(), ErrFourEye
	   }
	   if strings.EqualFold(actor, cur.UserID) {
		  return cur.Copy(), ErrFourEye
	   }
	  if len(s.ev[cur.ID]) == 0 {
		  return cur.Copy(), ErrEvidence
	  }
	   // 资金收口闸校验：追偿腿未全部终态时禁止结案（验收要求 1）。
	   if err := s.checkGateLocked(cur.ID); err != nil {
		   return cur.Copy(), err
	  }
	  to := StatusResolved
	  switch strings.ToLower(strings.TrimSpace(decision)) {
	   case "resolve", "close", "uphold", "":
		  to = StatusResolved
	  case "reject", "deny":
		  to = StatusRejected
	  default:
		  return cur.Copy(), ErrState
	   }
	  cur.Status = to
	   cur.ClosedBy = actor
	  cur.Decision = strings.ToLower(strings.TrimSpace(decision))
	  if cur.Decision == "" {
		   cur.Decision = "resolve"
	  }
	   cur.Note = strings.TrimSpace(note)
	  cur.UpdatedAt = s.now()
	   if _, err := s.bumpGenerationLocked(cur.ID); err != nil {
		  return cur.Copy(), err
	  }
	   s.noteLocked(cur.ID,
		   actor, string(to), cur.Note)
	   return cur.Copy(), nil
   }
