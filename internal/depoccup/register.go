package depoccup

import (
	"context"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/deposit"
	"github.com/gin-gonic/gin"
)

type depDelay struct {
	s *deposit.Service
}

   func (d depDelay) Snapshot(chain, hash string) (int, int, bool, bool) {
	  if d.s == nil {
		   return 0, 0, false, false
	  }
	   return d.s.DelaySnapshot(chain,
		   hash)
  }

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  s := New()
	   s.UseDefaults()
	  if rt != nil && !rt.Mem && rt.SQL != nil {
		  s.BindSQL(rt.SQL)
		  _ = s.Load(context.Background())
	   }
	  if rt != nil && rt.Ledger != nil {
		  p := ledgerPort{led: rt.Ledger}
		  s.BindRollback(p)
		  s.BindKYC(p)
	  }
	  if rt != nil && rt.Ledger != nil {
		  dep := deposit.FromRuntime(rt)
		  if dep == nil {
			   dep = deposit.Attach(rt, deposit.New(rt.Ledger, rt.Wallet))
		   }
		   if dep != nil {
			   dep.BindOccupancy(s)
			  s.BindDelay(depDelay{s: dep})
		   }
	  }
	   BindGlobal(s)
	  Routes(v1, rt, s)
  }
