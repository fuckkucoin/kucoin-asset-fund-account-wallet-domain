// 重点：Submit 只在新键路径调用；replay 不得再打远端。
// 风险点：远端失败禁止改走进程内账本。

// Author: Cason
// Date: 2021.01.05

package fundmesh

import (
	"context"
	"errors"
	"strings"
	"sync"
	"account-assets.kucoin.net/funding/internal/ledger"
	"github.com/shopspring/decimal"
)

// RemoteOutbound 远端出站扣减。

type RemoteOutbound interface {
	Submit(ctx context.Context, in OutboundIntent) (*WithdrawView, error)
	DebitCalls() int
}

   func (m *MemRemoteLedger) Submit(_ context.Context, in OutboundIntent) (*WithdrawView, error) {
	  if m == nil {
		   return nil, ErrLedgerRemoteRequired
	  }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   m.calls++
	   if m.debitByIdem == nil {
		  m.debitByIdem = map[string]string{}
	  }
	  if id, ok := m.debitByIdem[in.IdempotencyKey]; ok {
		  if v := m.rows[id]; v != nil {
			  cp := *v
			  return &cp, ErrDoubleDebitForbidden
		  }
		   return nil, ErrDoubleDebitForbidden
	   }
	  if m.fail != nil {
		   return nil, m.fail
	   }
	   id := strings.TrimSpace(in.WithdrawID)
	  if id == "" {
		   id = "wd-" + in.IdempotencyKey
		  if len(id) > 64 {
			  id = id[:64]
		   }
	  }
	   v := &WithdrawView{
		  ID: id,
		  UserID: in.UserID,
		  Phase: WithdrawFrozen,
		   Live: false,
	   }
	   m.rows[id] = v
	  m.debitByIdem[in.IdempotencyKey] = id
	  m.debits++
	   cp := *v
	   return &cp, nil
   }

   func (m *MemRemoteLedger) DebitCalls() int {
	  if m == nil {
		   return 0
	   }
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  return m.debits
  }

  func (a LedgerAdapter) Submit(ctx context.Context, in OutboundIntent) (*WithdrawView, error) {
	   if a.Svc == nil {
		   return nil, ErrLedgerRemoteRequired
	   }
	   amt, err := decimal.NewFromString(in.Amount)
	   if err != nil {
		   return nil, ErrOutboundPayload
	  }
	   tr, err := a.Svc.Create(ctx, ledger.CreateReq{
		   UserID: in.UserID,
		  Direction: ledger.WalletWithdraw,
		  Asset: in.Asset,
		   Amount:    amt,
		  Chain:     in.Chain,
		  ToAddress: in.Dest,
		   Memo:           in.Memo,
		   IdempotencyKey: in.IdempotencyKey,
	  })
	   if err != nil {
		   if errors.Is(err, ledger.ErrIdempotency) || errors.Is(err, ledger.ErrIdempotencyConflict) {
			  return nil, ErrOutboundIdemConflict
		  }
		   return nil, err
	   }
	   if tr == nil {
		   return nil, ErrLedgerRemoteFailed
	   }
	  return &WithdrawView{
		  ID: tr.ID,
		  UserID: tr.UserID,
		  Phase:  phaseFromLedger(string(tr.State), tr.TxHash),
		  TxHash: tr.TxHash,
		   Live:   tr.TxHash != "",
	  }, nil
   }

   func (a LedgerAdapter) DebitCalls() int { return 0 }

type countingOutbound struct {
	inner RemoteOutbound
	n int
	mu sync.Mutex
}

   func (c *countingOutbound) Submit(ctx context.Context, in OutboundIntent) (*WithdrawView, error) {
	  c.mu.Lock()
	  c.n++
	  c.mu.Unlock()
	   if c.inner == nil {
		  return nil, ErrLedgerRemoteRequired
	  }
	   return c.inner.Submit(ctx, in)
   }

   func (c *countingOutbound) DebitCalls() int {
	  c.mu.Lock()
	  defer c.mu.Unlock()
	  return c.n
   }

  func asRemoteOutbound(r RemoteLedger) RemoteOutbound {
	  if r == nil {
		   return nil
	   }
	   if o, ok := r.(RemoteOutbound); ok {
		  return o
	  }
	   return nil
   }
