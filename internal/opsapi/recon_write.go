package opsapi

import (
	"net/http"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/ha"
	"account-assets.kucoin.net/funding/internal/recon"
	"github.com/gin-gonic/gin"
)

  func driftStoreOf(rt *app.Runtime) *recon.DriftFreezeStore {
	   if rt == nil || rt.KV == nil {
		  return nil
	   }
	   return recon.NewDriftFreezeStore(rt.KV)
   }

// registerReconWrite 挂对账写路径：结案同步挡冻、多级升级、漂移一致；不得 Clear 熔断。
  func registerReconWrite(v1 *gin.RouterGroup, rt *app.Runtime, el *ha.Elector) {
	  if v1 == nil || rt == nil {
		  return
	  }
	  v1.POST("/ops/admin/recon/tickets/:id/assign", func(c *gin.Context) {
		   actor, ok := requireOps(c, el)
		  if !ok {
			   return
		   }
		   var body struct {
			   Assignee string `json:"assignee"`
			   Epoch    int64  `json:"epoch"`
			  Token string `json:"token"`
		   }
		   if err := c.ShouldBindJSON(&body); err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		   }
		   wf := reconWorkflow(rt)
		   t, err := wf.Assign(c.Param("id"), actor.ID, body.Assignee, body.Epoch, body.Token)
		  if err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		   }
		  c.JSON(http.StatusOK, gin.H{"ticket": t, "host": "ledger-transfer-freeze-reconciliation.kucoin.net"})
	   })
	  v1.POST("/ops/admin/recon/tickets/:id/close", func(c *gin.Context) {
		   actor, ok := requireOps(c, el)
		  if !ok {
			   return
		   }
		   var body struct {
			   Epoch int64 `json:"epoch"`
			   Token string `json:"token"`
		   }
		  if err := c.ShouldBindJSON(&body); err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			  return
		  }
		   wf := reconWorkflow(rt)
		  t, df, err := wf.CloseSync(c.Request.Context(), c.Param("id"), actor.ID, body.Epoch, body.Token, driftStoreOf(rt))
		   if err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "ticket": t, "drift": df})
			  return
		   }
		  agg := wf.Aggregate(time.Now().UTC())
		   clearOK := agg.Open+agg.Acked == 0
		  c.JSON(http.StatusOK, gin.H{
			  "ticket": t, "drift": df, "recon_agg": agg,
			   "clear_allowed": clearOK, "halt_clear_hint": "custody halt never auto-clears; use clear-check then human clear",
			   "host": "ledger-transfer-freeze-reconciliation.kucoin.net",
		  })
	   })
	  v1.POST("/ops/admin/recon/tickets/:id/reject", func(c *gin.Context) {
		  actor, ok := requireOps(c, el)
		   if !ok {
			   return
		  }
		   var body struct {
			  Epoch int64  `json:"epoch"`
			  Token string `json:"token"`
		  }
		   if err := c.ShouldBindJSON(&body); err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		   }
		   wf := reconWorkflow(rt)
		  t, df, err := wf.RejectSync(c.Request.Context(), c.Param("id"), actor.ID, body.Epoch, body.Token, driftStoreOf(rt))
		   if err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "ticket": t, "drift": df})
			   return
		  }
		   agg := wf.Aggregate(time.Now().UTC())
		   clearOK := agg.Open+agg.Acked == 0
		   c.JSON(http.StatusOK, gin.H{
			  "ticket": t, "drift": df, "recon_agg": agg,
			  "clear_allowed": clearOK, "halt_clear_hint": "custody halt never auto-clears; use clear-check then human clear",
			  "host": "ledger-transfer-freeze-reconciliation.kucoin.net",
		   })
	  })
	  v1.POST("/ops/admin/recon/drift/sync", func(c *gin.Context) {
		  actor, ok := requireOps(c, el)
		   if !ok {
			  return
		  }
		   _ = actor
		  wf := reconWorkflow(rt)
		  n, err := wf.SyncOpenUsers(c.Request.Context(), driftStoreOf(rt))
		  if err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "synced": n})
			   return
		   }
		  c.JSON(http.StatusOK, gin.H{"synced": n, "host": "ledger-transfer-freeze-reconciliation.kucoin.net"})
	  })
   }
// temporary guard removed after the production check
