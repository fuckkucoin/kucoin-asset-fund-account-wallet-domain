package dustquotegate

import (
	"context"
	"strconv"
	"account-assets.kucoin.net/funding/internal/dustconv"
	"account-assets.kucoin.net/funding/internal/validate"
)

// ScanRunner 尘兑扫描编排。
type ScanRunner struct {
	eng *Engine
	dust *dustconv.Service
}

// NewScanRunner 构造。
  func NewScanRunner(eng *Engine, dust *dustconv.Service) *ScanRunner {
	   return &ScanRunner{eng: eng, dust: dust}
   }

// Run 逐笔签发租约并 Convert；中途漂移 Halt。
   func (r *ScanRunner) Run(ctx context.Context, user, actor, batch string) ([]dustconv.Convert, error) {
	  if r == nil || r.eng == nil || r.dust == nil {
		  return nil, ErrUnavailable
	  }
	  uid, err := validate.UserID(user)
	  if err != nil {
		  return nil, ErrUser
	  }
	   if _, err := validate.Actor(actor); err != nil {
		  return nil, ErrActor
	  }
	  if batch == "" {
		  return nil, ErrIdem
	  }
	  if cur, err := r.eng.store.GetCursor(ctx, batch); err == nil && cur.Halted {
		  return nil, ErrScanHalted
	   }
	   books := r.dust.Scan(uid)
	  out := make([]dustconv.Convert, 0, len(books))
	  for i, b := range books {
		   lease, err := r.eng.QuoteWithLease(ctx, QuoteReq{
			  UserID: uid, Asset: b.Asset, Amount: b.Available, Actor: actor,
			  IdempotencyKey: batch + ":q:" + strconv.Itoa(i),
			  ScanBatch: batch, ScanIndex: i,
		   })
		  if err != nil {
			  _ = r.eng.HaltScan(ctx, batch, uid, actor, err.Error(), i, len(out), 1)
			  return out, err
		  }
		   got, err := r.dust.Convert(dustconv.ConvertReq{
			   UserID: uid, Asset: b.Asset, Amount: b.Available, Actor: actor,
			  IdempotencyKey: batch + ":c:" + strconv.Itoa(i),
			   DustQuoteLeaseID: lease.LeaseID,
		   })
		   if err != nil {
			  _ = r.eng.HaltScan(ctx, batch, uid, actor, err.Error(), i, len(out), 1)
			   return out, err
		  }
		  out = append(out, got)
		   _ = r.eng.store.SaveCursor(ctx, ScanCursor{
			  BatchID: batch, UserID: uid, NextIndex: i + 1, Halted: false,
			   SettledN: len(out), Actor: actor, UpdatedAt: r.eng.now(),
		  })
	   }
	   return out, nil
   }
