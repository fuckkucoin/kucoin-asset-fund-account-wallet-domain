package p2pgate

import (
	"context"
	"fmt"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/kyt"
	"account-assets.kucoin.net/funding/internal/kytsvc"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/p2pnet"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/validate"
)

var (
	ErrUnavailable = fmt.Errorf("p2p gate unavailable")
	ErrKYT         = fmt.Errorf("p2p gate kyt refused")
	ErrAmount = fmt.Errorf("p2p gate amount invalid")
)

type Result struct {
	Transfer p2pnet.Transfer `json:"transfer"`
	KYT      kytsvc.Result   `json:"kyt"`
}

type Service struct {
	mu sync.Mutex
	last []Result
	p2p *p2pnet.Service
	kyt *kytsvc.Service
}

  func New() *Service {
	  return &Service{
		  p2p: p2pnet.New(),
		   kyt: kytsvc.New(kyt.Static{Mode: kyt.Allow}),
	  }
   }

  func (s *Service) BindLedger(led *ledger.Service) *Service {
	   if s != nil && s.p2p != nil {
		  s.p2p.BindLedger(led)
	  }
	  return s
  }

  func (s *Service) BindKYT(k *kytsvc.Service) *Service {
	   if s != nil && k != nil {
		   s.kyt = k
	  }
	  return s
   }

  func (s *Service) P2P() *p2pnet.Service {
	  if s == nil {
		   return p2pnet.New()
	   }
	   return s.p2p
   }

   func (s *Service) Send(ctx context.Context, req p2pnet.SendReq) (Result, error) {
	  if s == nil {
		  return Result{}, ErrUnavailable
	   }
	   from, err := validate.UserID(req.From)
	   if err != nil {
		  return Result{}, err
	   }
	   to, err := validate.UserID(req.To)
	   if err != nil {
		   return Result{}, err
	   }
	   as, err := validate.Asset(req.Asset)
	   if err != nil {
		  return Result{}, err
	   }
	   if _, err := money.Parse(req.Amount); err != nil {
		  return Result{}, ErrAmount
	  }
	   kr, err := s.kyt.Screen(ctx, kyt.Request{
		   UserID: from, MovementID: req.IdempotencyKey, Chain: "ETH",
		   ToAddress: to, Asset: as, Amount: req.Amount,
	   })
	   if err != nil || kr.Decision != kyt.Allow {
		  return Result{}, ErrKYT
	  }
	  tr, err := s.p2p.Send(ctx, req)
	  if err != nil {
		  return Result{}, err
	   }
	  out := Result{Transfer: tr, KYT: kr}
	   s.mu.Lock()
	  s.last = append(s.last, out)
	  if len(s.last) > 200 {
		  s.last = s.last[len(s.last)-200:]
	   }
	  s.mu.Unlock()
	  return out, nil
   }

  func (s *Service) Recent(limit int) []Result {
	  if s == nil {
		  return nil
	   }
	  if limit <= 0 {
		   limit = 50
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  if len(s.last) <= limit {
		   return append([]Result(nil), s.last...)
	   }
	  return append([]Result(nil), s.last[len(s.last)-limit:]...)
  }

  func (s *Service) now() time.Time { return time.Now().UTC() }
