package subfreezecas

import (
	"net/http"
	"strconv"
	"strings"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/subacct"
)

type handler struct {
	eng *Engine
}

// RegisterRoutes 挂载冻结 CAS 路由。
  func RegisterRoutes(rg *gin.RouterGroup, eng *Engine) {
	   h := &handler{eng: eng}
	   rg.POST("/subacct/cas/parents", h.openParent)
	   rg.POST("/subacct/cas/children", h.openChild)
	  rg.POST("/subacct/cas/credit", h.credit)
	   rg.POST("/subacct/cas/freeze-parent", h.freezeParent)
	   rg.POST("/subacct/cas/freeze-child", h.freezeChild)
	   rg.POST("/subacct/cas/unfreeze-parent", h.unfreezeParent)
	  rg.POST("/subacct/cas/unfreeze-child", h.unfreezeChild)
	  rg.POST("/subacct/cas/debit", h.debit)
	  rg.GET("/subacct/cas/:freeze_cas_id", h.getCAS)
	   rg.GET("/subacct/cas/current", h.current)
	  rg.GET("/subacct/cas/proof/:proof_id", h.getProof)
	   rg.GET("/subacct/cas/proofs", h.listProofs)
	  rg.POST("/subacct/cas/proof/ack", h.ack)
	  rg.GET("/subacct/cas/export/:freeze_cas_id", h.exportCAS)
	  rg.GET("/subacct/cas/audit", h.audit)
	  rg.GET("/subacct/cas/revocations", h.revocations)
	   rg.POST("/subacct/cas/inspect", h.inspect)
   }

  func (h *handler) openParent(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	  var body subacct.OpenParentReq
	  if !mw.BindJSON(c, &body) {
		  return
	  }
	   got, err := h.eng.Books().OpenParent(body)
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, got)
   }

  func (h *handler) openChild(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	   var body subacct.OpenChildReq
	   if !mw.BindJSON(c, &body) {
		  return
	  }
	   got, err := h.eng.Books().OpenChild(body)
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, got)
   }

  func (h *handler) credit(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	   var body subacct.PostReq
	   if !mw.BindJSON(c, &body) {
		   return
	   }
	   got, err := h.eng.Books().Credit(body)
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, got)
   }

   func (h *handler) freezeParent(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountFreeze); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	   var body FreezeParentReq
	  if !mw.BindJSON(c, &body) {
		   return
	  }
	  got, err := h.eng.FreezeParentWithCAS(c.Request.Context(), body)
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, got)
   }

  func (h *handler) freezeChild(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountFreeze); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	  var body FreezeChildReq
	   if !mw.BindJSON(c, &body) {
		   return
	  }
	  got, err := h.eng.FreezeChild(c.Request.Context(), body)
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, got)
  }

   func (h *handler) unfreezeParent(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountFreeze); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	   var body struct {
		  ParentID string `json:"parent_id"`
		   Actor    string `json:"actor"`
	  }
	  if !mw.BindJSON(c, &body) {
		  return
	  }
	  got,
		  err := h.eng.UnfreezeParent(c.Request.Context(), body.ParentID, body.Actor)
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	   }
	  c.JSON(http.StatusOK, got)
  }

  func (h *handler) unfreezeChild(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountFreeze); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	   var body struct {
		  ChildID string `json:"child_id"`
		  Actor   string `json:"actor"`
	  }
	   if !mw.BindJSON(c, &body) {
		   return
	  }
	   if err := h.eng.UnfreezeChild(c.Request.Context(), body.ChildID, body.Actor); err != nil {
		  mw.WriteProblem(c, err)
		   return
	   }
	  c.JSON(http.StatusOK, gin.H{"ok": true, "child_id": body.ChildID})
  }

  func (h *handler) debit(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	  var body DebitCASReq
	  if !mw.BindJSON(c, &body) {
		   return
	  }
	   got, err := h.eng.DebitAssertCAS(c.Request.Context(), body)
	   if err != nil {
		   mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, got)
   }

  func (h *handler) getCAS(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	  got, err := h.eng.GetCAS(c.Request.Context(), c.Param("freeze_cas_id"))
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, got)
   }

   func (h *handler) current(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountRead); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	   got, err := h.eng.CurrentCAS(c.Request.Context(), c.Query("parent_id"))
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, got)
  }

   func (h *handler) getProof(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountRead); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	   got, err := h.eng.GetProof(c.Request.Context(), c.Param("proof_id"))
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, got)
   }

  func (h *handler) listProofs(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountRead); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	   if casID := strings.TrimSpace(c.Query("freeze_cas_id")); casID != "" {
		   lim, _ := strconv.Atoi(c.Query("limit"))
		  got, err := h.eng.ListProofsByCAS(c.Request.Context(), casID, lim)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, gin.H{"proofs": got, "host": "sub-account.kucoin.net"})
		  return
	  }
	   got, err := h.eng.ListOpenProofs(c.Request.Context(), 50)
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, gin.H{"proofs": got, "host": "sub-account.kucoin.net"})
   }

   func (h *handler) ack(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	   var body struct {
		   ProofID string `json:"proof_id"`
		   Actor   string `json:"actor"`
		   Note string `json:"note"`
	   }
	  if !mw.BindJSON(c, &body) {
		   return
	  }
	   got,
		   err := h.eng.AckProof(c.Request.Context(), body.ProofID, body.Actor, body.Note)
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, got)
  }

  func (h *handler) exportCAS(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	  got, err := h.eng.ExportCAS(c.Request.Context(), c.Param("freeze_cas_id"))
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, got)
   }

   func (h *handler) audit(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	   lim, _ := strconv.Atoi(c.Query("limit"))
	   got, err := h.eng.ListAudit(c.Request.Context(), c.Query("parent_id"), lim)
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, gin.H{"audits": got, "host": "sub-account.kucoin.net"})
   }

   func (h *handler) revocations(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountRead); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	  got, err := h.eng.ListRevocations(c.Request.Context(), c.Query("freeze_cas_id"))
	   if err != nil {
		   mw.WriteProblem(c, err)
		   return
	   }
	   c.JSON(http.StatusOK, gin.H{"revocations": got})
   }

   func (h *handler) inspect(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	   n, err := h.eng.InspectPunch(c.Request.Context(), 80)
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	   c.JSON(http.StatusOK, gin.H{"fixed": n})
   }
