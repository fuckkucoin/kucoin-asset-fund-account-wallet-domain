package opsapi

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/account"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/recon"
	"account-assets.kucoin.net/funding/internal/subacct"
	"account-assets.kucoin.net/funding/internal/xferloop"
	"github.com/gin-gonic/gin"
	"strconv"
)

   func mountFundDesk(v1 *gin.RouterGroup, rt *app.Runtime) {
	   v1.GET("/ops/admin/row-fence", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   if rt == nil || rt.Ledger == nil {
			  c.JSON(404, gin.H{"error": "ledger unavailable"})
			   return
		  }
		  id := strings.TrimSpace(c.Query("transfer_id"))
		   if id == "" {
			   ev, err := rt.Ledger.RecentFenceEvents(c.Request.Context(), 50)
			   if err != nil {
				   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
				   return
			  }
			   c.JSON(200, gin.H{"rules": ledger.FenceRules(), "events": ev})
			   return
		   }
		  op := ledger.FenceOp(c.Query("op"))
		  prev, err := rt.Ledger.PreviewFence(c.Request.Context(), id, op, queryEpoch(c), c.Query("token"))
		   if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		  }
		  c.JSON(200, gin.H{"preview": prev, "rules": ledger.FenceRules()})
	   })
	   v1.GET("/ops/admin/xfer-loop", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   b := xferloop.Live()
		  if b == nil {
			  c.JSON(200, gin.H{"board": xferloop.Board{}, "cases": []xferloop.Case{}, "policies": xferloop.Policies(), "status": xferloop.StatusCatalog()})
			   return
		   }
		   c.JSON(200, gin.H{
			   "board": b.Board(), "cases": b.ListQuery(xferQuery(c)),
			   "policies": xferloop.Policies(), "status": xferloop.StatusCatalog(),
		   })
	  })
	   v1.GET("/ops/admin/triparty-classes", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   c.JSON(200, gin.H{"classes": recon.AssetClasses(), "last": recon.LastTriparty()})
	   })
	  v1.GET("/ops/admin/freeze-cascade", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  uid := ""
		  if q := strings.TrimSpace(c.Query("user_id")); q != "" && rt != nil && rt.Cfg != nil {
			   bound, err := callerid.Bind(c, q, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
			  if err != nil {
				  mw.WriteProblem(c, err)
				  return
			   }
			  uid = bound
		  }
		  var read account.FreezeRead
		   if uid != "" && rt != nil && rt.Ledger != nil {
			   got, err := account.New(rt.Ledger).FreezeReadOf(c.Request.Context(), uid, 50)
			   if err != nil {
				   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
				   return
			  }
			   read = got
		   }
		  svc := subacct.NewWithKV(nil)
		  if rt != nil {
			  svc = subacct.NewWithKV(rt.KV)
		  }
		   c.JSON(200, gin.H{
			   "user": read, "cascade": svc.FreezeAudits(c.Query("parent_id"), 50),
			  "actions": subacct.FreezeActions(),
		  })
	  })
  }

   func xferQuery(c *gin.Context) xferloop.Query {
	   q := xferloop.Query{
		   Kind: xferloop.Kind(c.Query("kind")), State: xferloop.State(c.Query("state")),
		  Asset: c.Query("asset"), UserID: c.Query("user_id"), Limit: 50,
	  }
	  switch c.Query("breached") {
	   case "1", "true":
		   on := true
		  q.Breached = &on
	   case "0", "false":
		   off := false
		  q.Breached = &off
	   }
	  return q
   }

   func queryEpoch(c *gin.Context) int64 {
	  raw := strings.TrimSpace(c.Query("epoch"))
	   if raw == "" {
		  return 0
	  }
	   n, err := strconv.ParseInt(raw, 10, 64)
	   if err != nil {
		  return 0
	   }
	  return n
   }
