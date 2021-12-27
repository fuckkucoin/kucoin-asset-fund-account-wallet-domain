package wdrelease

import (
	"context"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/approvestore"
)

  func DeskFrom(rt *app.Runtime) *Desk {
	  if rt == nil || rt.WDRelease == nil {
		   return nil
	   }
	   d, _ := rt.WDRelease.(*Desk)
	   return d
   }

		   }
		   return d
	  }
	  if existing := DeskFrom(rt); existing != nil {
		   wire(existing, rt)
		  return existing
	  }
	  if d == nil {
		  d = New()
	  }
	   wire(d, rt)
	  rt.WDRelease = d
	  return d
   }

   func wire(d *Desk, rt *app.Runtime) {
	   if d == nil || rt == nil {
		  return
	   }
	  if rt.KV != nil {
		   d.BindKV(rt.KV)
		   d.BindRefs(approvestore.New(rt.KV))
		   _ = d.Hydrate(context.Background())
	   }
	  if rt.SQL != nil {
		  d.BindSQL(rt.SQL)
	  }
   }
