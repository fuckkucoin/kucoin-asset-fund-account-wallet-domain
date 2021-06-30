// - 所有写接口（trigger-scan、promote、step1/step2 clear、export）需 ActOpsAdmin 权限。
// - 读接口（list-cases、get-case、get-proof）需 ActQueryRead 权限。
// - 不暴露 fiatjnl 写路径；不改写 fiatadd/fiatrail 编排。
// - 错误通过 mw.WriteProblem 统一返回（RFC 7807）。
// 不变量：
// - 所有时间字段以 UTC RFC3339 返回。
// - ProofBundle 导出接口仅允许 ProofPending/ProofExported 状态的证明触发（verified 只读）。

// Author: KianFord
// Date: 2021.05.12

package fiatbalgate

import (
	"net/http"
	"strconv"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// Handler fiatbalgate HTTP 处理器集合。

type Handler struct {
	engine *Engine
	scanner *Scanner
	clearance *ClearanceService
	exporter  *ProofExporter
	store     Store
}

// NewHandler 构造 Handler。
  func NewHandler(engine *Engine, scanner *Scanner, clearance *ClearanceService, exporter *ProofExporter, store Store) *Handler {
	   return &Handler{
		  engine: engine,
		   scanner: scanner,
		  clearance: clearance,
		  exporter:  exporter,
		   store: store,
	  }
   }

// RegisterRoutes 注册所有 fiatbalgate 路由到 v1 路由组。
  func (h *Handler) RegisterRoutes(v1 *gin.RouterGroup) {
	   g := v1.Group("/fiatbalgate")
	   // 扫描触发（ops 主动触发，Worker 自动周期触发）
	   g.POST("/scan", h.handleTriggerScan)
	  // 案件相关
	   g.GET("/cases", h.handleListCases)
	  g.GET("/cases/:case_id", h.handleGetCase)
	   g.POST("/cases/:case_id/promote", h.handlePromoteBlocking)
	  // 四眼清障
	  g.POST("/cases/:case_id/clear/step1", h.handleClearStep1)
	  g.POST("/cases/:case_id/clear/step2", h.handleClearStep2)
	  g.GET("/cases/:case_id/clear/status", h.handleClearStatus)
	  // 证明导出与复验
	  g.GET("/proofs/:proof_id", h.handleGetProof)
	  g.POST("/proofs/:proof_id/export", h.handleExportProof)
	  g.POST("/proofs/:proof_id/verify", h.handleVerifyProof)
	   // 世代查询
	   g.GET("/gens/max", h.handleMaxGenSeq)
   }

// ── 扫描 ──────────────────────────────────────────────────────────────────────

   func (h *Handler) handleTriggerScan(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	   var body ScanReq
	  if !mw.BindJSON(c, &body) {
		   return
	   }
	  if body.ScannerID == "" {
		  body.ScannerID = "http-trigger"
	  }
	  result, err := h.scanner.Scan(c.Request.Context(), body)
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, result)
   }

// ── 案件 ──────────────────────────────────────────────────────────────────────

   func (h *Handler) handleListCases(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	  asset := c.Query("asset")
	  limit := 50
	  if s := c.Query("limit"); s != "" {
		  if n, err := strconv.Atoi(s); err == nil && n > 0 {
			   limit = n
		   }
	  }
	  cases, err := h.store.ListOpenOrBlockingCases(c.Request.Context(), asset, limit)
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, gin.H{"cases": cases, "host": "deposit-withdraw.kucoin.net"})
  }

   func (h *Handler) handleGetCase(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	  caseID := c.Param("case_id")
	  cs,
		   err := h.store.GetCase(c.Request.Context(), caseID)
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, cs)
   }

  func (h *Handler) handlePromoteBlocking(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	  caseID := c.Param("case_id")
	   cs, err := h.engine.PromoteToBlocking(c.Request.Context(), caseID)
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	   }
	  c.JSON(http.StatusOK, cs)
   }

// ── 四眼清障 ──────────────────────────────────────────────────────────────────

   func (h *Handler) handleClearStep1(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	  var body struct {
		  Actor string `json:"actor"`
		  Note string `json:"note"`
	   }
	  if !mw.BindJSON(c, &body) {
		  return
	   }
	   req := ClearReq{
		   CaseID: c.Param("case_id"),
		   Actor: body.Actor,
		  Note:  body.Note,
	   }
	   status, err := h.clearance.SubmitStep1(c.Request.Context(), req)
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, status)
  }

   func (h *Handler) handleClearStep2(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	   var body struct {
		   Actor string `json:"actor"`
		  Note  string `json:"note"`
	  }
	   if !mw.BindJSON(c, &body) {
		  return
	  }
	  req := ClearReq{
		  CaseID: c.Param("case_id"),
		  Actor: body.Actor,
		   Note:  body.Note,
	   }
	   status, proof, err := h.clearance.SubmitStep2(c.Request.Context(), req)
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	   }
	  c.JSON(http.StatusOK, gin.H{"status": status, "proof": proof})
  }

  func (h *Handler) handleClearStatus(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	   caseID := c.Param("case_id")
	  status, err := h.clearance.GetStatus(c.Request.Context(), caseID)
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, status)
   }

// ── 证明 ──────────────────────────────────────────────────────────────────────

   func (h *Handler) handleGetProof(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	  proofID := c.Param("proof_id")
	   p, err := h.store.GetProof(c.Request.Context(), proofID)
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	   c.JSON(http.StatusOK, p)
   }

  func (h *Handler) handleExportProof(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	  proofID := c.Param("proof_id")
	   actor := c.Query("actor")
	   if actor == "" {
		   actor = "ops"
	   }
	   bundle, err := h.exporter.Export(c.Request.Context(), proofID, actor)
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, bundle)
  }

  func (h *Handler) handleVerifyProof(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	  proofID := c.Param("proof_id")
	  var body struct {
		  VerifiedBy string `json:"verified_by"`
	   }
	   if !mw.BindJSON(c, &body) {
		  return
	  }
	  p, err := h.engine.VerifyProof(c.Request.Context(), proofID, body.VerifiedBy)
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	   c.JSON(http.StatusOK, p)
   }

// ── 世代 ──────────────────────────────────────────────────────────────────────

  func (h *Handler) handleMaxGenSeq(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	  seq, err := h.store.MaxGenSeq(c.Request.Context())
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, gin.H{"max_gen_seq": seq})
   }
