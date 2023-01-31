package sweeprun

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/auditchain/ingest"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

   func (s *Service) Start(id, actor string) (Job, error) {
	   if s == nil {
		  return Job{}, ErrUnavailable
	   }
	  if _, err := validate.Actor(actor); err != nil {
		   return Job{}, ErrActor
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   cur, ok := s.jobs[strings.TrimSpace(id)]
	  if !ok {
		   return Job{}, ErrJob
	   }
	  if cur.State != StateQueued && cur.State != StatePaused {
		  return cur.Copy(), ErrState
	  }
	  cur.State = StateRunning
	   cur.Actor = actor
	   cur.UpdatedAt = s.now()
	   return cur.Copy(), nil
  }

// Fill 部分成交。超额、超预算、超 job cap 一律拒，不得悄悄截断。
   func (s *Service) Fill(ctx context.Context, id, amount, actor string) (Job, error) {
	  if s == nil {
		   return Job{}, ErrUnavailable
	   }
	   if _, err := validate.Actor(actor); err != nil {
		  return Job{}, ErrActor
	  }
	   add, err := money.Parse(amount)
	   if err != nil {
		   return Job{}, ErrAmount
	   }
	   s.mu.Lock()
	  cur, ok := s.jobs[strings.TrimSpace(id)]
	  if !ok {
		  s.mu.Unlock()
		   return Job{}, ErrJob
	   }
	  if !cur.State.CanFill() {
		  out := cur.Copy()
		   s.mu.Unlock()
		  return out, ErrState
	   }
	  if err := s.requireExecLease(ctx, cur); err != nil {
		   s.mu.Unlock()
		  return cur.Copy(), err
	  }
	   remain, err := money.ParseBook(orZ(cur.Remaining))
	   if err != nil {
		  s.mu.Unlock()
		   return Job{}, ErrAmount
	   }
	   if add.Cmp(remain) > 0 {
		  s.mu.Unlock()
		   return cur.Copy(), ErrFill
	   }
	   filled, err := money.ParseBook(orZ(cur.Filled))
	  if err != nil {
		   s.mu.Unlock()
		   return Job{}, ErrAmount
	   }
	   nextFilled, err := filled.Add(add)
	  if err != nil {
		  s.mu.Unlock()
		  return Job{}, err
	  }
	  if strings.TrimSpace(cur.JobCap) != "" {
		   cap, cerr := money.Parse(cur.JobCap)
		   if cerr != nil {
			   s.mu.Unlock()
			  return Job{}, ErrCap
		   }
		  if nextFilled.Cmp(cap) > 0 {
			  s.mu.Unlock()
			   return cur.Copy(), ErrCap
		  }
	   }
	  if err := s.checkBudgetLocked(cur.Chain, cur.Asset, add); err != nil {
		  s.mu.Unlock()
		  return cur.Copy(), err
	  }
	  target, err := money.Parse(cur.Amount)
	   if err != nil {
		   s.mu.Unlock()
		   return Job{}, ErrAmount
	   }
	  nextRemain, err := remain.Sub(add)
	  if err != nil {
		   s.mu.Unlock()
		   return Job{}, err
	   }
	   sum, err := nextFilled.Add(nextRemain)
	  if err != nil {
		  s.mu.Unlock()
		  return Job{}, err
	   }
	  if err := money.Conserve(target, sum); err != nil {
		   s.mu.Unlock()
		  return Job{}, ErrAmount
	  }
	   prev := cur.Copy()
	  cur.Filled = nextFilled.String()
	  cur.Remaining = nextRemain.String()
	   if nextRemain.IsZero() {
		  cur.State = StateFilled
	   } else {
		   cur.State = StatePartial
	   }
	  cur.Actor = actor
	   cur.UpdatedAt = s.now()
	  rec := FillRec{Seq: len(cur.Fills) + 1, Amount: add.String(), Actor: actor, At: s.now()}
	   if err := s.addUsedLocked(cur.Chain, cur.Asset, add); err != nil {
		  *cur = prev
		  s.mu.Unlock()
		  return Job{}, err
	   }
	  if s.led != nil {
		  tr, lerr := s.led.Create(ctx, ledger.CreateReq{
			  UserID: "system", Direction: ledger.HotColdSweep, Asset: cur.Asset,
			   Amount: add.Dec(), Chain: cur.Chain, IdempotencyKey: cur.ID + ":f" + itoa(rec.Seq),
		  })
		   if lerr != nil {
			  _ = s.subUsedLocked(cur.Chain, cur.Asset, add)
			  *cur = prev
			  s.mu.Unlock()
			  return Job{}, lerr
		   }
		   rec.LedgerID = tr.ID
		  cur.LedgerID = tr.ID
	  }
	  cur.Fills = append(cur.Fills, rec)
	  out := cur.Copy()
	  s.mu.Unlock()
	   ingest.Try(ctx, ingest.SweepProof(out.Chain, out.Asset, rec.Amount, out.ID+":"+itoa(rec.Seq), actor, s.now()))
	   return out, nil
   }

  func itoa(n int) string {
	   if n == 0 {
		  return "0"
	   }
	   neg := n < 0
	   if neg {
		   n = -n
	  }
	  var buf [12]byte
	   i := len(buf)
	  for n > 0 {
		  i--
		   buf[i] = byte('0' + n%10)
		   n /= 10
	  }
	   if neg {
		   i--
		  buf[i] = '-'
	   }
	  return string(buf[i:])
  }
