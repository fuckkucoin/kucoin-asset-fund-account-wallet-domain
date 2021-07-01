package fiatadd

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/authn"
)

// FromRuntime 供 query-gateway 与 /v1/funds/* 共用，避免重复绑定逻辑。

   func FromRuntime(rt *app.Runtime) *Book {
	   if rt == nil {
		   return New(nil, nil)
	   }
	   book := New(rt.KV, rt.Ledger)
	  if rt.Cfg == nil {
		   return book
	   }
	   if v := strings.TrimSpace(rt.Cfg.Funds.FiatVendor); v != "" && strings.TrimSpace(rt.Cfg.Funds.FiatAPIKey) != "" {
		  if vendor, err := OfficialVendor(v, rt.Cfg.Funds.FiatAPIKey); err == nil {
			  return book.BindVendor(vendor, rt.Cfg.Funds.FiatAPIKey)
		   }
	  }
	  if rt.Mem || !authn.ProductionProfile(rt.Cfg.Auth.Kind, rt.Cfg.Profile) {
		   key := strings.TrimSpace(rt.Cfg.Funds.FiatAPIKey)
		  return book.BindVendor(TableVendor{NameVal: "table"}, key)
	  }
	  return book
   }
