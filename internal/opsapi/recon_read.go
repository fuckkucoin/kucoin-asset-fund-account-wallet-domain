package opsapi

import (
	"net/http"
	"strconv"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/ha"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/recon"
	"github.com/gin-gonic/gin"
)

// registerReconRead 挂对账只读面。写路径仍走事故升级与 ledger reconcile。

   func registerReconRead(v1 *gin.RouterGroup, rt *app.Runtime, el *ha.Elector) {
	   if v1 == nil || rt == nil {
		  return
	  }
	  _ = el
	   v1.GET("/ops/admin/recon/tickets", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  wf := reconWorkflow(rt)
		  limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
		  offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
		  var breached *bool
		   if raw := c.Query("breached"); raw == "1" || raw == "true" {
			  v := true
			   breached = &v
		  } else if raw == "0" || raw == "false" {
			  v := false
			   breached = &v
		   }
		   page := wf.ListViewsFiltered(recon.TicketFilter{
			  UserID: c.Query("user_id"),
			  State: recon.State(c.Query("state")),
			   Assignee: c.Query("assignee"),
			  Breached: breached,
			   Limit:    limit,
			   Offset:   offset,
		   }, time.Now().UTC())
		   c.JSON(http.StatusOK, gin.H{
			   "page": page,
			  "agg":  wf.Aggregate(time.Now().UTC()),
			   "clock": wf.Clock(time.Now().UTC()),
			  "host":  "ledger-transfer-freeze-reconciliation.kucoin.net",
		   })
	  })
	   v1.GET("/ops/admin/recon/tickets/:id", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  wf := reconWorkflow(rt)
		  t, err := wf.Book().Get(c.Param("id"))
		  if err != nil {
			   c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			  return
		   }
		   c.JSON(http.StatusOK, gin.H{
			  "view": wf.ViewOf(*t, time.Now().UTC()),
			  "host": "ledger-transfer-freeze-reconciliation.kucoin.net",
		   })
	   })
	  v1.GET("/ops/admin/recon/sla", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   wf := reconWorkflow(rt)
		   now := time.Now().UTC()
		  breached := true
		   page := wf.ListViewsFiltered(recon.TicketFilter{Breached: &breached, Limit: 200}, now)
		   c.JSON(http.StatusOK, gin.H{
			  "clock": wf.Clock(now),
			   "breached": page.Items,
			   "agg":      wf.Aggregate(now),
			   "host": "ledger-transfer-freeze-reconciliation.kucoin.net",
		   })
	   })
	   v1.GET("/ops/admin/recon/drift", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  wf := reconWorkflow(rt)
		  uid := c.Query("user_id")
		   var store *recon.DriftFreezeStore
		  if rt.KV != nil {
			   store = recon.NewDriftFreezeStore(rt.KV)
		  }
		   c.JSON(http.StatusOK, gin.H{
			  "drift": wf.DriftOfFull(c.Request.Context(), store, uid),
			   "host":  "ledger-transfer-freeze-reconciliation.kucoin.net",
		  })
	   })
	   v1.GET("/ops/admin/recon/escalations", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
		   minLevel, _ := strconv.Atoi(c.DefaultQuery("min_level", "0"))
		  var rows []recon.EscalationRecord
		  if rt.KV != nil {
			   rows = recon.NewEscalationStore(rt.KV).ListFiltered(c.Request.Context(), recon.EscalationFilter{
				   UserID: c.Query("user_id"), TicketID: c.Query("ticket_id"),
				  MinLevel: minLevel, Limit: limit,
			   })
		   }
		  c.JSON(http.StatusOK, gin.H{
			   "escalations": rows,
			  "host": "ledger-transfer-freeze-reconciliation.kucoin.net",
		  })
	  })
	  v1.POST("/ops/admin/recon/escalations/scan", func(c *gin.Context) {
		   actor := perm.FromRequest(c.Request, "")
		  if err := actor.Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   if el != nil {
			   if err := el.RequireLeader(); err != nil {
				  c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
				   return
			  }
		   }
		   wf := reconWorkflow(rt)
		   var store *recon.EscalationStore
		   if rt.KV != nil {
			  store = recon.NewEscalationStore(rt.KV)
		  }
		  recs, err := wf.EscalatePersistLeveled(c.Request.Context(), time.Now().UTC(), actor.ID, store)
		   if err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "escalations": recs})
			   return
		   }
		   c.JSON(http.StatusOK, gin.H{
			   "escalations": recs,
			  "count":       len(recs),
			   "host": "ledger-transfer-freeze-reconciliation.kucoin.net",
		  })
	   })
	  v1.GET("/ops/admin/recon/sla/board", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
		   wf := reconWorkflow(rt)
		  c.JSON(http.StatusOK, wf.Board(time.Now().UTC(), limit))
	  })
	   v1.GET("/ops/admin/recon/drift/frozen", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
		   var rows []recon.DriftFreeze
		  if rt.KV != nil {
			   rows = recon.NewDriftFreezeStore(rt.KV).ListFrozen(c.Request.Context(), limit)
		  }
		  c.JSON(http.StatusOK, gin.H{
			  "frozen": rows,
			   "host": "ledger-transfer-freeze-reconciliation.kucoin.net",
		   })
	  })
	  v1.GET("/ops/admin/recon/incidents", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
		  var engaged *bool
		   if raw := c.Query("engaged"); raw == "1" || raw == "true" {
			  v := true
			  engaged = &v
		   } else if raw == "0" || raw == "false" {
			  v := false
			  engaged = &v
		  }
		   var rows []recon.IncidentRecord
		   if rt.KV != nil {
			   rows = recon.NewIncidentStore(rt.KV).List(c.Request.Context(), recon.IncidentQuery{
				  UserID: c.Query("user_id"), TicketID: c.Query("ticket_id"),
				  Engaged: engaged, Scope: c.Query("scope"), Limit: limit,
			   })
		   }
		   c.JSON(http.StatusOK, gin.H{
			   "incidents": rows,
			   "host":      "ledger-transfer-freeze-reconciliation.kucoin.net",
		  })
	  })
	   v1.GET("/ops/admin/recon/incidents/board", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
		  board := recon.IncidentBoard{Host: "ledger-transfer-freeze-reconciliation.kucoin.net", ByScope: map[string]int{}, ByLevel: map[string]int{}}
		   if rt.KV != nil {
			  board = recon.NewIncidentStore(rt.KV).Board(c.Request.Context(), limit)
		   }
		  c.JSON(http.StatusOK, board)
	  })
  }
