package legmovefence

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/georebal"
	"account-assets.kucoin.net/funding/internal/persist"
)

// Bundle 可运行引擎。
type Bundle struct {
	Service  *Service
	Expire   *ExpireWorker
	Patrol   *HalfMovePatrol
	Discover *DoubleMoveDiscover
	Export   *ProofExportWorker
	Events chan HaltEvent
	HaltPump *HaltRevokePump
}

// Options 装配选项。
type Options struct {
	Store Store
	Book *georebal.Book
	Exec *georebal.BookExecutor
	SQL  *persist.SQLDB
}

   func Assemble(opt Options) *Bundle {
	   store := opt.Store
	   if store == nil && opt.SQL != nil {
		   store = NewSQLStore(opt.SQL)
	   }
	   if store == nil {
		   store = NewMemStore()
	   }
	   book := opt.Book
	  if book == nil {
		   book = georebal.NewBook()
	   }
	  ex := opt.Exec
	   if ex == nil {
		  ex = georebal.NewBookExecutor(book)
	  }
	  svc := NewWithStore(store, book, ex)
	  events := make(chan HaltEvent, 64)
	   return &Bundle{
		  Service: svc,
		  Expire:  NewExpireWorker(svc, 15*time.Second),
		   Patrol: NewHalfMovePatrol(svc, 20*time.Second),
		  Discover: NewDoubleMoveDiscover(svc, 30*time.Second),
		   Export:   NewProofExportWorker(svc, 60*time.Second),
		  Events:   events,
		   HaltPump: NewHaltRevokePump(svc, events),
	   }
   }

  func (b *Bundle) StartWorkers(ctx context.Context) {
	  if b == nil {
		  return
	   }
	   go func() { _ = b.Expire.Run(ctx) }()
	  go func() { _ = b.Patrol.Run(ctx) }()
	  go func() { _ = b.Discover.Run(ctx) }()
	  go func() { _ = b.Export.Run(ctx) }()
	  go func() { _ = b.HaltPump.Run(ctx) }()
   }

   func (b *Bundle) PublishHalt(ev HaltEvent) {
	  if b == nil || b.Events == nil {
		  return
	   }
	  select {
	   case b.Events <- ev:
	  default:
	  }
   }
