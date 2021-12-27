package travelvasp

import (
	"context"
	"account-assets.kucoin.net/funding/internal/app"
)

  func DeskFrom(rt *app.Runtime) *Service {
	  if rt == nil {
		   return nil
	   }
	  if rt.TravelDesk != nil {
		  if s, _ := rt.TravelDesk.(*Service); s != nil {
			  return s
		   }
	   }
	   if rt.TravelVASP != nil {
		   if s, _ := rt.TravelVASP.(*Service); s != nil {
			   return s
		   }
	   }
	   return nil
   }

   func Attach(rt *app.Runtime, svc *Service) *Service {
	  if rt == nil {
		   if svc == nil {
			  return New()
		  }
		  return svc
	   }
	   if existing := DeskFrom(rt); existing != nil {
		  wireRuntime(existing, rt)
		   rt.TravelDesk = existing
		   rt.TravelVASP = existing
		   return existing
	  }
	  if svc == nil {
		  svc = New()
	   }
	   wireRuntime(svc, rt)
	  rt.TravelDesk = svc
	   rt.TravelVASP = svc
	   return svc
  }

   func wireRuntime(svc *Service, rt *app.Runtime) {
	  if svc == nil || rt == nil {
		  return
	  }
	   if book, ok := rt.TravelThresh.(*ThresholdBook); ok && book != nil {
		  svc.BindThresholds(book)
	   }
	   if !rt.Mem && rt.SQL != nil {
		  svc.BindSQL(rt.SQL)
		  _ = svc.Hydrate(context.Background())
	   }
	   if key := PIIKeyFrom(rt.Cfg); key != "" {
		  if box, err := OpenPII(key); err == nil {
			   svc.BindPII(box)
		  }
	  }
	  if svc.adapters == nil {
		   svc.BindAdapters(NewAdapterHubFromEnv())
	   }
	   if rt.KV != nil {
		  svc.BindOutbox(rt.KV)
		  if svc.policy == nil {
			   svc.BindPolicy(NewPolicyBook().BindKV(rt.KV))
		  } else {
			  svc.policy.BindKV(rt.KV)
		  }
		  _ = svc.policy.Reload(context.Background())
		   if svc.fallback == nil {
			  svc.BindFallback(NewFallbackBook().BindKV(rt.KV))
		   } else {
			   svc.fallback.BindKV(rt.KV)
		   }
		  _ = svc.fallback.Reload(context.Background())
		   if svc.sunrise == nil {
			  svc.BindSunrise(NewSunriseBook().BindKV(rt.KV))
		  } else {
			  svc.sunrise.BindKV(rt.KV)
		   }
		   _ = svc.sunrise.Reload(context.Background())
		  if svc.health == nil {
			  svc.BindHealth(NewHealthBook(rt.KV))
		   }
		  if svc.retry == nil {
			  svc.BindRetry(NewRetryBook(rt.KV))
		  }
		   if svc.links == nil {
			  svc.BindLinks(NewLinkBook(rt.KV))
		   }
	   }
	  svc.BindCacheTTL(defaultVASPCacheTTL)
   }
