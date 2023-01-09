package wdtriplegate

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// HTTPHandler wdtriplegate HTTP 处理器。
type HTTPHandler struct {
	gate *Gate
}

// NewHTTPHandler 构造 HTTP 处理器。
  func NewHTTPHandler(gate *Gate) *HTTPHandler {
	   if gate == nil {
		   gate = NewGate(GateConfig{})
	  }
	   return &HTTPHandler{gate: gate}
   }

// RegisterRoutes 注册路由到指定 RouterGroup。
// 前缀：/wd-triple-gate

   func (h *HTTPHandler) RegisterRoutes(g *gin.RouterGroup) {
	  tg := g.Group("/wd-triple-gate")
	   // ── 租约查询（只读）──────────────────────────────────────────────────────
	   // GET /wd-triple-gate/lease/:movement_id — 查询 movement 完整租约状态
	  tg.GET("/lease/:movement_id", h.getLeaseStatus)
	   // GET /wd-triple-gate/lease/id/:lease_id — 按 lease_id 查询
	   tg.GET("/lease/id/:lease_id", h.getLeaseByID)
	  // GET /wd-triple-gate/lease/:movement_id/history — 租约历史列表
	  tg.GET("/lease/:movement_id/history", h.leaseHistory)
	   // GET /wd-triple-gate/bind/:movement_id — 查询预检绑定记录
	   tg.GET("/bind/:movement_id", h.getBindRecord)
	   // ── 漂移案件（只读）─────────────────────────────────────────────────────
	   // GET /wd-triple-gate/drift-cases — 列出全部 Open 漂移案件
	  tg.GET("/drift-cases", h.listOpenDriftCases)
	   // GET /wd-triple-gate/drift-cases/:drift_case_id — 查询指定案件
	   tg.GET("/drift-cases/:drift_case_id", h.getDriftCase)
	   // GET /wd-triple-gate/drift-cases/by-movement/:movement_id — 按 movement 查案件
	   tg.GET("/drift-cases/by-movement/:movement_id", h.driftCasesByMovement)
	  // ── 漂移案件操作（需 OpsAdmin）──────────────────────────────────────────
	   // POST /wd-triple-gate/drift-cases/:drift_case_id/attach-proof — 附上证明摘要
	  tg.POST("/drift-cases/:drift_case_id/attach-proof", h.attachDriftProof)
	  // POST /wd-triple-gate/drift-cases/:drift_case_id/confirm-close — 双人确认关闭
	   tg.POST("/drift-cases/:drift_case_id/confirm-close", h.confirmDriftClose)
	  // ── 运维操作（需 OpsAdmin）──────────────────────────────────────────────
	  // POST /wd-triple-gate/lease/id/:lease_id/force-expire — 强制将租约标记为 Expired（触发重检）
	  tg.POST("/lease/id/:lease_id/force-expire", h.forceExpireLease)
	   // POST /wd-triple-gate/lease/:movement_id/consume — 广播成功后消费租约
	   tg.POST("/lease/:movement_id/consume", h.consumeLease)
   }

// GET /wd-triple-gate/lease/:movement_id

   func (h *HTTPHandler) getLeaseStatus(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	   movementID := c.Param("movement_id")
	   resp, err := h.gate.LeaseStatus(movementID)
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	   c.JSON(http.StatusOK, resp)
  }

// GET /wd-triple-gate/lease/id/:lease_id
   func (h *HTTPHandler) getLeaseByID(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	  leaseID := c.Param("lease_id")
	   lease, err := h.gate.engine.GetLease(leaseID)
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, lease)
  }

// GET /wd-triple-gate/lease/:movement_id/history
   func (h *HTTPHandler) leaseHistory(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	  movementID := c.Param("movement_id")
	  leases, err := h.gate.engine.ListLeasesByMovement(movementID)
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, gin.H{"leases": leases})
  }

// GET /wd-triple-gate/bind/:movement_id
   func (h *HTTPHandler) getBindRecord(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	   movementID := c.Param("movement_id")
	   bind, err := h.gate.binder.GetBind(movementID)
	   if err != nil {
		   mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, bind)
   }

// GET /wd-triple-gate/drift-cases

  func (h *HTTPHandler) listOpenDriftCases(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	   cases,
		  err := h.gate.ListOpenDriftCases(100)
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, gin.H{"drift_cases": cases})
  }

// GET /wd-triple-gate/drift-cases/:drift_case_id

   func (h *HTTPHandler) getDriftCase(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	  driftCaseID := c.Param("drift_case_id")
	  dc, err := h.gate.GetDriftCase(driftCaseID)
	  if err != nil {
		   mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, dc)
   }

// GET /wd-triple-gate/drift-cases/by-movement/:movement_id
   func (h *HTTPHandler) driftCasesByMovement(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	   movementID := c.Param("movement_id")
	  cases, err := h.gate.detector.ListByMovement(movementID)
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, gin.H{"drift_cases": cases})
  }

// POST /wd-triple-gate/drift-cases/:drift_case_id/attach-proof

   func (h *HTTPHandler) attachDriftProof(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	   driftCaseID := c.Param("drift_case_id")
	   var body struct {
		   ProofChecksum string `json:"proof_checksum"`
	   }
	  if !mw.BindJSON(c, &body) {
		   return
	  }
	  dc, err := h.gate.AttachDriftProof(driftCaseID, body.ProofChecksum)
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, dc)
  }

// POST /wd-triple-gate/drift-cases/:drift_case_id/confirm-close
   func (h *HTTPHandler) confirmDriftClose(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	   driftCaseID := c.Param("drift_case_id")
	  var body struct {
		  ConfirmedBy string `json:"confirmed_by"`
		   CoConfirmedBy string `json:"co_confirmed_by"`
		   Note          string `json:"note,omitempty"`
	  }
	   if !mw.BindJSON(c, &body) {
		  return
	   }
	   dc, err := h.gate.ConfirmDriftClose(ConfirmDriftCaseReq{
		  DriftCaseID: driftCaseID,
		  ConfirmedBy: body.ConfirmedBy,
		  CoConfirmedBy: body.CoConfirmedBy,
		   Note:          body.Note,
	   })
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, dc)
  }

// POST /wd-triple-gate/lease/id/:lease_id/force-expire
   func (h *HTTPHandler) forceExpireLease(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	   leaseID := c.Param("lease_id")
	   lease, err := h.gate.ForceExpire(leaseID)
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	   }
	  c.JSON(http.StatusOK, lease)
  }

// POST /wd-triple-gate/lease/:movement_id/consume
   func (h *HTTPHandler) consumeLease(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	   movementID := c.Param("movement_id")
	   var body struct {
		   TripleLeaseID string `json:"triple_lease_id"`
	   }
	  if !mw.BindJSON(c, &body) {
		  return
	  }
	   lease, err := h.gate.ConsumeLease(body.TripleLeaseID)
	   if err != nil {
		   mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, gin.H{"lease": lease, "movement_id": movementID})
   }
// temporary guard removed after the production check
