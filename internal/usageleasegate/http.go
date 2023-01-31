// 功能：占用窗口租约 HTTP：SnapshotWithLease / RecordAssert / 证明导出 / 运维释放。
// 作者：Rook
// 日期：2021.12.13

package usageleasegate

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"account-assets.kucoin.net/funding/internal/keysrc"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/opsfc"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

   func RegisterRoutes(v1 *gin.RouterGroup, eng *Engine) {
	  g := v1.Group("/usageleasegate")
	  g.POST("/snapshot", snapshotHandler(eng))
	   g.POST("/breach-daily", snapshotHandler(eng))
	   g.POST("/record", recordHandler(eng))
	  g.GET("/lease/:id", getLeaseHandler(eng))
	   g.GET("/leases", listLeasesHandler(eng))
	   g.GET("/proofs", exportProofsHandler(eng))
	  g.POST("/ops/release", opsReleaseHandler(eng))
	  g.GET("/health", healthHandler(eng))
	  g.POST("/workers/expire-release", expireHandler(eng))
	  g.POST("/workers/window-patrol", patrolHandler(eng))
	  g.POST("/workers/over-cap", overCapHandler(eng))
   }

   func snapshotHandler(eng *Engine) gin.HandlerFunc {
	  return func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   var req SnapshotWithLeaseReq
		   if !mw.BindJSON(c, &req) {
			  return
		   }
		  resp, err := eng.SnapshotWithLease(c.Request.Context(), req)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, resp)
	  }
   }

   func recordHandler(eng *Engine) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   var req RecordAssertReq
		  if !mw.BindJSON(c, &req) {
			   return
		  }
		  resp, err := eng.RecordAssert(c.Request.Context(), req)
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
		  limit, _ := strconv.Atoi(c.Query("limit"))
		   list, err := eng.ListLeases(c.Request.Context(), strings.TrimSpace(c.Query("user_id")), limit)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(http.StatusOK, gin.H{"leases": list, "count": len(list)})
	   }
   }

   func exportProofsHandler(eng *Engine) gin.HandlerFunc {
	   return func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   limit, _ := strconv.Atoi(c.Query("limit"))
		  resp, err := eng.ExportProofs(c.Request.Context(), ProofExportReq{
			  LeaseID: c.Query("lease_id"),
			   UserID: c.Query("user_id"),
			   Kind:   c.Query("kind"),
			  Limit:  limit,
		   })
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, resp)
	   }
   }

   func opsReleaseHandler(eng *Engine) gin.HandlerFunc {
	  return func(c *gin.Context) {
		   if !opsfc.GateOps(c) {
			   return
		  }
		   if err := keysrc.RefuseLocalHMAC(os.Getenv("AUTH_KIND"), os.Getenv("APP_PROFILE"), os.Getenv("KEY_SOURCE")); err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   var req OpsReleaseReq
		  if !mw.BindJSON(c, &req) {
			   return
		  }
		  resp,
			  err := eng.OpsRelease(c.Request.Context(), req)
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
		   n, err := eng.ExpireRelease(c.Request.Context(), DefaultExpiryBatch)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(http.StatusOK, gin.H{"released": n})
	  }
   }

   func patrolHandler(eng *Engine) gin.HandlerFunc {
	  return func(c *gin.Context) {
		   n, err := eng.PatrolWindowDrift(c.Request.Context(), DefaultPatrolBatch)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(http.StatusOK, gin.H{"revoked": n})
	  }
   }

  func overCapHandler(eng *Engine) gin.HandlerFunc {
	   return func(c *gin.Context) {
		   n, err := eng.DiscoverOverCap(c.Request.Context(), DefaultPatrolBatch)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(http.StatusOK, gin.H{"found": n})
	  }
  }
