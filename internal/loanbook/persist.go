package loanbook

import (
	"context"
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindLoanDump = "loanbook_dump"

type dump struct {
	Loans []Loan `json:"loans"`
	Inst []Installment `json:"installments"`
	Tickets []Ticket `json:"tickets"`
}

  func (s *Service) Save(ctx context.Context, kv persist.KV, key string) error {
	  if s == nil || kv == nil {
		   return ErrUnavailable
	  }
	  key = strings.TrimSpace(key)
	   if key == "" {
		   return ErrLoan
	   }
	   s.mu.Lock()
	  d := dump{}
	  for _, l := range s.loans {
		   d.Loans = append(d.Loans, l.Copy())
	  }
	   for _, rows := range s.inst {
		   d.Inst = append(d.Inst, rows...)
	   }
	   for _, t := range s.tickets {
		  d.Tickets = append(d.Tickets,
			   t.Copy())
	   }
	   s.mu.Unlock()
	   body, err := json.Marshal(d)
	  if err != nil {
		   return err
	   }
	   _, existed, err := persist.GetOrInit(ctx, kv, kindLoanDump, key, body)
	  if err != nil {
		  return err
	   }
	  if existed {
		  return persist.Replace(ctx, kv, kindLoanDump, key, body)
	   }
	  return nil
   }

  func Load(ctx context.Context, kv persist.KV, key string) (*Service, error) {
	  if kv == nil {
		   return nil, ErrUnavailable
	  }
	  row, err := kv.Get(ctx, kindLoanDump, strings.TrimSpace(key))
	   if err != nil {
		  if err == persist.ErrNotFound {
			  return nil, ErrLoan
		  }
		  return nil, err
	  }
	  var d dump
	  if err := json.Unmarshal(row.Body, &d); err != nil {
		   return nil, err
	   }
	   s := New()
	   s.mu.Lock()
	   for i := range d.Loans {
		  cp := d.Loans[i]
		  s.loans[cp.ID] = &cp
		  if cp.IdempotencyKey != "" {
			   s.byIdem[cp.IdempotencyKey] = cp.ID
		  }
	   }
	  for _, inst := range d.Inst {
		  s.inst[inst.LoanID] = append(s.inst[inst.LoanID], inst)
	  }
	   for i := range d.Tickets {
		   cp := d.Tickets[i]
		  s.tickets[cp.ID] = &cp
	   }
	  s.mu.Unlock()
	  return s, nil
   }
