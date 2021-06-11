package fundintgate

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/fundint"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/userdir"
)

// Bundle 可运行的引擎装配。

type Bundle struct {
	Service *Service
	Expire  *ExpireWorker
	Patrol  *UnconsumedPatrolWorker
	Export *ProofExportWorker
	Flips  chan GateFlipEvent
	Revoke *HaltFlipRevokeWorker
}

// Options 装配选项。

type Options struct {
	Store Store
	Resolver Resolver
	Gate     GateEval
	Sender   Sender
	SQL      *persist.SQLDB
	Dir userdir.Directory
	Led *ledger.Service
	GateFn fundint.GateFunc
	MinAmt string
}

// Assemble 按选项装配引擎与 Worker。

   func Assemble(opt Options) *Bundle {
	   store := opt.Store
	   if store == nil && opt.SQL != nil {
		  store = NewSQLStore(opt.SQL)
	   }
	   if store == nil {
		   store = NewMemStore()
	  }
	  resolver := opt.Resolver
	   if resolver == nil && opt.Dir != nil {
		   resolver = DirectoryResolver{Dir: opt.Dir}
	   }
	  gate := opt.Gate
	  if gate == nil && opt.GateFn != nil {
		   gate = GateFuncAdapter{Fn: opt.GateFn}
	   }
	   sender := opt.Sender
	   if sender == nil && opt.Led != nil {
		   sender = LedgerSender{Led: opt.Led}
	   }
	   svc := NewWithStore(store, resolver, gate, sender)
	  if opt.MinAmt != "" {
		   svc.BindMinAmount(opt.MinAmt)
	   }
	   flips := make(chan GateFlipEvent, 32)
	  b := &Bundle{
		  Service: svc,
		   Expire:  NewExpireWorker(svc, 15*time.Second),
		  Patrol:  NewUnconsumedPatrolWorker(svc, 30*time.Second),
		  Export:  NewProofExportWorker(svc, 60*time.Second),
		  Flips:   flips,
		   Revoke:  NewHaltFlipRevokeWorker(svc, flips),
	  }
	  return b
  }

// StartWorkers 启动后台泵。
  func (b *Bundle) StartWorkers(ctx context.Context) {
	  if b == nil {
		   return
	  }
	   go func() { _ = b.Expire.Run(ctx) }()
	   go func() { _ = b.Patrol.Run(ctx) }()
	  go func() { _ = b.Export.Run(ctx) }()
	  go func() { _ = b.Revoke.Run(ctx) }()
  }

// PublishFlip 向吊销泵投递翻转事件。
  func (b *Bundle) PublishFlip(ev GateFlipEvent) {
	   if b == nil || b.Flips == nil {
		  return
	  }
	   select {
	   case b.Flips <- ev:
	  default:
	  }
  }
