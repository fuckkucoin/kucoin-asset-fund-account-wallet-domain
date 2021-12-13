package stucksettle

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// Register 注册 stucksettle HTTP 路由。
// 前缀：/v1/stucksettle/
  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   gate := NewGate(GateConfig{NodeID: "api-server"})
	   h := &httpHandler{gate: gate}
	   g := v1.Group("/stucksettle")
	  // 世代状态查询
	  g.GET("/gen/:transfer_id", h.getGen)
	  // 双结案件
	   g.GET("/dup-cases/:transfer_id", h.listDupCases)
	  g.POST("/dup-cases/:dup_case_id/confirm", h.confirmDupCase)
	   // 证明查询与导出
	   g.GET("/proof/:transfer_id", h.getProof)
	   g.GET("/proof/export", h.exportProof)
	   // 互斥锁快照（运维）
	  g.GET("/mutex/snapshot", h.mutexSnapshot)
	  // 双结手动扫描触发（运维）
	  g.POST("/scan/:transfer_id", h.triggerScan)
	   _ = rt
  }

type httpHandler struct {
	gate *Gate
}

// GET /v1/stucksettle/gen/:transfer_id

  func (h *httpHandler) getGen(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	  trID := c.Param("transfer_id")
	  gen, err := h.gate.GenStatus(trID)
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	   c.JSON(http.StatusOK, gen)
  }

// GET /v1/stucksettle/dup-cases/:transfer_id

  func (h *httpHandler) listDupCases(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	   trID := c.Param("transfer_id")
	   cases, err := h.gate.DupCasesByTransfer(trID)
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, gin.H{"dup_cases": cases, "count": len(cases)})
  }

// POST /v1/stucksettle/dup-cases/:dup_case_id/confirm
   func (h *httpHandler) confirmDupCase(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	   dupCaseID := c.Param("dup_case_id")
	  var body struct {
		  ConfirmedBy   string `json:"confirmed_by"`
		  CoConfirmedBy string `json:"co_confirmed_by"`
		  Note          string `json:"note"`
	  }
	  if !mw.BindJSON(c, &body) {
		  return
	  }
	  actor := perm.FromRequest(c.Request, "").ID
	  if actor != "" {
		   body.ConfirmedBy = actor
	   }
	  req := ConfirmDupCaseReq{
		  DupCaseID:     dupCaseID,
		  ConfirmedBy:   body.ConfirmedBy,
		   CoConfirmedBy: body.CoConfirmedBy,
		   Note:          body.Note,
	   }
	   result, err := h.gate.ConfirmDupCase(req)
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	   MetricDupCaseClosed.Inc()
	   c.JSON(http.StatusOK, result)
  }

// GET /v1/stucksettle/proof/:transfer_id

   func (h *httpHandler) getProof(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	   trID := c.Param("transfer_id")
	   proof, err := h.gate.engine.GetEffectProof(trID)
	   if err != nil {
		   mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, Summarize(proof))
   }

// GET /v1/stucksettle/proof/export?transfer_id=xxx&lease_id=yyy
   func (h *httpHandler) exportProof(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	  req := ProofExportReq{
		  TransferID: c.Query("transfer_id"),
		   LeaseID: c.Query("lease_id"),
	   }
	   export,
		  err := h.gate.ExportProof(req)
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	   c.JSON(http.StatusOK, export)
  }

// GET /v1/stucksettle/mutex/snapshot

  func (h *httpHandler) mutexSnapshot(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	  snap := h.gate.MutexSnapshot()
	  c.JSON(http.StatusOK, gin.H{"locks": snap, "count": len(snap)})
   }

// POST /v1/stucksettle/scan/:transfer_id

   func (h *httpHandler) triggerScan(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	   trID := c.Param("transfer_id")
	  result, err := h.gate.ScanDups(trID)
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	   }
	   c.JSON(http.StatusOK, result)
  }
