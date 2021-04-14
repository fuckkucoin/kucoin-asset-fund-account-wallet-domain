package disputepaygate

import (
	"net/http"
	"strings"
	"time"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
)

type handler struct {
	eng    *Engine
	binder *Binder
}

// RegisterRoutes 挂载结案租约路由。

   func RegisterRoutes(rg *gin.RouterGroup, eng *Engine, binder *Binder) {
	   h := &handler{eng: eng,
		  binder: binder}
	  if h.binder == nil {
		   h.binder = NewBinder(eng, NewMemoryPoster())
	  }
	  rg.POST("/dispute-pay/resolve", h.resolve)
	  rg.POST("/dispute-pay/assert", h.assertPay)
	   rg.POST("/dispute-pay/assert-claw", h.assertClaw)
	   rg.POST("/dispute-pay/credit", h.credit)
	  rg.POST("/dispute-pay/debit", h.debit)
	   rg.POST("/dispute-pay/consume", h.consume)
	   rg.POST("/dispute-pay/revoke", h.revoke)
	   rg.GET("/dispute-pay/lease/:lease_id", h.getLease)
	   rg.GET("/dispute-pay/claw/:lease_id", h.getClaw)
	  rg.GET("/dispute-pay/proof/:proof_id", h.getProof)
	  rg.GET("/dispute-pay/proofs", h.listProofs)
	   rg.GET("/dispute-pay/audit", h.listAudit)
  }

  func (h *handler) resolve(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActAMLClose); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	   var body struct {
		  ID       string `json:"id"`
		  Actor    string `json:"actor"`
		  Decision string `json:"decision"`
		   Note string `json:"note"`
		  TTL string `json:"ttl"`
		  ReleaseRef string `json:"release_ref"`
	  }
	   if !mw.BindJSON(c, &body) {
		   return
	   }
	  if body.Actor == "" {
		   body.Actor = perm.FromRequest(c.Request, "ops-reviewer").ID
	   }
	   var ttl time.Duration
	  if body.TTL != "" {
		  d, err := time.ParseDuration(body.TTL)
		   if err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": "ttl"})
			  return
		  }
		   ttl = d
	  }
	   res, err := h.eng.ResolveWithPayLease(c.Request.Context(), ResolveReq{
		  CaseID: body.ID, Actor: body.Actor, Decision: body.Decision, Note: body.Note, TTL: ttl, ReleaseRef: body.ReleaseRef,
	  })
	   if err != nil {
		  if res != nil && res.ProofID != "" {
			   c.JSON(http.StatusConflict, gin.H{
				   "error": err.Error(), "proof_id": res.ProofID, "dispute_pay_lease_id": res.PayLeaseID,
			  })
			  return
		   }
		  mw.WriteProblem(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, gin.H{
		  "case": res.Case,
		  "kind": string(res.Kind),
		   "dispute_pay_lease_id": res.PayLeaseID,
		  "proof_id": res.ProofID,
		   "pay_lease":  res.PayLease,
		  "claw_lease": res.ClawLease,
	  })
   }

  func (h *handler) assertPay(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	  var body struct {
		  LeaseID string `json:"dispute_pay_lease_id"`
		   CaseID string `json:"case_id"`
		  Amount string `json:"amount"`
		  Asset string `json:"asset"`
		   LedgerID string `json:"ledger_id"`
	  }
	  if !mw.BindJSON(c, &body) {
		   return
	   }
	   res, err := h.eng.AssertPayLease(c.Request.Context(), AssertPayInput{
		  LeaseID: body.LeaseID, CaseID: body.CaseID, Amount: body.Amount, Asset: body.Asset, LedgerID: body.LedgerID,
	   })
	   if err != nil {
		  c.JSON(http.StatusConflict, gin.H{
			   "allowed": false, "reason": reasonOf(res), "detail": detailOf(res),
			  "proof_id": proofOf(res), "dispute_pay_lease_id": body.LeaseID,
			  "error": err.Error(),
		  })
		  return
	   }
	   c.JSON(http.StatusOK, gin.H{"allowed": true, "dispute_pay_lease_id": res.PayLeaseID, "lease": res.Lease})
   }

   func (h *handler) assertClaw(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	   var body struct {
		  LeaseID string `json:"claw_lease_id"`
		  CaseID  string `json:"case_id"`
		   Amount  string `json:"amount"`
		   Asset   string `json:"asset"`
		   LedgerID string `json:"ledger_id"`
	   }
	  if !mw.BindJSON(c, &body) {
		  return
	   }
	  res, err := h.eng.AssertClawLease(c.Request.Context(), AssertPayInput{
		   LeaseID: body.LeaseID, CaseID: body.CaseID, Amount: body.Amount, Asset: body.Asset, LedgerID: body.LedgerID,
	   })
	  if err != nil {
		  c.JSON(http.StatusConflict, gin.H{
			  "allowed": false, "reason": reasonOf(res), "proof_id": proofOf(res), "error": err.Error(),
		   })
		   return
	  }
	  c.JSON(http.StatusOK, gin.H{"allowed": true, "claw_lease_id": res.PayLeaseID, "lease": res.Claw})
   }

   func (h *handler) credit(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	   var body LedgerPost
	   if !mw.BindJSON(c, &body) {
		  return
	   }
	  if body.LeaseID == "" {
		  body.LeaseID = c.PostForm("dispute_pay_lease_id")
	  }
	   got, err := h.binder.CreditWithLease(c.Request.Context(), body)
	   if err != nil {
		  c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "entry": got})
		  return
	   }
	  c.JSON(http.StatusOK, got)
  }

   func (h *handler) debit(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	  var body LedgerPost
	  if !mw.BindJSON(c, &body) {
		  return
	   }
	  got,
		   err := h.binder.DebitWithClaw(c.Request.Context(), body)
	   if err != nil {
		  c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "entry": got})
		  return
	   }
	  c.JSON(http.StatusOK, got)
   }

   func (h *handler) consume(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	  var body struct {
		  LeaseID  string `json:"dispute_pay_lease_id"`
		  ClawID   string `json:"claw_lease_id"`
		  LedgerID string `json:"ledger_id"`
	   }
	   if !mw.BindJSON(c, &body) {
		   return
	   }
	  if strings.TrimSpace(body.ClawID) != "" {
		   l, err := h.eng.ConsumeClawLease(c.Request.Context(), body.ClawID, body.LedgerID)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, l)
		   return
	  }
	  l, err := h.eng.ConsumePayLease(c.Request.Context(), body.LeaseID, body.LedgerID)
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, l)
  }

  func (h *handler) revoke(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	   var body struct {
		   LeaseID string `json:"dispute_pay_lease_id"`
		   Actor   string `json:"actor"`
		  Note    string `json:"note"`
	   }
	   if !mw.BindJSON(c, &body) {
		   return
	   }
	   if err := h.eng.RevokeLease(c.Request.Context(), body.LeaseID, body.Actor, body.Note); err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, gin.H{"revoked": true, "dispute_pay_lease_id": body.LeaseID})
   }

   func (h *handler) getLease(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	  l, err := h.eng.GetPayLease(c.Request.Context(), c.Param("lease_id"))
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, l)
  }

   func (h *handler) getClaw(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	   l, err := h.eng.GetClawLease(c.Request.Context(), c.Param("lease_id"))
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	   c.JSON(http.StatusOK, l)
   }

   func (h *handler) getProof(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	  p, err := h.eng.ExportProof(c.Request.Context(), c.Param("proof_id"))
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	   c.JSON(http.StatusOK, p.Export())
   }

  func (h *handler) listProofs(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	   rows, err := h.eng.ListProofs(c.Request.Context(), c.Query("case_id"), 50)
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, gin.H{"proofs": SortProofs(rows)})
  }

  func (h *handler) listAudit(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	   rows, err := h.eng.ListAudit(c.Request.Context(), c.Query("case_id"), 50)
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, gin.H{"audit": rows})
   }

  func reasonOf(ar *AssertResult) string {
	  if ar == nil {
		   return ""
	   }
	  return string(ar.Reason)
  }

  func detailOf(ar *AssertResult) string {
	  if ar == nil {
		  return ""
	   }
	   return ar.Detail
   }
