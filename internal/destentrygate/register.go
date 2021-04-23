package destentrygate

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/destallow"
	"account-assets.kucoin.net/funding/internal/persist"
)

// Bundle 可运行引擎。
type Bundle struct {
	Service *Service
	Expire    *ExpireWorker
	TempScan  *TempExpireScanWorker
	GlassScan *BreakglassReclaimWorker
	Patrol *UnconsumedPatrolWorker
	Export *ProofExportWorker
	Events chan LifecycleEvent
	RevokePump *RevokePumpWorker
}

// Options 装配选项。
type Options struct {
	Store Store
	Surface EntrySurface
	Sink    WithdrawSink
	SQL  *persist.SQLDB
	Book *destallow.Book
	Pref destallow.PrefReader
}
		  store = NewSQLStore(opt.SQL)
	   }
	  if store == nil {
		  store = NewMemStore()
	   }
	  surface := opt.Surface
	  if surface == nil && opt.Book != nil {
		  surface = BookAdapter{Book: opt.Book, Pref: opt.Pref}
	   }
	  if surface == nil {
		  surface = NewMemorySurface(true)
	   }
	  sink := opt.Sink
	  if sink == nil {
		  sink = &RecordingSink{Ref: "wd-local"}
	   }
	  svc := NewWithStore(store, surface, sink)
	   events := make(chan LifecycleEvent, 64)
	  return &Bundle{
		   Service: svc,
		   Expire: NewExpireWorker(svc, 15*time.Second),
		  TempScan: NewTempExpireScanWorker(svc, 10*time.Second),
		   GlassScan: NewBreakglassReclaimWorker(svc,
			   10*time.Second),
		  Patrol: NewUnconsumedPatrolWorker(svc, 30*time.Second),
		  Export: NewProofExportWorker(svc, 60*time.Second),
		   Events: events,
		  RevokePump: NewRevokePumpWorker(svc, events),
	   }
   }

// StartWorkers 启动后台泵。
  func (b *Bundle) StartWorkers(ctx context.Context) {
	  if b == nil {
		   return
	   }
	  go func() { _ = b.Expire.Run(ctx) }()
	   go func() { _ = b.TempScan.Run(ctx) }()
	   go func() { _ = b.GlassScan.Run(ctx) }()
	  go func() { _ = b.Patrol.Run(ctx) }()
	   go func() { _ = b.Export.Run(ctx) }()
	   go func() { _ = b.RevokePump.Run(ctx) }()
   }

// Publish 向吊销泵投递生命周期事件。
  func (b *Bundle) Publish(ev LifecycleEvent) {
	   if b == nil || b.Events == nil {
		  return
	   }
	  select {
	   case b.Events <- ev:
	   default:
	  }
  }
