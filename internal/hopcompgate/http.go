package hopcompgate

import (
	"net/http"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
)

// Register 挂载 hopcompgate 路由到 Gin 路由组。
  func Register(v1 *gin.RouterGroup, g *Gate) {
	   if g == nil {
		  g = New()
	   }
	  h := &handler{gate: g}
	   v1.POST("/hopcompgate/cases/query", h.queryCases)
	   v1.GET("/hopcompgate/cases/:id", h.getCase)
	   v1.POST("/hopcompgate/compensate", h.compensate)
	  v1.POST("/hopcompgate/force-compensate", h.forceCompensate)
	   v1.POST("/hopcompgate/proofs/export", h.exportProofs)
	   v1.POST("/hopcompgate/leases/acquire", h.acquireLease)
	  v1.GET("/hopcompgate/leases/:id", h.getLease)
   }

type handler struct {
	gate *Gate
}

// ──────────────────────────────────────────────────────────────────────────────
// 补偿仓查询
// ──────────────────────────────────────────────────────────────────────────────

  func (h *handler) queryCases(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	  var body CaseQueryReq
	   if !mw.BindJSON(c, &body) {
		  return
	  }
	   cases, err := h.gate.QueryCases(c.Request.Context(), body)
	   if err != nil {
		   mw.WriteProblem(c, err)
		   return
	   }
	   c.JSON(http.StatusOK, gin.H{"cases": cases, "count": len(cases)})
   }

  func (h *handler) getCase(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	  id := c.Param("id")
	   cas,
		  err := h.gate.GetCase(c.Request.Context(), id)
	   if err != nil {
		   mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, cas)
  }

// ──────────────────────────────────────────────────────────────────────────────
// 执行补偿
// ──────────────────────────────────────────────────────────────────────────────

   func (h *handler) compensate(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	  var body CompensateReq
	   if !mw.BindJSON(c, &body) {
		  return
	   }
	  cas, err := h.gate.ExecuteCompensate(c.Request.Context(), body)
	   if err != nil {
		   mw.WriteProblem(c, err)
		   return
	   }
	  c.JSON(http.StatusOK, cas)
  }

// ──────────────────────────────────────────────────────────────────────────────
// 双人强制补偿
// ──────────────────────────────────────────────────────────────────────────────

   func (h *handler) forceCompensate(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	  var body ForceCompensateReq
	   if !mw.BindJSON(c, &body) {
		  return
	  }
	   cas, err := h.gate.ForceCompensate(c.Request.Context(), body)
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, cas)
   }

// ──────────────────────────────────────────────────────────────────────────────
// 双移证明导出
// ──────────────────────────────────────────────────────────────────────────────

  func (h *handler) exportProofs(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	  var body ProofExportReq
	  if !mw.BindJSON(c, &body) {
		   return
	   }
	  proofs, err := h.gate.ExportProofs(c.Request.Context(), body)
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, gin.H{"proofs": proofs, "count": len(proofs)})
   }

// ──────────────────────────────────────────────────────────────────────────────
// 租约管理
// ──────────────────────────────────────────────────────────────────────────────

   func (h *handler) acquireLease(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	   var body AcquireLeaseReq
	   if !mw.BindJSON(c, &body) {
		   return
	  }
	   lease, err := h.gate.AcquireLease(c.Request.Context(), body)
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, lease)
   }

   func (h *handler) getLease(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	   id := c.Param("id")
	   lease,
		  err := h.gate.GetLease(c.Request.Context(), id)
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, lease)
   }
