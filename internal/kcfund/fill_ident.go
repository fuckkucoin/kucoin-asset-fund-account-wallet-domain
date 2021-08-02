package kcfund

import (
	   "sync"
	"account-assets.kucoin.net/funding/internal/cvfund"
)

var (
	fillIdentOnce sync.Once
	fillIdentSvc *cvfund.FillIdentity
)

  func (a *API) fillIdent() *cvfund.FillIdentity {
	     fillIdentOnce.Do(func() {
	    	  var secret string
	   	  var cns []string
		  require := false
	  	  if a != nil && a.rt != nil && a.rt.Cfg != nil {
	    		  secret = a.rt.Cfg.ConvertFill.HMACSecret
	    		  cns = a.rt.Cfg.ConvertFill.MTLSCNs
	   		  require = a.rt.Cfg.ConvertFill.RequireIdent
		  }
		  fillIdentSvc = cvfund.NewFillIdentity(secret, cns)
		  if require && fillIdentSvc != nil {
			  fillIdentSvc.RequireIdent(true)
		  }
	  })
	  return fillIdentSvc
  }
