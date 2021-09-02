// @FileName: http.go
// @Summary: HTTP：ApproveMemoWithEffect / DecideMemoGuarded / 证明查询 / 运维冲正闸。
// @Author: Ronan
// @Created: 2021.09.01

package memoeffectgate

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
	   g := v1.Group("/memoeffectgate")
	   g.POST("/approve", approveHandler(eng))
	  g.POST("/decide", decideHandler(eng))
	  g.GET("/effect/:id", getEffectHandler(eng))
	   g.GET("/fence/:id", getFenceHandler(eng))
	   g.GET("/proofs", exportProofsHandler(eng))
	  g.GET("/proof/:id", getProofHandler(eng))
	  g.POST("/ops/reverse", opsReverseHandler(eng))
	  g.GET("/health", healthHandler(eng))
	  g.POST("/workers/half-cut", halfCutHandler(eng))
	   g.POST("/workers/reject-after-credit", rejectPatrolHandler(eng))
	  g.POST("/workers/expire", expireHandler(eng))
	  g.POST("/workers/export-proofs", exportJobHandler(eng))
  }

  func approveHandler(eng *Engine) gin.HandlerFunc {
	   return func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   var req ApproveMemoWithEffectReq
		   if !mw.BindJSON(c, &req) {
			   return
		   }
		   resp, err := eng.ApproveMemoWithEffect(c.Request.Context(), req)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(http.StatusOK, resp)
	  }
   }

   func decideHandler(eng *Engine) gin.HandlerFunc {
	  return func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   var req DecideMemoGuardedReq
		   if !mw.BindJSON(c, &req) {
			  return
		   }
		  resp, err := eng.DecideMemoGuarded(c.Request.Context(), req)
		  if err != nil {
			  c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "result": resp})
			   return
		   }
		  c.JSON(http.StatusOK, resp)
	   }
  }

   func getEffectHandler(eng *Engine) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   e, err := eng.GetEffect(c.Request.Context(), c.Param("id"))
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, e)
	   }
  }

   func getFenceHandler(eng *Engine) gin.HandlerFunc {
	   return func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   f, err := eng.GetFence(c.Request.Context(), c.Param("id"))
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, f)
	   }
   }

   func getProofHandler(eng *Engine) gin.HandlerFunc {
	   return func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   p, err := eng.GetProof(c.Request.Context(), c.Param("id"))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(http.StatusOK, p)
	   }
   }

   func exportProofsHandler(eng *Engine) gin.HandlerFunc {
	   return func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  kind := ProofKind(strings.TrimSpace(c.Query("kind")))
		  limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
		   rows,
			   err := eng.ListProofs(c.Request.Context(), kind, limit)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, gin.H{"proofs": rows})
	   }
  }

  func opsReverseHandler(eng *Engine) gin.HandlerFunc {
	   return func(c *gin.Context) {
		  if !opsfc.GateOps(c) {
			   return
		  }
		   if err := keysrc.RefuseLocalHMAC(os.Getenv("AUTH_KIND"), os.Getenv("APP_PROFILE"), os.Getenv("KEY_SOURCE")); err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   var req ReverseGateReq
		   if !mw.BindJSON(c, &req) {
			   return
		   }
		  p, err := eng.OpsReverseGate(c.Request.Context(), req)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, p)
	  }
   }

  func healthHandler(eng *Engine) gin.HandlerFunc {
	  return func(c *gin.Context) {
		   c.JSON(http.StatusOK, eng.Health(c.Request.Context()))
	   }
  }

   func halfCutHandler(eng *Engine) gin.HandlerFunc {
	   return func(c *gin.Context) {
		   n, err := eng.PatrolHalfCut(c.Request.Context(), DefaultPatrolBatch)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(http.StatusOK, gin.H{"resumed": n})
	   }
  }

   func rejectPatrolHandler(eng *Engine) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  n, err := eng.PatrolRejectAfterCredit(c.Request.Context(), DefaultPatrolBatch)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(http.StatusOK, gin.H{"proofs": n})
	   }
   }

   func expireHandler(eng *Engine) gin.HandlerFunc {
	   return func(c *gin.Context) {
		   n,
			  err := eng.ExpireBound(c.Request.Context(), DefaultExpiryBatch)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, gin.H{"expired": n})
	   }
  }

  func exportJobHandler(eng *Engine) gin.HandlerFunc {
	   return func(c *gin.Context) {
		   n, err := eng.ExportOpenProofs(c.Request.Context(), DefaultPatrolBatch)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(http.StatusOK, gin.H{"exported": n})
	   }
  }
