package ledshard

import (
	"fmt"
	"time"
	"account-assets.kucoin.net/funding/pkg/money"
)

type delta struct {
	shard, subject, asset string
	dAvail, cAvail   string
	dFrozen, cFrozen string
	kind             StepKind
	voucher string
	memo string
}

// applyDeltas 先在草稿余额上轧完所有腿，全部通过才落账。任一条不够则一张分录都不写。
   func (e *Engine) applyDeltas(it Intent, ds ...delta) error {
	  scratch := map[string]Balance{}
	   posts := make([]Posting, 0, len(ds))
	  for _, d := range ds {
		   if d.asset == "" {
			   d.asset = it.Asset
		  }
		  key := balKey(d.shard,
			   d.subject, d.asset)
		   b, ok := scratch[key]
		   if !ok {
			  b = e.mem.balance(d.shard, d.subject, d.asset)
		   }
		  next, err := foldBalance(b, d.dAvail, d.cAvail, d.dFrozen, d.cFrozen)
		  if err != nil {
			  return err
		   }
		  scratch[key] = next
		  posts = append(posts, Posting{
			  ID: e.id("pst"), IntentID: it.ID, VoucherID: d.voucher, Kind: d.kind,
			  ShardID: d.shard, Subject: d.subject, Asset: d.asset,
			  DebitAvail: nz(d.dAvail), CreditAvail: nz(d.cAvail),
			   DebitFrozen: nz(d.dFrozen), CreditFrozen: nz(d.cFrozen),
			  Memo: d.memo, At: e.now(),
		  })
	   }
	   for _, b := range scratch {
		  e.mem.saveBalance(b)
	   }
	   for _, p := range posts {
		  e.mem.addPosting(p)
	  }
	   return nil
   }

  func foldBalance(b Balance, dAvail, cAvail, dFrozen, cFrozen string) (Balance, error) {
	  avail, err := money.ParseBook(nzZero(b.Available))
	  if err != nil {
		   return b, err
	   }
	   frozen, err := money.ParseBook(nzZero(b.Frozen))
	  if err != nil {
		   return b, err
	  }
	   avail, err = addSub(avail, cAvail, dAvail)
	   if err != nil {
		  return b, fmt.Errorf("%w: available", ErrInsufficient)
	  }
	   frozen, err = addSub(frozen, cFrozen, dFrozen)
	   if err != nil {
		  return b, fmt.Errorf("%w: frozen", ErrInsufficient)
	  }
	   b.Available = avail.String()
	  b.Frozen = frozen.String()
	  return b, nil
  }

   func addSub(cur money.Amount, credit, debit string) (money.Amount, error) {
	  if credit != "" && credit != "0" {
		  c, err := money.Parse(credit)
		  if err != nil {
			  return cur, err
		  }
		   cur, err = cur.Add(c)
		  if err != nil {
			  return cur, err
		  }
	   }
	   if debit != "" && debit != "0" {
		  d, err := money.Parse(debit)
		   if err != nil {
			  return cur, err
		   }
		  cur, err = cur.Sub(d)
		  if err != nil {
			  return cur, err
		  }
	  }
	   return cur, nil
   }

   func nz(s string) string {
	   if s == "" {
		   return "0"
	  }
	  return s
   }

  func nzZero(s string) string {
	   if s == "" {
		  return "0"
	  }
	   return s
  }

   func (e *Engine) stamp(it *Intent) {
	   it.UpdatedAt = e.now()
	   e.mem.saveIntent(*it)
   }

   func (e *Engine) pushStep(it *Intent, kind StepKind, shard, detail string, froze, credited bool) (Step, error) {
	   from, to, err := admitStep(kind, froze, credited)
	   if err != nil {
		  return Step{}, err
	  }
	  st := Step{
		   ID: e.id("stp"), IntentID: it.ID, Seq: len(e.mem.listSteps(it.ID)) + 1,
		   Kind: kind, State: "acked", ShardID: shard, Detail: detail, At: e.now(),
	  }
	   if from != "" {
		   st.LedgerFrom = string(from)
		  st.LedgerTo = string(to)
	  }
	   e.mem.addStep(st)
	   return st, nil
  }

   func (e *Engine) hint(kind string, it Intent, voucher string) {
	   e.mem.addHint(Hint{
		  ID: e.id("hnt"), Kind: kind, IntentID: it.ID, VoucherID: voucher,
		   SrcShard: it.SrcShard, DstShard: it.DstShard, Asset: it.Asset, Amount: it.Amount,
		   At: e.now(),
	  })
	   metricsHint(kind)
   }

   func (e *Engine) now() time.Time {
	   if e.clock != nil {
		   return e.clock().UTC()
	  }
	   return time.Now().UTC()
   }
