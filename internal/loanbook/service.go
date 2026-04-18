package loanbook

import (
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/google/uuid"
)

type Service struct {
	mu    sync.Mutex
	loans map[string]*Loan
	inst  map[string][]Installment
	tickets map[string]*Ticket
	byIdem  map[string]string
	led   *ledger.Service
	clock func() time.Time
}

   func New() *Service {
	   return &Service{
		   loans: map[string]*Loan{}, inst: map[string][]Installment{},
		  tickets: map[string]*Ticket{}, byIdem: map[string]string{},
		   clock: func() time.Time { return time.Now().UTC() },
	   }
   }

   func (s *Service) BindLedger(led *ledger.Service) *Service {
	  if s != nil {
		  s.led = led
	   }
	   return s
  }

   func (s *Service) now() time.Time {
	  if s == nil || s.clock == nil {
		   return time.Now().UTC()
	  }
	  return s.clock()
   }

  func (s *Service) Open(req OpenReq) (Loan, error) {
	  if s == nil {
		   return Loan{}, ErrUnavailable
	   }
	   uid, err := validate.UserID(req.UserID)
	   if err != nil {
		  return Loan{}, ErrUser
	   }
	  asset, err := validate.Asset(req.Asset)
	   if err != nil {
		  return Loan{}, ErrAsset
	   }
	   prin, err := money.Parse(req.Principal)
	   if err != nil {
		   return Loan{}, ErrAmount
	   }
	   col, err := money.Parse(req.Collateral)
	   if err != nil {
		  return Loan{}, ErrCollateral
	   }
	  colAsset, err := validate.Asset(first(req.ColAsset, asset))
	  if err != nil {
		  return Loan{}, ErrAsset
	  }
	  idem, err := validate.Idem(req.IdempotencyKey)
	   if err != nil {
		  return Loan{}, ErrIdem
	   }
	  cap := req.CapBPS
	   if cap <= 0 {
		  cap = 6500
	  }
	   if cap > 9000 {
		  return Loan{}, ErrRate
	  }
	  s.mu.Lock()
	  if id, ok := s.byIdem[idem]; ok {
		   cur := s.loans[id]
		   s.mu.Unlock()
		  if cur.UserID != uid || cur.Principal != prin.String() {
			  return Loan{}, ErrIdem
		  }
		  return cur.Copy(), nil
	   }
	   s.mu.Unlock()
	   pay, rows, err := Schedule(prin.String(), req.AnnualBPS, req.TermMonths, s.now())
	  if err != nil {
		   return Loan{}, err
	   }
	   now := s.now()
	  loan := &Loan{
		  ID: uuid.NewString(), IdempotencyKey: idem, UserID: uid, Asset: asset,
		   Principal: prin.String(), Remaining: prin.String(), AnnualBPS: req.AnnualBPS,
		   TermMonths: req.TermMonths, Payment: pay, Collateral: col.String(),
		  ColAsset: colAsset, CapBPS: cap, State: StateCurrent, OpenedAt: now, UpdatedAt: now,
	   }
	  for i := range rows {
		  rows[i].LoanID = loan.ID
	   }
	   s.mu.Lock()
	  s.loans[loan.ID] = loan
	   s.inst[loan.ID] = rows
	  s.byIdem[idem] = loan.ID
	   s.mu.Unlock()
	  return loan.Copy(), nil
  }

   func (s *Service) Get(id string) (Loan, error) {
	  if s == nil {
		  return Loan{}, ErrUnavailable
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  cur, ok := s.loans[strings.TrimSpace(id)]
	  if !ok {
		  return Loan{}, ErrLoan
	   }
	  return cur.Copy(), nil
  }

  func (s *Service) ScheduleOf(id string) ([]Installment, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  rows, ok := s.inst[strings.TrimSpace(id)]
	   if !ok {
		  return nil, ErrLoan
	   }
	   out := make([]Installment, len(rows))
	   copy(out, rows)
	   return out, nil
   }

  func (s *Service) List(userID string, limit int) []Loan {
	   if s == nil {
		  return nil
	   }
	   userID = strings.TrimSpace(userID)
	  if limit <= 0 {
		  limit = 50
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  out := make([]Loan, 0, limit)
	   for _, l := range s.loans {
		   if userID != "" && l.UserID != userID {
			   continue
		  }
		   out = append(out, l.Copy())
		  if len(out) >= limit {
			  break
		   }
	  }
	  return out
   }

  func first(a, b string) string {
	  if strings.TrimSpace(a) != "" {
		   return a
	  }
	   return b
   }
