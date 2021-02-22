package app

import (
	"account-assets.kucoin.net/funding/internal/casestore"
	"account-assets.kucoin.net/funding/internal/dualctl"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/polystore"
	"account-assets.kucoin.net/funding/internal/recon"
)

// Durable 一等实体的进程内装配：Open 绑定 SQL，OpenMem 使用内存与 KV。
type Durable struct {
	Recon *recon.Book
	Dual  *dualctl.Book
	Cases *casestore.Repo
	Policy *polystore.Store
}

  func attachDurable(rt *Runtime) {
	   if rt == nil {
		  return
	  }
	   var db *persist.SQLDB
	   if !rt.Mem {
		  db = rt.SQL
	   }
	   kv := rt.KV
	   rt.Durable = Durable{
		   Recon: recon.New().BindSQL(db),
		  Dual:   dualctl.New(0).BindSQL(db),
		   Cases:  casestore.New(kv).BindSQL(db),
		   Policy: polystore.New(kv).BindSQL(db),
	  }
   }
