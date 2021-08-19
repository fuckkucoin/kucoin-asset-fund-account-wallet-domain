package keyrotgate

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

   func init() {
	   apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	   apierr.Register(ErrInvalidTransition, http.StatusConflict)
	  apierr.Register(ErrLeaseNotFound, http.StatusNotFound)
	   apierr.Register(ErrLeaseExpired, http.StatusGone)
	  apierr.Register(ErrLeaseBlocked,
		   http.StatusForbidden)
	   apierr.Register(ErrLeaseTerminal, http.StatusConflict)
	   apierr.Register(ErrLeaseIDRequired,
		  http.StatusBadRequest)
	  apierr.Register(ErrTicketIDRequired, http.StatusBadRequest)
	   apierr.Register(ErrSlotIDRequired, http.StatusBadRequest)
	   apierr.Register(ErrFingerprintRequired, http.StatusBadRequest)
	   apierr.Register(ErrActorRequired, http.StatusBadRequest)
	   apierr.Register(ErrFourEye, http.StatusForbidden)
	   apierr.Register(ErrFingerprintMismatch, http.StatusConflict)
	   apierr.Register(ErrRotateGenMismatch, http.StatusConflict)
	  apierr.Register(ErrInflightSignExists, http.StatusConflict)
	   apierr.Register(ErrSignBindNotFound,
		   http.StatusNotFound)
	  apierr.Register(ErrSignBindTerminal, http.StatusConflict)
	   apierr.Register(ErrOperationIDRequired, http.StatusBadRequest)
	   apierr.Register(ErrInvalidKind, http.StatusBadRequest)
	   apierr.Register(ErrSlotRetiredReject,
		  http.StatusForbidden)
	   apierr.Register(ErrDriftCaseNotFound, http.StatusNotFound)
	  apierr.Register(ErrDriftCaseTerminal, http.StatusConflict)
	  apierr.Register(ErrProofChecksumEmpty,
		   http.StatusBadRequest)
	  apierr.Register(ErrDualConfirm, http.StatusBadRequest)
	  apierr.Register(ErrOpenDriftExists, http.StatusForbidden)
	  apierr.Register(ErrProofNotFound, http.StatusNotFound)
	   apierr.Register(ErrProofRequired, http.StatusForbidden)
  }

// Register 将 keyrotgate 路由挂载到 v1 RouterGroup。
  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  var store Store = NewMemStore()
	   if rt != nil && !rt.Mem && rt.SQL != nil && rt.SQL.DB != nil {
		  store = NewSQLStore(rt.SQL.DB)
	   }
	   gate := NewGate(store, nil)
	  h := &httpHandler{gate: gate}
	  h.mount(v1)
  }

// httpHandler keyrotgate HTTP 处理器（内部类型）。
type httpHandler struct {
	gate *Gate
}

// mount 注册路由到指定 RouterGroup。前缀：/key-rot-gate
   func (h *httpHandler) mount(v1 *gin.RouterGroup) {
	  g := v1.Group("/key-rot-gate")
	  // ── 轮换租约（写操作需 ActCeremony）────────────────────────────────────
	   g.POST("/leases/open", h.openLease)
	  g.POST("/leases/:lease_id/approve", h.approveLease)
	   g.POST("/leases/:lease_id/execute", h.executeLease)
	   g.GET("/leases/:lease_id", h.getLease)
	  g.GET("/leases/by-ticket/:ticket_id", h.getLeaseByTicket)
	  g.GET("/slots/:slot_id/leases", h.listActiveLeasesBySlot)
	   // ── 在途签名绑定 ─────────────────────────────────────────────────────────
	   g.POST("/sign-binds/bind", h.bindSign)
	   g.POST("/sign-binds/:bind_id/release", h.releaseSign)
	   g.GET("/sign-binds/:bind_id", h.getSignBind)
	   g.GET("/slots/:slot_id/sign-binds/active", h.listActiveSignBinds)
	  // ── 漂移案件 ─────────────────────────────────────────────────────────────
	  g.GET("/drift-cases/:case_id", h.getDriftCase)
	  g.GET("/slots/:slot_id/drift-cases/open", h.listOpenDriftCases)
	  g.POST("/drift-cases/:case_id/prove", h.proveCase)
	   g.POST("/drift-cases/:case_id/close", h.closeCase)
	   // ── 指纹证明 ─────────────────────────────────────────────────────────────
	  g.GET("/proofs/:proof_id", h.getProof)
	  g.GET("/leases/:lease_id/proof", h.getProofByLease)
  }

// ── 轮换租约处理器 ────────────────────────────────────────────────────────────

   func (h *httpHandler) openLease(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActCeremony); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	  var req OpenLeaseReq
	  if !mw.BindJSON(c, &req) {
		   return
	   }
	  l, err := h.gate.OpenLease(c.Request.Context(), req)
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, l)
   }

  func (h *httpHandler) approveLease(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActCeremony); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	   leaseID := c.Param("lease_id")
	  var body struct {
		  ApprovedBy string `json:"approved_by"`
	   }
	  if !mw.BindJSON(c, &body) {
		   return
	  }
	  l, err := h.gate.ApproveLease(c.Request.Context(), ApproveLeaseReq{
		  RotateLeaseID: leaseID,
		   ApprovedBy: body.ApprovedBy,
	  })
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, l)
   }

   func (h *httpHandler) executeLease(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActCeremony); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	   leaseID := c.Param("lease_id")
	  var body struct {
		   ExecutedBy string `json:"executed_by"`
	  }
	  if !mw.BindJSON(c, &body) {
		  return
	   }
	  // HTTP 层不注入 slotOpFn；Execute 仅推进状态并产出证明（操作由 keycer 调用层处理）
	   // 此接口供运维查询/触发执行，实际槽操作由 key-cer 服务内部调用 Gate.ExecuteRotate
	  c.JSON(http.StatusNotImplemented, gin.H{
		  "error":       "execute must be called internally via keycer integration; use /leases/:id for status",
		   "lease_id":    leaseID,
		  "executed_by": body.ExecutedBy,
	  })
   }

   func (h *httpHandler) getLease(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	   l, err := h.gate.GetLease(c.Request.Context(), c.Param("lease_id"))
	  if err != nil {
		   mw.WriteProblem(c, err)
		   return
	   }
	   c.JSON(http.StatusOK, l)
   }

   func (h *httpHandler) getLeaseByTicket(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	   l, err := h.gate.store.GetLeaseByTicket(c.Request.Context(), c.Param("ticket_id"))
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, l)
   }

   func (h *httpHandler) listActiveLeasesBySlot(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	   leases, err := h.gate.store.ListActiveLeasesBySlot(c.Request.Context(), c.Param("slot_id"))
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, gin.H{"leases": leases})
  }

// ── 在途签名绑定处理器 ────────────────────────────────────────────────────────

   func (h *httpHandler) bindSign(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActCeremony); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	  var req BindSignReq
	   if !mw.BindJSON(c, &req) {
		  return
	  }
	   b, err := h.gate.BindSign(c.Request.Context(), req)
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, b)
  }

  func (h *httpHandler) releaseSign(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActCeremony); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	   b, err := h.gate.ReleaseSign(c.Request.Context(), ReleaseSignReq{
		   SignBindID: c.Param("bind_id"),
	  })
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, b)
  }

  func (h *httpHandler) getSignBind(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	  b, err := h.gate.GetSignBind(c.Request.Context(), c.Param("bind_id"))
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	   }
	   c.JSON(http.StatusOK, b)
   }

  func (h *httpHandler) listActiveSignBinds(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	  binds, err := h.gate.store.ListActiveSignBinds(c.Request.Context(), c.Param("slot_id"))
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, gin.H{"sign_binds": binds})
  }

// ── 漂移案件处理器 ────────────────────────────────────────────────────────────

   func (h *httpHandler) getDriftCase(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	   d, err := h.gate.GetDriftCase(c.Request.Context(), c.Param("case_id"))
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, d)
  }

   func (h *httpHandler) listOpenDriftCases(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	  cases, err := h.gate.ListOpenDriftCases(c.Request.Context(), c.Param("slot_id"))
	   if err != nil {
		   mw.WriteProblem(c, err)
		   return
	   }
	   c.JSON(http.StatusOK, gin.H{"drift_cases": cases})
  }

  func (h *httpHandler) proveCase(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActCeremony); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	   caseID := c.Param("case_id")
	  var body struct {
		  ProofChecksum string `json:"proof_checksum"`
		   ConfirmedBy   string `json:"confirmed_by"`
		   CoConfirmedBy string `json:"co_confirmed_by"`
		   Note          string `json:"note"`
	   }
	   if !mw.BindJSON(c, &body) {
		   return
	  }
	  d, err := h.gate.ProveCase(c.Request.Context(), ProveRotateReq{
		  DriftCaseID:   caseID,
		   ProofChecksum: body.ProofChecksum,
		   ConfirmedBy: body.ConfirmedBy,
		   CoConfirmedBy: body.CoConfirmedBy,
		   Note: body.Note,
	   })
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, d)
   }

   func (h *httpHandler) closeCase(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActCeremony); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	   caseID := c.Param("case_id")
	   var body struct {
		   ClosedBy string `json:"closed_by"`
	   }
	  if !mw.BindJSON(c, &body) {
		   return
	   }
	   d, err := h.gate.CloseCase(c.Request.Context(), CloseDriftCaseReq{
		  DriftCaseID: caseID,
		   ClosedBy: body.ClosedBy,
	   })
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	   }
	   c.JSON(http.StatusOK, d)
  }

// ── 指纹证明处理器 ────────────────────────────────────────────────────────────

   func (h *httpHandler) getProof(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	  p, err := h.gate.GetProof(c.Request.Context(), c.Param("proof_id"))
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, p)
  }

   func (h *httpHandler) getProofByLease(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	   p, err := h.gate.GetProofByLease(c.Request.Context(), c.Param("lease_id"))
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, p)
  }
