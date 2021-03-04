package booksnapgate

import (
	"net/http"
	"strconv"
	"strings"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

   func RegisterRoutes(v1 *gin.RouterGroup, eng *Engine) {
	   g := v1.Group("/booksnapgate")
	   g.POST("/pin", pinSnapHandler(eng))
	  g.POST("/assemble", assembleHandler(eng))
	  g.POST("/repin", rePinHandler(eng))
	   g.POST("/abort", abortHandler(eng))
	  g.GET("/lease/:id", getLeaseHandler(eng))
	  g.GET("/leases", listLeasesHandler(eng))
	  g.GET("/proofs", exportProofsHandler(eng))
	  g.GET("/health", healthHandler(eng))
  }

   func pinSnapHandler(eng *Engine) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  var req PinSnapReq
		   if !mw.BindJSON(c, &req) {
			  return
		  }
		   resp, err := eng.PinSnap(c.Request.Context(), req.UserID, req.Asset, 0, req.Window)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, resp)
	  }
  }

   func assembleHandler(eng *Engine) gin.HandlerFunc {
	   return func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  var req AssembleReq
		   if !mw.BindJSON(c, &req) {
			   return
		   }
		  resp, err := eng.Assemble(c.Request.Context(), req.LeaseID)
		   if err != nil {
			   if IsRetryable(err) {
				  c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "result": resp})
				   return
			  }
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(http.StatusOK, resp)
	   }
  }

   func rePinHandler(eng *Engine) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   var req RePinReq
		  if !mw.BindJSON(c, &req) {
			   return
		  }
		  resp, err := eng.RePin(c.Request.Context(), req.LeaseID)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(http.StatusOK, resp)
	  }
   }

  func abortHandler(eng *Engine) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  var req AbortReq
		  if !mw.BindJSON(c, &req) {
			  return
		  }
		  err := eng.Abort(c.Request.Context(),
			  req.LeaseID, req.Reason)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, gin.H{"status": "aborted"})
	   }
   }

   func getLeaseHandler(eng *Engine) gin.HandlerFunc {
	  return func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  id := strings.TrimSpace(c.Param("id"))
		   lease, err := eng.GetLease(c.Request.Context(), id)
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
		   if userID == "" {
			   c.JSON(http.StatusBadRequest, gin.H{"error": "user_id required"})
			   return
		   }
		   limit := 50
		   if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 {
			  limit = l
		  }
		   leases,
			   err := eng.ListLeases(c.Request.Context(), userID, limit)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK,
			  LeaseQueryResp{Leases: leases, Count: len(leases)})
	   }
  }

  func exportProofsHandler(eng *Engine) gin.HandlerFunc {
	   return func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   leaseID := strings.TrimSpace(c.Query("lease_id"))
		   userID := strings.TrimSpace(c.Query("user_id"))
		  limit := 50
		  if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 {
			   limit = l
		  }
		   proofs, err := eng.ExportProofs(c.Request.Context(), leaseID, userID, limit)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, ProofExportResp{Proofs: proofs, Count: len(proofs)})
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
