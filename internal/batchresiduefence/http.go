package batchresiduefence

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

   func RegisterRoutes(v1 *gin.RouterGroup, eng *Engine) {
	  g := v1.Group("/wdbatch")
	  g.POST("/retry-with-fence", retryWithFenceHandler(eng))
	  g.POST("/submit-assert", submitAssertHandler(eng))
	   g.POST("/residue-fence/issue", issueHandler(eng))
	   g.GET("/residue-fence/:id", getFenceHandler(eng))
	  g.GET("/residue-fences", listFenceHandler(eng))
	  g.POST("/residue-fence/release", releaseHandler(eng))
	  g.GET("/dup-freeze-proofs", exportProofHandler(eng))
	   g.POST("/workers/residue-patrol", patrolHandler(eng))
	  g.POST("/workers/dup-freeze-export", exportJobHandler(eng))
	  g.POST("/workers/half-cut-resume", halfResumeHandler(eng))
	   g.GET("/residue-fence/health", healthHandler(eng))
   }

  func retryWithFenceHandler(eng *Engine) gin.HandlerFunc {
	   return func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  var req RetryWithFenceReq
		  if !mw.BindJSON(c, &req) {
			  return
		   }
		   resp, err := eng.RetryWithFence(c.Request.Context(), req)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, resp)
	   }
  }

  func submitAssertHandler(eng *Engine) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   var req SubmitAssertReq
		  if !mw.BindJSON(c, &req) {
			   return
		   }
		   resp, err := eng.SubmitAssert(c.Request.Context(), req)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, resp)
	  }
  }

   func issueHandler(eng *Engine) gin.HandlerFunc {
	   return func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   var req IssueFenceReq
		  if !mw.BindJSON(c, &req) {
			  return
		   }
		   got, err := eng.IssueFence(c.Request.Context(), req)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(http.StatusOK, got)
	  }
   }

   func getFenceHandler(eng *Engine) gin.HandlerFunc {
	   return func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  got, err := eng.GetFence(c.Request.Context(), c.Param("id"))
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, got)
	   }
   }

   func listFenceHandler(eng *Engine) gin.HandlerFunc {
	   return func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   rows, err := eng.ListFences(c.Request.Context(), c.Query("batch_id"))
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(http.StatusOK, gin.H{"fences": rows})
	   }
  }

  func releaseHandler(eng *Engine) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			  ID string `json:"id"`
			  Actor string `json:"actor"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		   got, err := eng.ReleaseFence(c.Request.Context(), body.ID, body.Actor)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(http.StatusOK, got)
	   }
  }

  func exportProofHandler(eng *Engine) gin.HandlerFunc {
	  return func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  got, err := eng.ExportDupProofs(c.Request.Context(), c.Query("batch_id"))
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, got)
	  }
  }

  func patrolHandler(eng *Engine) gin.HandlerFunc {
	   return func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   var body struct {
			   Limit int `json:"limit"`
		   }
		  _ = c.ShouldBindJSON(&body)
		   got, err := eng.PatrolResidue(c.Request.Context(), body.Limit)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(http.StatusOK, got)
	   }
  }

  func exportJobHandler(eng *Engine) gin.HandlerFunc {
	   return func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  n,
			  err := eng.ExportUnexported(c.Request.Context(), DefaultPatrolLimit)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, gin.H{"exported": n})
	  }
  }

   func halfResumeHandler(eng *Engine) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			  Actor string `json:"actor"`
			  Limit int    `json:"limit"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  n, err := eng.ResumeHalfCuts(c.Request.Context(), body.Actor, body.Limit)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, gin.H{"resumed": n})
	  }
   }

  func healthHandler(eng *Engine) gin.HandlerFunc {
	   return func(c *gin.Context) {
		  c.JSON(http.StatusOK, gin.H{"ok": eng != nil, "module": "batchresiduefence"})
	   }
  }
