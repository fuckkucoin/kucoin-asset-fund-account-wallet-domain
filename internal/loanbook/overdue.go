package loanbook

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/validate"
)

// ScanOverdue 过期未还记 overdue，并冻账户。不解冻——补还后人手解。
   func (s *Service) ScanOverdue(ctx context.Context, actor string) (int, error) {
	  if s == nil {
		  return 0, ErrUnavailable
	   }
	   if _, err := validate.Actor(actor); err != nil {
		   return 0, ErrActor
	   }
	   now := s.now()
	   s.mu.Lock()
	   ids := make([]string, 0)
	   for id, rows := range s.inst {
		  loan := s.loans[id]
		  if loan == nil || loan.State == StatePaid || loan.State == StateDefault {
			  continue
		  }
		   hit := false
		  for i := range rows {
			   if rows[i].Status == InstPaid {
				   continue
			   }
			   if now.After(rows[i].DueAt) {
				   rows[i].Status = InstOverdue
				   hit = true
			   }
		   }
		   if hit {
			   loan.State = StateOverdue
			  loan.UpdatedAt = now
			   ids = append(ids, loan.UserID)
		   }
	  }
	   s.mu.Unlock()
	   n := 0
	   seen := map[string]struct{}{}
	  for _, uid := range ids {
		   if _, ok := seen[uid]; ok {
			   continue
		   }
		  seen[uid] = struct{}{}
		  if err := s.freezeUser(ctx, uid); err != nil {
			   return n, err
		  }
		   n++
	  }
	   return n, nil
  }

  func (s *Service) freezeUser(ctx context.Context, userID string) error {
	   s.mu.Lock()
	   for _, l := range s.loans {
		   if l.UserID == userID && l.State == StateOverdue {
			  l.Frozen = true
			  l.State = StateFrozen
		  }
	   }
	  s.mu.Unlock()
	  if s.led == nil {
		  return nil
	  }
	  return s.led.SetFrozen(ctx,
		   userID, true)
  }

  func (s *Service) OverdueOf(id string) []Installment {
	  if s == nil {
		  return nil
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   rows := s.inst[strings.TrimSpace(id)]
	  out := make([]Installment, 0)
	   for _, r := range rows {
		   if r.Status == InstOverdue {
			  out = append(out, r)
		  }
	  }
	  return out
   }
