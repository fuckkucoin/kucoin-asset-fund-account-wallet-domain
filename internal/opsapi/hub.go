package opsapi

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/cases"
	"account-assets.kucoin.net/funding/internal/compliance"
	"account-assets.kucoin.net/funding/internal/destallow"
	"account-assets.kucoin.net/funding/internal/ha"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"account-assets.kucoin.net/funding/internal/policypack"
	"account-assets.kucoin.net/funding/internal/reviewsla"
	"account-assets.kucoin.net/funding/internal/signpipe"
	"account-assets.kucoin.net/funding/internal/sweeppol"
	"account-assets.kucoin.net/funding/internal/usage"
)

type Hub struct {
	RT *app.Runtime
	EL   *ha.Elector
	Pair *ha.Pair
	Pipe  *compliance.Pipeline
	SLA   *reviewsla.Book
	Packs *policypack.Catalog
	Inv   *hotinv.Book
	Sweep *sweeppol.Book
	Sign  *signpipe.Pipe
	Allow *destallow.Book
	Use   *usage.Store
	Cases *cases.Service
}

  func NewHub(rt *app.Runtime, el *ha.Elector) *Hub {
	   pipe := compliance.New(nil, nil)
	   hot := map[string]string{}
	  if rt != nil && rt.Cfg != nil {
		   hot = rt.Cfg.Transfer.HotMax
	   }
	   h := &Hub{
		  RT: rt, EL: el,
		   Pair: ha.NewPair("ledger", "a", "b", ha.NewMemBackend(), 0),
		   Pipe: pipe,
		  SLA:   reviewsla.New(0),
		  Packs: policypack.NewCatalog(),
		   Inv:   hotinv.FromHotMax(hot),
		  Sweep: sweeppol.FromHotMax(hot),
		   Sign:  signpipe.New(0),
		  Allow: destallow.New(),
		  Use:   pipe.Usage(),
		  Cases: pipe.Cases(),
	  }
	   if rt != nil && !rt.Mem && rt.SQL != nil {
		  kind, profile := "", ""
		  if rt.Cfg != nil {
			   kind, profile = rt.Cfg.Auth.Kind, rt.Cfg.Profile
		   }
		   destallow.BindProd(h.Allow, rt.SQL, kind, profile)
		   h.Sweep.BindSQL(rt.SQL)
	  }
	  return h
   }
