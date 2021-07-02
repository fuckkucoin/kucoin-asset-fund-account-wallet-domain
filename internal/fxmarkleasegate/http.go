package fxmarkleasegate

import (
	"net/http"
	"os"
	"strings"
	"account-assets.kucoin.net/funding/internal/keysrc"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/opsfc"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

   func RegisterRoutes(v1 *gin.RouterGroup, eng *Engine) {
	   g := v1.Group("/fxmarkleasegate")
	  g.POST("/issue", issueHandler(eng))
	   g.POST("/bind", bindHandler(eng))
	  g.POST("/assert", assertHandler(eng))
	   g.GET("/lease/:id", getLeaseHandler(eng))
	   g.GET("/leases", listLeasesHandler(eng))
	   g.GET("/proofs", exportProofsHandler(eng))
	  g.POST("/ops/revoke", opsRevokeHandler(eng))
	  g.GET("/health", healthHandler(eng))
	  g.POST("/workers/expire-scan", expireHandler(eng))
	   g.POST("/workers/put-revoke", putRevokeHandler(eng))
	  g.POST("/workers/live-patrol", patrolHandler(eng))
   }

  func issueHandler(eng *Engine) gin.HandlerFunc {
	   return func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   var req IssueMarkLeaseReq
		  if !mw.BindJSON(c, &req) {
			  return
		   }
		  resp, err := eng.IssueMarkLease(c.Request.Context(), req)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, resp)
	   }
   }

  func bindHandler(eng *Engine) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  var req BindAssertReq
		  if !mw.BindJSON(c, &req) {
			   return
		   }
		  resp,
			  err := eng.BindBeforePost(c.Request.Context(), req)
		   if err != nil {
			   c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "result": resp})
			   return
		  }
		   c.JSON(http.StatusOK, resp)
	   }
   }

  func assertHandler(eng *Engine) gin.HandlerFunc {
	  return func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   var req AssertBeforePostReq
		   if !mw.BindJSON(c, &req) {
			  return
		  }
		   resp, err := eng.AssertBeforePost(c.Request.Context(), req)
		  if err != nil {
			  c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "result": resp})
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
		  l, err := eng.GetLease(c.Request.Context(), c.Param("id"))
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(http.StatusOK, l)
	  }
  }

  func listLeasesHandler(eng *Engine) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  pair := strings.TrimSpace(c.Query("pair"))
		   rows, err := eng.ListLeases(c.Request.Context(), pair, 50)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, gin.H{"leases": rows, "count": len(rows)})
	   }
   }

   func exportProofsHandler(eng *Engine) gin.HandlerFunc {
	   return func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  resp, err := eng.ExportProofs(c.Request.Context(), ProofExportReq{
			   LeaseID: c.Query("lease_id"), Kind: c.Query("kind"),
		   })
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, resp)
	   }
   }

   func opsRevokeHandler(eng *Engine) gin.HandlerFunc {
	  return func(c *gin.Context) {
		   if !opsfc.GateOps(c) {
			  return
		   }
		  if err := keysrc.RefuseLocalHMAC(os.Getenv("AUTH_KIND"), os.Getenv("APP_PROFILE"), os.Getenv("KEY_SOURCE")); err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   var req OpsRevokeReq
		  if !mw.BindJSON(c, &req) {
			  return
		   }
		   resp, err := eng.OpsRevoke(c.Request.Context(), req)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, resp)
	  }
  }

   func healthHandler(eng *Engine) gin.HandlerFunc {
	   return func(c *gin.Context) {
		   resp, err := eng.Health(c.Request.Context())
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, resp)
	   }
   }

   func expireHandler(eng *Engine) gin.HandlerFunc {
	   return func(c *gin.Context) {
		  n,
			  err := eng.ExpireScan(c.Request.Context(), DefaultExpiryBatch)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, gin.H{"expired": n})
	   }
   }

   func putRevokeHandler(eng *Engine) gin.HandlerFunc {
	   return func(c *gin.Context) {
		   n, err := eng.PumpPutRevoke(c.Request.Context(), DefaultPatrolBatch)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, gin.H{"revoked": n})
	  }
  }

  func patrolHandler(eng *Engine) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  n, err := eng.PatrolLiveDrift(c.Request.Context(), DefaultPatrolBatch)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(http.StatusOK, gin.H{"revoked": n})
	  }
  }
