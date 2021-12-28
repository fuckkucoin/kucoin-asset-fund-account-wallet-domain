package wdbatch

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/withdraw"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/shopspring/decimal"
)

// Submit 逐笔过闸。停机整批失败即关；单笔冻结/不足记 skipped/failed，不中断其余。
// 结束后 requested = submitted + skipped + failed。
   func (s *Service) Submit(ctx context.Context, id, actor string) (Batch, error) {
	   if s == nil {
		  return Batch{}, ErrUnavailable
	  }
	   if _, err := requireActor(actor); err != nil {
		   return Batch{}, err
	   }
	   s.mu.Lock()
	   if s.halt {
		   s.mu.Unlock()
		  return Batch{}, ErrHalt
	   }
	  cur, ok := s.rows[strings.TrimSpace(id)]
	  if !ok {
		   s.mu.Unlock()
		  return Batch{}, ErrBatch
	   }
	  if cur.Status != StatusOpen {
		   cp := cur.Copy()
		  s.mu.Unlock()
		  return cp, ErrState
	   }
	   items := append([]Item(nil), cur.Items...)
	  s.mu.Unlock()
	   sub, skip, fail := money.Zero(), money.Zero(), money.Zero()
	  for i := range items {
		  amt, err := money.Parse(items[i].Amount)
		  if err != nil {
			   return Batch{}, ErrAmount
		  }
		  if s.led != nil {
			   if err := s.led.CheckUser(ctx, items[i].UserID); err != nil {
				   items[i].Status = ItemSkipped
				  items[i].Reason = "frozen"
				  next, aerr := skip.Add(amt)
				  if aerr != nil {
					   return Batch{}, aerr
				  }
				  skip = next
				  continue
			  }
		  }
		  if s.wd != nil {
			  tr, werr := s.wd.Request(ctx, withdraw.Request{
				   UserID: items[i].UserID, Asset: items[i].Asset,
				   Amount: decimal.RequireFromString(items[i].Amount),
				  Chain: items[i].Chain, ToAddress: items[i].ToAddress,
				   IdempotencyKey: items[i].IdempotencyKey,
			  })
			   if werr != nil || tr == nil {
				  items[i].Status = ItemFailed
				  if werr != nil {
					   items[i].Reason = werr.Error()
				  } else {
					  items[i].Reason = "withdraw"
				   }
				  next, aerr := fail.Add(amt)
				  if aerr != nil {
					   return Batch{}, aerr
				   }
				   fail = next
				  continue
			  }
			   items[i].Status = ItemSubmitted
			  items[i].LedgerID = tr.ID
			  _ = s.IndexSigned(cur.ID, tr.ID)
			   next, aerr := sub.Add(amt)
			  if aerr != nil {
				   return Batch{}, aerr
			   }
			   sub = next
			  continue
		  }
		  if s.led != nil {
			  tr, lerr := s.led.Create(ctx, ledger.CreateReq{
				  UserID: items[i].UserID, Direction: ledger.WalletWithdraw,
				   Asset: items[i].Asset, Amount: decimal.RequireFromString(items[i].Amount),
				   Chain:          items[i].Chain,
				   ToAddress:      items[i].ToAddress,
				   IdempotencyKey: items[i].IdempotencyKey,
			  })
			   if lerr != nil || tr == nil {
				   items[i].Status = ItemFailed
				   if lerr != nil {
					   items[i].Reason = lerr.Error()
				   }
				   next, aerr := fail.Add(amt)
				  if aerr != nil {
					   return Batch{}, aerr
				  }
				   fail = next
				   continue
			   }
			  items[i].Status = ItemSubmitted
			   items[i].LedgerID = tr.ID
			   _ = s.IndexSigned(cur.ID,
				   tr.ID)
			   next, aerr := sub.Add(amt)
			   if aerr != nil {
				   return Batch{}, aerr
			   }
			  sub = next
			  continue
		   }
		   items[i].Status = ItemFailed
		   items[i].Reason = "withdraw conserve gate unbound"
		  next, aerr := fail.Add(amt)
		   if aerr != nil {
			   return Batch{}, aerr
		  }
		  fail = next
	   }
	   right, err := sub.Add(skip)
	   if err != nil {
		  return Batch{}, err
	   }
	   right, err = right.Add(fail)
	   if err != nil {
		   return Batch{}, err
	  }
	  s.mu.Lock()
	   live := s.rows[strings.TrimSpace(id)]
	  reqAmt,
		  err := money.ParseBook(orZ(live.Requested))
	   if err != nil {
		  s.mu.Unlock()
		  return Batch{}, err
	   }
	   if err := money.Conserve(reqAmt, right); err != nil {
		  s.mu.Unlock()
		  return Batch{}, ErrConserve
	  }
	   live.Items = items
	   live.Submitted = sub.String()
	   live.Skipped = skip.String()
	   live.Failed = fail.String()
	   live.Status = StatusSubmitted
	  live.UpdatedAt = s.now()
	   cp := live.Copy()
	   s.mu.Unlock()
	   return cp, nil
  }

   func orZ(s string) string {
	   if strings.TrimSpace(s) == "" {
		  return "0"
	   }
	  return s
   }
