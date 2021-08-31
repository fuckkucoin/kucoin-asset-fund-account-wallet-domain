package loanbook

import (
	"strings"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/marginhook"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/validate"
)

// NeedTopUp 剩余本金相对抵押超 cap 就要补仓。

   func NeedTopUp(remaining, collateral string, capBPS int) (string, bool, error) {
	   rem, err := money.ParseBook(remaining)
	   if err != nil {
		  return "", false, ErrAmount
	   }
	  col, err := money.ParseBook(collateral)
	   if err != nil {
		   return "", false, ErrCollateral
	   }
	  if rem.IsZero() {
		   return "0", false, nil
	   }
	  if capBPS <= 0 {
		   capBPS = 6500
	   }
	   if err := marginhook.CheckLTV(rem.String(), col.String(), capBPS); err == nil {
		   return "0", false, nil
	  }
	  needCol := rem.Dec().Mul(decimal.NewFromInt(10000)).Div(decimal.NewFromInt(int64(capBPS)))
	   want := money.FromDecimal(needCol.Round(18))
	  if want.Cmp(col) <= 0 {
		  return "0", false, nil
	   }
	  gap, err := want.Sub(col)
	  if err != nil {
		  return "", false, err
	  }
	  return gap.String(), true, nil
  }

  func (s *Service) OpenTopUp(loanID, actor, note string) (Ticket, error) {
	   if s == nil {
		   return Ticket{}, ErrUnavailable
	   }
	   actor,
		   err := validate.Actor(actor)
	   if err != nil {
		   return Ticket{}, ErrActor
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   loan, ok := s.loans[strings.TrimSpace(loanID)]
	   if !ok {
		   return Ticket{}, ErrLoan
	   }
	   need, hit, err := NeedTopUp(loan.Remaining, loan.Collateral, loan.CapBPS)
	   if err != nil {
		  return Ticket{}, err
	  }
	   if !hit {
		   return Ticket{}, ErrTicket
	  }
	  now := s.now()
	   t := &Ticket{
		  ID:     uuid.NewString(),
		   LoanID: loan.ID, UserID: loan.UserID,
		   Kind: TicketTopUp, Need: need, Asset: loan.ColAsset, Status: TicketOpen,
		  Note: strings.TrimSpace(note), OpenedBy: actor, OpenedAt: now, UpdatedAt: now,
	  }
	   s.tickets[t.ID] = t
	   return t.Copy(), nil
  }

  func (s *Service) FillTopUp(id, amount, actor string) (Ticket, error) {
	   if s == nil {
		  return Ticket{}, ErrUnavailable
	   }
	   if _, err := validate.Actor(actor); err != nil {
		  return Ticket{}, ErrActor
	  }
	   add, err := money.Parse(amount)
	  if err != nil {
		   return Ticket{}, ErrAmount
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   t, ok := s.tickets[strings.TrimSpace(id)]
	   if !ok {
		   return Ticket{}, ErrTicket
	  }
	   if t.Status != TicketOpen {
		   return t.Copy(), ErrState
	  }
	  loan, ok := s.loans[t.LoanID]
	   if !ok {
		  return Ticket{}, ErrLoan
	   }
	   cur, err := money.ParseBook(loan.Collateral)
	   if err != nil {
		   return Ticket{}, ErrCollateral
	  }
	   next, err := cur.Add(add)
	   if err != nil {
		   return Ticket{}, err
	   }
	   loan.Collateral = next.String()
	   loan.UpdatedAt = s.now()
	  t.Status = TicketFilled
	  t.UpdatedAt = s.now()
	  return t.Copy(), nil
   }

   func (s *Service) CloseTicket(id, actor string) (Ticket, error) {
	  if s == nil {
		   return Ticket{}, ErrUnavailable
	   }
	  actor, err := validate.Actor(actor)
	   if err != nil {
		   return Ticket{}, ErrActor
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  t, ok := s.tickets[strings.TrimSpace(id)]
	   if !ok {
		  return Ticket{}, ErrTicket
	  }
	   if t.Status != TicketFilled && t.Status != TicketOpen {
		   return t.Copy(), ErrState
	   }
	  t.Status = TicketClosed
	  t.ClosedBy = actor
	   t.UpdatedAt = s.now()
	   return t.Copy(), nil
   }

  func (s *Service) Tickets(loanID string) []Ticket {
	  if s == nil {
		   return nil
	   }
	   loanID = strings.TrimSpace(loanID)
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   out := make([]Ticket, 0)
	   for _, t := range s.tickets {
		   if loanID != "" && t.LoanID != loanID {
			   continue
		  }
		  out = append(out, t.Copy())
	   }
	  return out
   }
