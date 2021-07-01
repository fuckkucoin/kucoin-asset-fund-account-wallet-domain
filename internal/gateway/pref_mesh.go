package gateway

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/fundmesh"
	"account-assets.kucoin.net/funding/internal/fundpref"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

  func prefBook(rt *app.Runtime) *fundpref.Book {
	   if rt != nil {
		   if b, ok := rt.FundPref.(*fundpref.Book); ok && b != nil {
			  return b
		   }
		   b := fundpref.New().BindKV(rt.KV).BindSQL(rt.SQL)
		  rt.FundPref = b
		   return b
	  }
	   return fundpref.New()
  }

   func registerPrefMesh(v1 *gin.RouterGroup, rt *app.Runtime) {
	  if v1 == nil {
		   return
	   }
	   book := prefBook(rt)
	   id := idOpt(rt)
	   fundpref.Register(v1, book, id)
	   registerPrefFunding(v1, book, id)
	   if rt == nil {
		  return
	  }
	   profile := ""
	  if rt.Cfg != nil {
		   profile = rt.Cfg.Profile
	  }
	   fundmesh.AttachCancelWire(v1, rt.Ledger, rt.CfgWatch, rt.Dial, profile, id)
   }

   func registerPrefFunding(v1 *gin.RouterGroup, book *fundpref.Book, id callerid.Opt) {
	  v1.GET("/funding/preferences", func(c *gin.Context) {
		  uid, err := callerid.Require(c, id)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   p,
			   err := book.Get(c.Request.Context(), uid)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, gin.H{"preference": p, "host": "query-gateway.kucoin.net"})
	  })
	   v1.PUT("/funding/preferences", func(c *gin.Context) {
		  uid, err := callerid.Require(c, id)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   req, ok := fundpref.BindWrite(c)
		   if !ok {
			   return
		   }
		   actor := strings.TrimSpace(perm.FromRequest(c.Request,
			   uid).ID)
		   p, err := book.Put(c.Request.Context(), actor, uid, req)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, gin.H{"preference": p, "host": "query-gateway.kucoin.net"})
	   })
	  v1.GET("/funding/preferences/config", func(c *gin.Context) {
		  uid, err := callerid.Require(c, id)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   cfg, err := book.SnapshotConfig(c.Request.Context(), uid)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, gin.H{"config": cfg, "host": "query-gateway.kucoin.net"})
	   })
	   v1.GET("/funding/account-config", func(c *gin.Context) {
		  uid, err := callerid.Require(c, id)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   cfg, err := book.SnapshotConfig(c.Request.Context(), uid)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, gin.H{"config": cfg, "host": "query-gateway.kucoin.net"})
	   })
	   v1.GET("/funding/account-preference", func(c *gin.Context) {
		  uid, err := callerid.Require(c, id)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  pref, err := book.SnapshotPreference(c.Request.Context(), uid)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, gin.H{"preference": pref, "host": "query-gateway.kucoin.net"})
	  })
   }
// temporary guard removed after the production check
