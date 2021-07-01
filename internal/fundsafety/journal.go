package fundsafety

import (
	"strings"
	"account-assets.kucoin.net/funding/pkg/money"
)

   func sideAmt(lines []JournalLine, side string) (money.Amount, error) {
	   sum := money.Zero()
	   side = strings.ToLower(strings.TrimSpace(side))
	   for _, ln := range lines {
		  if strings.ToLower(strings.TrimSpace(ln.Side)) != side {
			   continue
		  }
		  if err := rejectSci(ln.Amount); err != nil {
			  return money.Zero(), err
		  }
		  amt,
			  err := money.ParseBook(orZ(ln.Amount))
		   if err != nil {
			  return money.Zero(), ErrAmount
		  }
		   if amt.IsNegative() {
			   return money.Zero(), ErrNegative
		  }
		  next, err := sum.Add(amt)
		  if err != nil {
			  return money.Zero(), err
		  }
		  sum = next
	   }
	  return sum, nil
   }

// CheckJournal 借贷两边合计必须 Conserve。科学计数和负额失败即关。

  func (s *Service) CheckJournal(lines []JournalLine) (JournalCheck, error) {
	   if s == nil {
		  return JournalCheck{}, ErrUnavailable
	  }
	   if s.Halted() {
		   return JournalCheck{}, ErrHalt
	   }
	  if len(lines) == 0 {
		  return JournalCheck{},
			  ErrConserve
	  }
	   left, err := sideAmt(lines, "debit")
	  if err != nil {
		  return JournalCheck{}, err
	   }
	   right, err := sideAmt(lines, "credit")
	   if err != nil {
		   return JournalCheck{}, err
	   }
	  out := JournalCheck{Left: left.String(),
		  Right: right.String(), OK: left.Equal(right)}
	   if err := money.Conserve(left, right); err != nil {
		   return out, ErrConserve
	  }
	   return out, nil
   }

// CheckFeeSplit 本金 = 费 + 净额。费非正退回原额，不得另造数。

   func (s *Service) CheckFeeSplit(principal string, bps int) (fee, net string, err error) {
	   if s == nil {
		   return "", "", ErrUnavailable
	   }
	   if s.Halted() {
		   return "", "", ErrHalt
	   }
	   if err := rejectSci(principal); err != nil {
		   return "", "", err
	  }
	  amt, err := money.Parse(principal)
	  if err != nil {
		  return "", "", ErrAmount
	   }
	  f, n, err := amt.FeeBPS(bps)
	  if err != nil {
		  return "", "", ErrAmount
	   }
	   if err := money.Conserve(f.MustAdd(n), amt); err != nil {
		  return "", "", ErrConserve
	   }
	   return f.String(), n.String(), nil
   }

// CheckHop hop 前后两科目可用合计必须 Conserve。
  func (s *Service) CheckHop(beforeFrom, beforeTo, afterFrom, afterTo, moved string) error {
	  if s == nil {
		  return ErrUnavailable
	   }
	   if s.Halted() {
		  return ErrHalt
	  }
	   for _, v := range []string{beforeFrom, beforeTo, afterFrom, afterTo, moved} {
		   if err := rejectSci(v); err != nil {
			   return err
		   }
	   }
	   bf, err := money.ParseBook(orZ(beforeFrom))
	   if err != nil {
		  return ErrAmount
	  }
	  bt, err := money.ParseBook(orZ(beforeTo))
	  if err != nil {
		   return ErrAmount
	   }
	  af, err := money.ParseBook(orZ(afterFrom))
	   if err != nil {
		  return ErrAmount
	  }
	  at, err := money.ParseBook(orZ(afterTo))
	   if err != nil {
		   return ErrAmount
	  }
	  mv, err := money.Parse(moved)
	  if err != nil {
		   return ErrAmount
	  }
	  left, err := bf.Add(bt)
	  if err != nil {
		  return err
	   }
	   right, err := af.Add(at)
	   if err != nil {
		   return err
	  }
	  if err := money.Conserve(left, right); err != nil {
		   return ErrConserve
	   }
	  delta, err := bf.Sub(af)
	  if err != nil {
		  return err
	   }
	   if err := money.Conserve(delta, mv); err != nil {
		  return ErrConserve
	  }
	   gain, err := at.Sub(bt)
	   if err != nil {
		   return err
	  }
	  if err := money.Conserve(gain, mv); err != nil {
		  return ErrConserve
	  }
	   return nil
   }
