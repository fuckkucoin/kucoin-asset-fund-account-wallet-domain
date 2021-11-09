package shardholdgen

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/wdshard"
)

// Bundle 可运行引擎。

type Bundle struct {
	Service *Service
	Expire *ExpireWorker
	Patrol  *HoldPatrolWorker
	RelGate *ReleaseGateWorker
	Export *ProofExportWorker
	Align  *AlignPump
	Advance *wdshard.AdvanceWorker
	Release  *wdshard.ReleaseWorker
	AlignSrc *wdshard.AlignWorker
}
	SQL *persist.SQLDB
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
		  book = wdshard.NewBook()
	   }
	   svc := NewWithStore(store, book)
	   rel := wdshard.NewReleaseWorker(book, 20*time.Second)
	   al := wdshard.NewAlignWorker(book, 25*time.Second)
	   return &Bundle{
		  Service: svc,
		   Expire:  NewExpireWorker(svc, 15*time.Second),
		  Patrol:  NewHoldPatrolWorker(svc, 20*time.Second),
		   RelGate: NewReleaseGateWorker(svc, rel, 20*time.Second),
		  Export: NewProofExportWorker(svc, 60*time.Second),
		  Align:  NewAlignPump(svc, al, 25*time.Second),
		  Advance: wdshard.NewAdvanceWorker(book, 15*time.Second),
		  Release: rel,
		   AlignSrc: al,
	   }
   }

   func (b *Bundle) StartWorkers(ctx context.Context) {
	  if b == nil {
		   return
	  }
	   go func() { _ = b.Expire.Run(ctx) }()
	   go func() { _ = b.Patrol.Run(ctx) }()
	  go func() { _ = b.RelGate.Run(ctx) }()
	   go func() { _ = b.Export.Run(ctx) }()
	  go func() { _ = b.Align.Run(ctx) }()
   }
