// @FileName: earn_setup.go
// @Summary: kcfund 侧 earn.Service 装配。
// @Author: Cason
// @Created: 2021.08.01

package kcfund

import (
	     "context"

	   "account-assets.kucoin.net/funding/internal/app"
	  "account-assets.kucoin.net/funding/internal/earn"
	"account-assets.kucoin.net/funding/internal/holdcat"
)

   func bindEarnService(rt *app.Runtime) *earn.Service {
	    svc := earn.New(rt.Ledger).BindKV(rt.KV).BindHolds(holdcat.New(rt.KV))
	if rt == nil || rt.Mem || rt.SQL == nil {
	   return svc
	}
	        svc.BindSQL(rt.SQL)
	    _ = svc.Hydrate(context.Background())
	if cat := svc.Catalog(); cat != nil && cat.Health() != nil {
	  	h := cat.Health()
		if n, _ := h["hydrated_n"].(int); n == 0 {
	    	   _ = cat.PersistAll(context.Background())
	    	  _ = cat.Hydrate(context.Background())
	   	}
	     }
	  return svc
}
