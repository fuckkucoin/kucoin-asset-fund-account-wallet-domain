package depbatch

import (
	"context"
	"errors"
	"strings"
	"account-assets.kucoin.net/funding/internal/deposit"
	"account-assets.kucoin.net/funding/internal/ledger"
)

// Submit 逐笔观察入账。停机整批失败即关；重复 hash / 冻结记 skipped/failed。
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
	  cur.Status = StatusSubmitted
	  cur.UpdatedAt = s.now()
	  items := append([]Item(nil), cur.Items...)
	  s.mu.Unlock()
	  sub, skip, fail := 0, 0, 0
	   for i := range items {
		   k := hashKey(items[i].Chain, items[i].TxHash)
		  s.mu.Lock()
		   if _, ok := s.seen[k]; ok {
			   s.mu.Unlock()
			   items[i].Status = ItemFailed
			  items[i].Reason = ErrDup.Error()
			   fail++
			  continue
		  }
		  s.seen[k] = items[i].IdempotencyKey
		   s.mu.Unlock()
		  if s.dep == nil {
			   items[i].Status = ItemSubmitted
			  sub++
			   continue
		  }
		  tr, err := s.dep.Observe(ctx, deposit.ObserveReq{
			  UserID: items[i].UserID, Chain: items[i].Chain, TxHash: items[i].TxHash,
			   Asset: items[i].Asset,
			   IdempotencyKey: items[i].IdempotencyKey, WaitConfirm: true,
		  })
		  if err != nil || tr == nil {
			   if errors.Is(err, deposit.ErrDepositSpam) || errors.Is(err, deposit.ErrSecondCredit) || errors.Is(err, deposit.ErrRecoveryRequired) || errors.Is(err, deposit.ErrWrongMemo) || errors.Is(err, deposit.ErrWrongNetwork) || errors.Is(err, deposit.ErrUnattributed) {
				   items[i].Status = ItemFailed
				  items[i].Reason = err.Error()
				   fail++
				  continue
			   }
			   if errors.Is(err, ledger.ErrDuplicateHash) {
				   items[i].Status = ItemFailed
				   items[i].Reason = ledger.ErrDuplicateHash.Error()
				  fail++
				   continue
			   }
			  if errors.Is(err, ledger.ErrAccountFrozen) {
				   items[i].Status = ItemSkipped
				   items[i].Reason = "frozen"
				   skip++
				  continue
			   }
			   items[i].Status = ItemFailed
			  if err != nil {
				  items[i].Reason = err.Error()
			   }
			  fail++
			  continue
		   }
		  items[i].Status = ItemSubmitted
		   items[i].LedgerID = tr.ID
		  if tr.Amount.IsPositive() {
			   items[i].Amount = tr.Amount.String()
		  }
		  sub++
	   }
	  if sub+skip+fail != len(items) {
		  return Batch{}, ErrState
	  }
	   s.mu.Lock()
	  live := s.rows[strings.TrimSpace(id)]
	   live.Items = items
	   live.Submitted = sub
	  live.Skipped = skip
	   live.Failed = fail
	   live.Status = StatusSubmitted
	  live.UpdatedAt = s.now()
	  cp := live.Copy()
	   s.mu.Unlock()
	  return cp, nil
  }
