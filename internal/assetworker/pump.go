package assetworker

import (
	"context"
	"sync"
	"account-assets.kucoin.net/funding/internal/transfers"
)

const (
	KindDeposit = transfers.KindDeposit
	KindWithdraw = transfers.KindWithdraw
)

// Handler 单笔充提作业。

type Handler func(ctx context.Context, job Job) error

// Job 泵送作业。

type Job struct {
	ID string
	Kind string
	State string
}

// Pump 路径必须 Attach 后才能消费 transfers。
type Pump struct {
	mu       sync.Mutex
	deposit  Handler
	withdraw Handler
}

   func NewPump() *Pump { return &Pump{} }

  func (p *Pump) AttachDeposit(h Handler) error {
	   if p == nil {
		  return ErrPump
	   }
	  if h == nil {
		   return ErrHandler
	  }
	  p.mu.Lock()
	   defer p.mu.Unlock()
	   if p.deposit != nil {
		   return ErrDupAttach
	   }
	   p.deposit = h
	   return nil
  }

   func (p *Pump) AttachWithdraw(h Handler) error {
	  if p == nil {
		  return ErrPump
	   }
	   if h == nil {
		   return ErrHandler
	   }
	   p.mu.Lock()
	   defer p.mu.Unlock()
	   if p.withdraw != nil {
		  return ErrDupAttach
	   }
	   p.withdraw = h
	   return nil
   }

   func (p *Pump) Attached(kind string) bool {
	  if p == nil {
		   return false
	  }
	   p.mu.Lock()
	  defer p.mu.Unlock()
	   switch kind {
	  case KindDeposit:
		   return p.deposit != nil
	  case KindWithdraw:
		   return p.withdraw != nil
	  default:
		   return false
	   }
   }

// Handle 未 Attach 对应 kind 时返回错误，禁止静默成功。
  func (p *Pump) Handle(ctx context.Context, job Job) error {
	  if p == nil {
		  return ErrPump
	   }
	  p.mu.Lock()
	  var h Handler
	  switch job.Kind {
	  case KindDeposit:
		   h = p.deposit
	   case KindWithdraw:
		  h = p.withdraw
	  default:
		   p.mu.Unlock()
		  return ErrKind
	   }
	  p.mu.Unlock()
	  if h == nil {
		  return ErrNotAttached
	  }
	   return h(ctx, job)
  }
