package loanbook

import (
	"strings"
	"account-assets.kucoin.net/funding/pkg/money"
)

// RemainingInterest 未还各期利息之和。已还期不计入。
  func RemainingInterest(rows []Installment) (string, error) {
	  sum := money.Zero()
	  for _, r := range rows {
		   if r.Status == InstPaid {
			  continue
		   }
		  got, err := money.ParseBook(r.Interest)
		   if err != nil {
			  return "", ErrAmount
		   }
		  sum, err = sum.Add(got)
		  if err != nil {
			   return "", err
		   }
	  }
	   for _, r := range rows {
		  if r.Status == InstPaid {
			   continue
		   }
		  got,
			  err := money.Parse(r.Principal)
		  if err != nil {
			   return "", ErrAmount
		   }
		  sum, err = sum.Add(got)
		  if err != nil {
			   return "", err
		   }
	   }
	   return sum.String(), nil
  }

   func (s *Service) RemainingInterest(id string) (string, error) {
	   rows, err := s.ScheduleOf(id)
	   if err != nil {
		  return "", err
	  }
	  return RemainingInterest(rows)
  }

  func (s *Service) DueNext(id string) (Installment, error) {
	   if s == nil {
		  return Installment{}, ErrUnavailable
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   rows, ok := s.inst[strings.TrimSpace(id)]
	  if !ok {
		  return Installment{}, ErrLoan
	  }
	  for _, r := range rows {
		   if r.Status != InstPaid {
			  return r, nil
		   }
	  }
	  return Installment{}, ErrState
  }
