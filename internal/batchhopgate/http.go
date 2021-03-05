package batchhopgate

import (
	"net/http"
	"strconv"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// Register 将 batchhopgate 路由挂载到 /v1 路由组。

  func Register(v1 *gin.RouterGroup, _ *app.Runtime) {
	  svc := NewService(nil)
	  registerRoutes(v1, svc)
   }

// registerRoutes 内部供注入 svc。
   func registerRoutes(v1 *gin.RouterGroup, svc *Service) {
	   g := v1.Group("/batchhopgate")
	   // ── 世代查询 ─────────────────────────────────────────────────────────────
	   g.GET("/generation", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  userID := strings.TrimSpace(c.Query("user_id"))
		  asset := strings.TrimSpace(c.Query("asset"))
		  if userID == "" || asset == "" {
			   c.JSON(http.StatusBadRequest, gin.H{"error": "user_id and asset required"})
			   return
		   }
		   gen,
			  err := svc.GetGeneration(c.Request.Context(), userID, asset)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, gin.H{"generation": gen})
	  })
	   // ── 世代令牌颁发 ─────────────────────────────────────────────────────────
	  g.GET("/token", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  caseID := strings.TrimSpace(c.Query("case_id"))
		  if caseID == "" {
			  c.JSON(http.StatusBadRequest, gin.H{"error": "case_id required"})
			  return
		   }
		   token, err := svc.IssueToken(c.Request.Context(), caseID)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(http.StatusOK, gin.H{
			   "token":      token,
			  "expires_in": int(time.Until(token.ExpiresAt).Seconds()),
		  })
	   })
	   // ── 残留案查询列表 ───────────────────────────────────────────────────────
	  g.GET("/residuals", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   userID := strings.TrimSpace(c.Query("user_id"))
		   if userID == "" {
			   c.JSON(http.StatusBadRequest, gin.H{"error": "user_id required"})
			  return
		   }
		   limit := 20
		   if ls := c.Query("limit"); ls != "" {
			  if n, err := strconv.Atoi(ls); err == nil && n > 0 && n <= 100 {
				   limit = n
			   }
		  }
		   list,
			  err := svc.ListResiduals(c.Request.Context(), userID, limit)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(http.StatusOK, gin.H{"residuals": list, "count": len(list)})
	  })
	  // ── 单条残留案查询 ──────────────────────────────────────────────────────
	   g.GET("/residual/:id", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   residual, err := svc.GetResidual(c.Request.Context(), c.Param("id"))
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(http.StatusOK, gin.H{"residual": residual})
	   })
	   // ── 手动开立残留案（ops/集成） ───────────────────────────────────────
	   g.POST("/residual/open", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   var body OpenResidualFullReq
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		   result,
			  err := svc.OpenResidual(c.Request.Context(), body)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, gin.H{
			   "residual": result.Residual,
			  "proof": result.Proof,
			  "token": result.Token,
			   "expires_in": int(time.Until(result.Token.ExpiresAt).Seconds()),
		   })
	  })
	   // ── 四眼确认收口 ─────────────────────────────────────────────────────────
	   g.POST("/residual/confirm", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   var body struct {
			  CaseID      string `json:"case_id"`
			   ConfirmerID string `json:"confirmer_id"`
			   Note string `json:"note"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		  result,
			   err := svc.ConfirmClose(c.Request.Context(), body.CaseID, body.ConfirmerID, body.Note)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, gin.H{"residual": result})
	  })
	   // ── 管理员强制阻塞残留案 ─────────────────────────────────────────────────
	   g.POST("/residual/block", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			   CaseID string `json:"case_id"`
			  Reason string `json:"reason"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		   result, err := svc.BlockResidual(c.Request.Context(), body.CaseID, body.Reason)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, gin.H{"residual": result})
	  })
	   // ── 申请续跳互斥租约 ─────────────────────────────────────────────────────
	   g.POST("/resume/request", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			   ResidualCaseID string `json:"residual_case_id"`
			  Token       GenToken `json:"token"`
			   ResumeFrom  int      `json:"resume_from"`
			   ConfirmedBy string   `json:"confirmed_by"`
			   Note string `json:"note"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		   lease, err := svc.RequestResume(c.Request.Context(), RequestResumeReq{
			  ResidualCaseID: body.ResidualCaseID,
			  Token:          body.Token,
			   ResumeFrom:     body.ResumeFrom,
			  ConfirmedBy:    body.ConfirmedBy,
			   Note:           body.Note,
		   })
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(http.StatusOK, gin.H{
			  "lease": lease,
			  "expires_in": int(time.Until(lease.ExpiresAt).Seconds()),
		   })
	   })
	   // ── 单条续跳租约查询 ─────────────────────────────────────────────────────
	  g.GET("/lease/:id", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   lease, err := svc.store.GetLease(c.Request.Context(), c.Param("id"))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, gin.H{"lease": lease})
	  })
	   // ── 验证占用证明 Digest ───────────────────────────────────────────────────
	  g.POST("/proof/verify", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			   ProofID string `json:"proof_id"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  proof, err := svc.VerifyProof(c.Request.Context(), body.ProofID)
		  if err != nil {
			  if err == ErrProofDigestMismatch {
				   svc.metrics.proofDigestFail.Inc()
			  }
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, gin.H{"proof": proof, "digest_ok": true})
	   })
	   // ── 单条占用证明查询 ─────────────────────────────────────────────────────
	  g.GET("/proof/:id", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   proof, err := svc.GetProof(c.Request.Context(), c.Param("id"))
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, gin.H{"proof": proof})
	  })
	   // ── 导出占用证明 JSON ─────────────────────────────────────────────────────
	   g.GET("/proof/:id/export", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  data, err := svc.ExportProof(c.Request.Context(), c.Param("id"))
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.Data(http.StatusOK, "application/json; charset=utf-8", data)
	   })
  }
