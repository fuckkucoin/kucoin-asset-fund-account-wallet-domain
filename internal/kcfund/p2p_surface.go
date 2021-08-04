// @FileName: p2p_surface.go
// @Summary: P2P 资金占用公开 REST：holds / details / flow / merchant。
// prod 先 fundmesh → p2p-funding；回落本进程 p2pgate.Funds。不含盘口写路径。
// @Author: EliotVoss
// @Created: 2021.08.02

package kcfund

import (
	"net/http"
	"strings"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/p2pgate"
	"github.com/gin-gonic/gin"
)

  func (a *API) registerP2POccupancy(g *gin.RouterGroup) {
	    g.GET("/v1/p2p/holds", a.p2pHolds)
	  g.GET("/v1/p2p/holds/:id", a.p2pHoldDetail)
	  g.GET("/v1/p2p/summary", a.p2pSummary)
	      g.GET("/v1/p2p/details", a.p2pDetails)
	      g.GET("/v1/p2p/flow", a.p2pFlow)
	   g.GET("/v1/p2p/merchant", a.p2pMerchant)
	      g.GET("/v1/p2p/merchant/summary", a.p2pMerchantSummary)
	       g.GET("/v1/p2p/orders", a.p2pOrders)
	   g.GET("/v1/p2p/orders/:id", a.p2pOrder)
	   g.GET("/v1/p2p/integrity", a.p2pIntegrity)
	      g.GET("/v1/p2p/aggregate", a.p2pAggregate)
	   g.GET("/v1/p2p/windows", a.p2pWindows)
	  g.GET("/v1/p2p/cooling", a.p2pCooling)
	      g.GET("/v1/p2p/catalog", a.p2pCatalog)
	      g.GET("/v1/p2p/snapshot", a.p2pSnapshot)
  }

   func (a *API) p2pFunds() *p2pgate.Funds {
	   if a == nil || a.rt == nil {
	  	  return p2pgate.NewFunds(nil, nil)
	    }
	     return p2pgate.NewFunds(a.rt.Ledger,
		   holdcat.New(a.rt.KV))
  }

  func (a *API) p2pQuery(c *gin.Context, uid string) (p2pgate.OccupancyQuery, bool) {
	  q, err := p2pgate.ParseOccupancyQuery(uid, c.Request.URL.Query())
	      if err != nil {
		   fail(c, 400, err.Error())
		  return p2pgate.OccupancyQuery{}, false
	   }
	   return q, true
   }

  func (a *API) p2pHolds(c *gin.Context) {
	  if a.forwardMeshP2P(c, http.MethodGet, "/api/v1/p2p/holds", nil) {
	  	  return
	      }
	    uid, okU := a.requireUser(c)
	      if !okU {
	    	  return
	  }
	    rows,
	   	   err := a.p2pFunds().ListOpen(c.Request.Context(), uid)
	   if err != nil {
		   fail(c, 400, err.Error())
		   return
	  }
	   ok(c, gin.H{"items": rows, "userId": uid})
   }

   func (a *API) p2pHoldDetail(c *gin.Context) {
	  if a.forwardMeshP2P(c, http.MethodGet, "/api/v1/p2p/holds/"+c.Param("id"), nil) {
	   	  return
	   }
	   uid, okU := a.requireUser(c)
	  if !okU {
		   return
	     }
	   got, err := a.p2pFunds().GetDetail(c.Request.Context(), uid, c.Param("id"))
	       if err != nil {
	  	  fail(c, httpStatusP2P(err), err.Error())
	  	   return
	      }
	      ok(c, gin.H{"detail": got, "rest": p2pgate.HoldToREST(got.Hold)})
  }

   func (a *API) p2pSummary(c *gin.Context) {
	   if a.forwardMeshP2P(c, http.MethodGet, "/api/v1/p2p/summary", nil) {
		   return
	   }
	      uid, okU := a.requireUser(c)
	       if !okU {
		   return
	  }
	      sum, err := a.p2pFunds().OccupancySummaryOf(c.Request.Context(), uid)
	     if err != nil {
		   fail(c, 400, err.Error())
		   return
	  }
	   ok(c, p2pgate.SummaryToREST(sum))
   }

   func (a *API) p2pDetails(c *gin.Context) {
	   if a.forwardMeshP2P(c, http.MethodGet, "/api/v1/p2p/details", nil) {
		   return
	       }
	    uid, okU := a.requireUser(c)
	      if !okU {
		  return
	   }
	     q, okQ := a.p2pQuery(c, uid)
	  if !okQ {
		  return
	  }
	  if strings.EqualFold(c.Query("format"), "csv") {
	    	   raw, err := a.p2pFunds().ExportDetailsCSV(c.Request.Context(), q)
	  	  if err != nil {
			  fail(c, 400, err.Error())
			  return
		  }
		   c.Data(http.StatusOK, "text/csv; charset=utf-8", []byte(raw))
		  return
	      }
	   page, err := a.p2pFunds().ListDetails(c.Request.Context(), q)
	   if err != nil {
	    	  fail(c, 400, err.Error())
	   	   return
	    }
	      ok(c, gin.H{"items": p2pgate.HoldsToREST(page.Items), "page": page.Page})
  }

   func (a *API) p2pFlow(c *gin.Context) {
	    if a.forwardMeshP2P(c, http.MethodGet, "/api/v1/p2p/flow", nil) {
	    	   return
	   }
	  uid, okU := a.requireUser(c)
	      if !okU {
	    	   return
	      }
	  q, okQ := a.p2pQuery(c, uid)
	  if !okQ {
		   return
	  }
	    if strings.EqualFold(c.Query("format"), "csv") {
	  	  raw, err := a.p2pFunds().ExportFlowCSV(c.Request.Context(), q)
		   if err != nil {
			   fail(c, 400, err.Error())
	   		  return
		  }
	  	  c.Data(http.StatusOK, "text/csv; charset=utf-8", []byte(raw))
		  return
	       }
	     page,
	  	   err := a.p2pFunds().ListFlow(c.Request.Context(), q)
	  if err != nil {
	    	   fail(c, 400, err.Error())
	   	   return
	     }
	     net,
		  err := p2pgate.NetFlow(page.Items)
	   if err != nil {
		  fail(c, 400, err.Error())
	    	  return
	  }
	  ok(c, gin.H{"items": p2pgate.FlowsToREST(page.Items), "page": page.Page, "net": net})
   }

  func (a *API) p2pMerchant(c *gin.Context) {
	     if a.forwardMeshP2P(c, http.MethodGet, "/api/v1/p2p/merchant", nil) {
		  return
	   }
	      uid, okU := a.requireUser(c)
	  if !okU {
	   	   return
	   }
	    q, okQ := a.p2pQuery(c, uid)
	  if !okQ {
		   return
	     }
	  page, err := a.p2pFunds().ListMerchant(c.Request.Context(), q)
	    if err != nil {
		   fail(c, 400, err.Error())
		   return
	  }
	   ok(c, gin.H{"items": p2pgate.MerchantsToREST(page.Items), "page": page.Page})
   }

   func (a *API) p2pMerchantSummary(c *gin.Context) {
	  if a.forwardMeshP2P(c, http.MethodGet, "/api/v1/p2p/merchant/summary", nil) {
		  return
	  }
	     uid, okU := a.requireUser(c)
	   if !okU {
	    	  return
	   }
	  q, okQ := a.p2pQuery(c, uid)
	   if !okQ {
		   return
	  }
	   sum, err := a.p2pFunds().MerchantOccupancy(c.Request.Context(), q)
	   if err != nil {
	    	  fail(c, 400, err.Error())
	   	  return
	   }
	  ok(c, sum)
  }

  func (a *API) p2pOrders(c *gin.Context) {
	     if a.forwardMeshP2P(c, http.MethodGet, "/api/v1/p2p/orders", nil) {
	  	   return
	  }
	       uid, okU := a.requireUser(c)
	   if !okU {
		   return
	  }
	       q, okQ := a.p2pQuery(c, uid)
	     if !okQ {
		   return
	  }
	    page, err := a.p2pFunds().ListOrders(c.Request.Context(), q)
	   if err != nil {
	   	   fail(c, 400, err.Error())
	  	   return
	  }
	   ok(c, gin.H{"items": page.Items, "page": page.Page})
   }

 func (a *API) p2pOrder(c *gin.Context) {
	  if a.forwardMeshP2P(c, http.MethodGet, "/api/v1/p2p/orders/"+c.Param("id"), nil) {
		  return
	  }
	  uid, okU := a.requireUser(c)
	     if !okU {
		  return
	    }
	      sheet, err := a.p2pFunds().GetOrderSheet(c.Request.Context(), uid, c.Param("id"))
	  if err != nil {
		  fail(c, 400, err.Error())
		  return
	     }
	       ok(c, sheet)
   }

   func (a *API) p2pIntegrity(c *gin.Context) {
	  if a.forwardMeshP2P(c, http.MethodGet, "/api/v1/p2p/integrity", nil) {
	    	   return
	   }
	    uid, okU := a.requireUser(c)
	   if !okU {
	  	   return
	   }
	      rep, err := a.p2pFunds().Integrity(c.Request.Context(), uid)
	   if err != nil {
	   	   fail(c, 400, err.Error())
		  return
	   }
	  ok(c, rep)
   }

   func (a *API) p2pAggregate(c *gin.Context) {
	       if a.forwardMeshP2P(c, http.MethodGet, "/api/v1/p2p/aggregate", nil) {
	    	  return
	  }
	  uid, okU := a.requireUser(c)
	   if !okU {
		   return
	     }
	   q, okQ := a.p2pQuery(c, uid)
	  if !okQ {
	   	  return
	    }
	     byAsset, err := a.p2pFunds().AggregateByAsset(c.Request.Context(), q)
	       if err != nil {
		   fail(c, 400, err.Error())
	   	   return
	       }
	   byReason, err := a.p2pFunds().AggregateByReason(c.Request.Context(), uid)
	   if err != nil {
		  fail(c, 400, err.Error())
		  return
	  }
	     ok(c, gin.H{"byAsset": byAsset, "byReason": byReason})
   }

   func (a *API) p2pWindows(c *gin.Context) {
	    if a.forwardMeshP2P(c, http.MethodGet, "/api/v1/p2p/windows", nil) {
		   return
	      }
	      uid, okU := a.requireUser(c)
	      if !okU {
		  return
	   }
	      q, okQ := a.p2pQuery(c, uid)
	  if !okQ {
	    	   return
	      }
	   days, err := a.p2pFunds().DayWindows(c.Request.Context(), q)
	   if err != nil {
		  fail(c, 400, err.Error())
	   	  return
	  }
	  ok(c, gin.H{"days": days})
  }

   func (a *API) p2pCooling(c *gin.Context) {
	       if a.forwardMeshP2P(c, http.MethodGet, "/api/v1/p2p/cooling", nil) {
		   return
	     }
	   uid, okU := a.requireUser(c)
	     if !okU {
		   return
	  }
	  cool, err := a.p2pFunds().CoolingState(c.Request.Context(), uid)
	     if err != nil {
		   fail(c, 400, err.Error())
	    	  return
	  }
	  ok(c, cool)
  }

   func (a *API) p2pCatalog(c *gin.Context) {
	  if a.forwardMeshP2P(c, http.MethodGet, "/api/v1/p2p/catalog", nil) {
		   return
	   }
	   ok(c, p2pgate.OccupancyCatalog())
   }

   func (a *API) p2pSnapshot(c *gin.Context) {
	   if a.forwardMeshP2P(c, http.MethodGet, "/api/v1/p2p/snapshot", nil) {
		   return
	  }
	  uid, okU := a.requireUser(c)
	  if !okU {
	    	  return
	   }
	     snap, err := a.p2pFunds().Snapshot(c.Request.Context(), uid)
	     if err != nil {
	  	  fail(c, 400, err.Error())
	   	   return
	      }
	  ok(c, p2pgate.SnapshotREST(snap))
   }

   func httpStatusP2P(err error) int {
	      if err == p2pgate.ErrHoldGone {
	   	  return 404
	   }
	    if err == p2pgate.ErrQueryRange || err == p2pgate.ErrQueryPage || err == p2pgate.ErrQueryAsset {
	  	  return 400
	   }
	      return 400
  }
