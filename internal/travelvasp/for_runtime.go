package travelvasp

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/authn"
)

// ThresholdBookFrom 导出门槛表；与 RegisterThresholds 共享指针。

  func ThresholdBookFrom(rt *app.Runtime) *ThresholdBook {
	   return thresholdBookFrom(rt)
  }

// GateForRuntime 构造对客旅行规则闸；prod 剖面下 outbound 须 queued 以后才允许出金。

   func GateForRuntime(rt *app.Runtime) *CustomerGate {
	  book := thresholdBookFrom(rt)
	   vasp := serviceFrom(rt)
	   origin,
		  peer := OriginCode(), PeerCode()
	   if rt != nil && rt.Cfg != nil {
		  if v := strings.TrimSpace(rt.Cfg.Travel.OriginVASP); v != "" {
			  origin = strings.ToUpper(v)
		   }
		  if v := strings.TrimSpace(rt.Cfg.Travel.PeerVASP); v != "" {
			  peer = strings.ToUpper(v)
		  }
	   }
	  strict := false
	  if rt != nil && rt.Cfg != nil {
		   strict = authn.ProductionProfile(rt.Cfg.Auth.Kind, rt.Cfg.Profile)
	   }
	  bind := NewBindStore(nil)
	  if rt != nil && rt.KV != nil {
		   bind = NewBindStore(rt.KV)
	  }
	  return NewCustomerGate(book,
		   vasp, bind, origin, peer, strict)
  }

  func serviceFrom(rt *app.Runtime) *Service {
	  if s := DeskFrom(rt); s != nil {
		   return s
	   }
	   s := New()
	   if rt != nil {
		   rt.TravelVASP = s
		   rt.TravelDesk = s
	  }
	   return s
  }
