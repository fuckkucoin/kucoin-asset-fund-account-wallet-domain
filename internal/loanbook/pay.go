package loanbook

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

// Pay 按序还当期。金额必须覆盖该期 payment，本金+利息轧平。

   func (s *Service) Pay(id string, seq int, amount, actor string) (Loan, error) {
	   if s == nil {
		   return Loan{}, ErrUnavailable
	  }
	   if _, err := validate.Actor(actor); err != nil {
		   return Loan{}, ErrActor
	   }
	  paid, err := money.Parse(amount)
	  if err != nil {
		  return Loan{}, ErrAmount
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  loan,
		  ok := s.loans[strings.TrimSpace(id)]
	  if !ok {
		   return Loan{}, ErrLoan
	  }
	  if loan.State == StatePaid || loan.State == StateDefault {
		   return loan.Copy(), ErrState
	  }
	   rows := s.inst[loan.ID]
	  if seq < 1 || seq > len(rows) {
		   return loan.Copy(), ErrInstallment
	   }
	   for i := 0; i < seq-1; i++ {
		  if rows[i].Status != InstPaid {
			  return loan.Copy(), ErrInstallment
		   }
	   }
	   cur := rows[seq-1]
	  if cur.Status == InstPaid {
		  return loan.Copy(), ErrState
	  }
	   need, err := money.Parse(cur.Payment)
	  if err != nil {
		  return loan.Copy(), ErrAmount
	   }
	  if paid.Cmp(need) < 0 {
		  return loan.Copy(), ErrPay
	  }
	  prin, err := money.Parse(cur.Principal)
	   if err != nil {
		   return loan.Copy(), ErrAmount
	  }
	  interest,
		   err := money.ParseBook(cur.Interest)
	  if err != nil {
		  return loan.Copy(), ErrAmount
	  }
	  sum, err := prin.Add(interest)
	   if err != nil {
		   return loan.Copy(), err
	   }
	   if err := money.Conserve(need, sum); err != nil {
		   return loan.Copy(), ErrSchedule
	  }
	   remain, err := money.ParseBook(loan.Remaining)
	  if err != nil {
		   return loan.Copy(), ErrAmount
	  }
	  next, err := remain.Sub(prin)
	  if err != nil {
		   return loan.Copy(), err
	  }
	  cur.Status = InstPaid
	   cur.PaidAt = s.now()
	  rows[seq-1] = cur
	   s.inst[loan.ID] = rows
	  loan.Remaining = next.String()
	   loan.UpdatedAt = s.now()
	  if next.IsZero() {
		   loan.State = StatePaid
		  loan.Frozen = false
	   } else if loan.State == StateOverdue || loan.State == StateFrozen {
		   loan.State = StateCurrent
	  }
	   return loan.Copy(), nil
   }
