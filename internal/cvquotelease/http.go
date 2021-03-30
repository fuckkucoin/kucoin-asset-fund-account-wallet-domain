// 闪兑报价租约 HTTP 接口。

// Author: Finn
// Date: 2021.03.29

package cvquotelease

import (
	"net/http"
	"strconv"
	"strings"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

  func RegisterRoutes(v1 *gin.RouterGroup, eng *Engine) {
	   g := v1.Group("/cvquotelease")
	  g.POST("/quote", quoteHandler(eng))
	  g.POST("/execute", executeHandler(eng))
	   g.POST("/place-quoted", placeQuotedHandler(eng))
	  g.POST("/mark", markPutHandler(eng))
	   g.GET("/lease/:id", getLeaseHandler(eng))
	  g.GET("/leases", listLeasesHandler(eng))
	   g.GET("/proofs", exportProofsHandler(eng))
	  g.GET("/health", healthHandler(eng))
	  g.POST("/workers/expire", expireWorkerHandler(eng))
	   g.POST("/workers/revoke-pump", revokePumpHandler(eng))
	   g.POST("/workers/reconcile", reconcileHandler(eng))
  }

   func quoteHandler(eng *Engine) gin.HandlerFunc {
	   return func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   var req QuoteWithLeaseReq
		   if !mw.BindJSON(c, &req) {
			  return
		   }
		   resp, err := eng.QuoteWithLease(c.Request.Context(), req)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, resp)
	   }
   }

  func executeHandler(eng *Engine) gin.HandlerFunc {
	   return func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   var req ExecuteWithLeaseReq
		   if !mw.BindJSON(c, &req) {
			   return
		  }
		   resp,
			   err := eng.ExecuteWithLease(c.Request.Context(), req)
		  if err != nil {
			   c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "result": resp})
			   return
		  }
		  c.JSON(http.StatusOK, resp)
	   }
   }

   func placeQuotedHandler(eng *Engine) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  var req PlaceQuotedReq
		  if !mw.BindJSON(c, &req) {
			   return
		  }
		  resp, err := eng.PlaceQuoted(c.Request.Context(), req)
		  if err != nil {
			  c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "result": resp})
			  return
		  }
		  c.JSON(http.StatusOK, resp)
	   }
   }

   func markPutHandler(eng *Engine) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   var req MarkPutReq
		   if !mw.BindJSON(c, &req) {
			   return
		  }
		  resp, err := eng.MarkPut(c.Request.Context(), req)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, resp)
	  }
   }

  func getLeaseHandler(eng *Engine) gin.HandlerFunc {
	  return func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   lease, err := eng.GetLease(c.Request.Context(), strings.TrimSpace(c.Param("id")))
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, lease)
	  }
  }

   func listLeasesHandler(eng *Engine) gin.HandlerFunc {
	  return func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   userID := strings.TrimSpace(c.Query("user_id"))
		   limit := 50
		   if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 {
			   limit = l
		  }
		   leases, err := eng.ListLeases(c.Request.Context(), userID, limit)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, gin.H{"leases": leases, "count": len(leases)})
	   }
  }

   func exportProofsHandler(eng *Engine) gin.HandlerFunc {
	   return func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  limit := 50
		   if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 {
			  limit = l
		   }
		  resp, err := eng.ExportProofs(c.Request.Context(), ProofExportReq{
			  LeaseID: strings.TrimSpace(c.Query("lease_id")),
			  QuoteID: strings.TrimSpace(c.Query("quote_id")),
			   UserID: strings.TrimSpace(c.Query("user_id")),
			  Limit:  limit,
		  })
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(http.StatusOK, resp)
	   }
   }

   func healthHandler(eng *Engine) gin.HandlerFunc {
	   return func(c *gin.Context) {
		   h, err := eng.Health(c.Request.Context())
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(http.StatusOK, h)
	  }
  }

   func expireWorkerHandler(eng *Engine) gin.HandlerFunc {
	   return func(c *gin.Context) {
		  n, err := eng.ScanExpiredQuotes(c.Request.Context(), DefaultExpiryBatch)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, gin.H{"expired": n})
	   }
   }

   func revokePumpHandler(eng *Engine) gin.HandlerFunc {
	   return func(c *gin.Context) {
		   n, err := eng.PumpMarkRevokes(c.Request.Context(), DefaultRevokeBatch)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, gin.H{"revoked": n})
	  }
   }

   func reconcileHandler(eng *Engine) gin.HandlerFunc {
	  return func(c *gin.Context) {
		   n, err := eng.ReconcileUnconsumed(c.Request.Context(), DefaultExpiryBatch)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(http.StatusOK, gin.H{"reconciled": n})
	  }
  }
