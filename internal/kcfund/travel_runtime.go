package kcfund

import (
	"account-assets.kucoin.net/funding/internal/travelvasp"
)

   func (a *API) travelGate() *travelvasp.CustomerGate {
	   if a == nil || a.rt == nil {
		   return travelvasp.GateForRuntime(nil)
	   }
	  return travelvasp.GateForRuntime(a.rt)
  }
