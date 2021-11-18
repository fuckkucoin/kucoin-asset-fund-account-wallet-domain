package signpipegate

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/authn"
	"account-assets.kucoin.net/funding/internal/persist"
)

// BindStore 必须 SQL（signpipe_inflight_leases）；禁止静默 Mem。
   func BindStore(rt *app.Runtime) (Store, error) {
	  mem := rt == nil || rt.Mem || rt.SQL == nil || rt.SQL.DB == nil
	   kind, profile, storage := "", "", ""
	  if rt != nil && rt.Cfg != nil {
		  kind, profile, storage = rt.Cfg.Auth.Kind, rt.Cfg.Profile, rt.Cfg.Storage
	   }
	   if err := persist.RefuseMemRegister(authn.ProductionProfile(kind, profile), storage, mem); err != nil {
		  return nil, err
	   }
	  if mem {
		  return NewMemStore(), nil
	  }
	  return NewSQLStoreDB(rt.SQL.DB), nil
   }
