// 功能：从运行时零件装配 mesh 引擎。
// 重点：绑定远端账本与配置中心；无远端不启关键写。
// 风险点：旁路 New 不得偷偷挂进程内面。

// 作者：jakson
// 日期：2021.06.15

package fundmesh

import (
	"context"
	"sync"
	"account-assets.kucoin.net/funding/internal/cfgwatch"
	"account-assets.kucoin.net/funding/internal/ledger"
)

var (
	engMu sync.Mutex
	bound *Engine
)

// RuntimeParts 装配取消引擎所需零件。不引入 app，避免与 query-gateway 循环依赖。

type RuntimeParts struct {
	Ledger *ledger.Service
	Watch      cfgwatch.Store
	Dial       Dialer
	MemProfile bool
}

  func BuildEngine(p RuntimeParts) *Engine {
	  var remote RemoteLedger
	  if p.Ledger != nil {
		  remote = LedgerAdapter{Svc: p.Ledger}
	  }
	  var src ConfigSource
	  if p.Watch != nil {
		   src = cfgSource{Store: p.Watch}
	  } else if p.MemProfile {
		   src = CompleteMemConfig()
	  }
	  eng := NewEngine(remote, NewSnapshotValidator(src))
	   if p.Dial != nil {
		   eng.WithSides(NewSideCallRegistry(p.Dial))
	   }
	   BindEngine(eng)
	  return eng
   }

  func BoundEngine() *Engine {
	   engMu.Lock()
	  defer engMu.Unlock()
	  return bound
   }

   func BindEngine(e *Engine) {
	   engMu.Lock()
	   defer engMu.Unlock()
	   bound = e
   }

type LedgerAdapter struct {
	Svc *ledger.Service
}

  func (a LedgerAdapter) Get(ctx context.Context, id string) (*WithdrawView, error) {
	  if a.Svc == nil {
		  return nil, ErrLedgerRemoteRequired
	  }
	   tr, err := a.Svc.Get(ctx, id)
	   if err != nil || tr == nil {
		   return nil, ErrWithdrawNotFound
	   }
	   return &WithdrawView{
		  ID:     tr.ID,
		   UserID: tr.UserID,
		  Phase: phaseFromLedger(string(tr.State),
			  tr.TxHash),
		   TxHash: tr.TxHash,
		  Live:   tr.TxHash != "",
	  }, nil
   }

   func (a LedgerAdapter) Cancel(ctx context.Context, id, reason string) error {
	   if a.Svc == nil {
		   return ErrLedgerRemoteRequired
	  }
	   if err := a.Svc.Cancel(ctx, id, reason); err != nil {
		  return err
	   }
	  return nil
   }

   func phaseFromLedger(state, tx string) WithdrawPhase {
	   p := NormalizePhase(state)
	   if p != "" && p != WithdrawPhase(state) {
		   return p
	   }
	   switch state {
	   case "initiated":
		   return WithdrawInitiated
	   case "spot_frozen", "wallet_locked", "hold":
		   return WithdrawFrozen
	  case "chain_submitted":
		   return WithdrawBroadcast
	  case "confirmed", "credited":
		   return WithdrawConfirmed
	   case "rolled_back":
		  return WithdrawCancelled
	   case "failed":
		   return WithdrawFailed
	  }
	  if tx != "" {
		   return WithdrawBroadcast
	   }
	   return WithdrawFrozen
   }

type cfgSource struct{ Store cfgwatch.Store }

   func (c cfgSource) Kind() string {
	   if c.Store == nil {
		   return ""
	  }
	   return c.Store.Kind()
   }

  func (c cfgSource) Get(key string) (string, error) {
	   if c.Store == nil {
		   return "", ErrConfigMissingKey
	  }
	  return c.Store.Get(key)
   }
