package stablemintgate

import (
	"net/http"
	"strconv"
	"strings"
	"time"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/stablebr"
)

type handler struct {
	eng *Engine
}

// RegisterRoutes 挂载铸赎租约路由。
   func RegisterRoutes(rg *gin.RouterGroup, eng *Engine) {
	   h := &handler{eng: eng}
	  rg.POST("/stable-mint/attest", h.attest)
	  rg.POST("/stable-mint/attest-clear", h.clear)
	  rg.POST("/stable-mint/issue", h.issue)
	   rg.POST("/stable-mint/assert", h.assertLease)
	   rg.POST("/stable-mint/mint", h.mint)
	   rg.POST("/stable-mint/redeem", h.redeem)
	  rg.POST("/stable-mint/revoke", h.revoke)
	   rg.POST("/stable-mint/half/resolve", h.resolveHalf)
	   rg.GET("/stable-mint/lease/:lease_id", h.getLease)
	  rg.GET("/stable-mint/proof/:proof_id", h.getProof)
	  rg.GET("/stable-mint/proofs", h.listProofs)
	   rg.GET("/stable-mint/half", h.listHalf)
	  rg.GET("/stable-mint/export/:lease_id", h.exportBundle)
	  rg.GET("/stable-mint/audit", h.listAudit)
	   rg.GET("/stable-mint/checklist", h.checklist)
	  rg.GET("/stable-mint/fingerprint", h.fingerprint)
  }

   func (h *handler) attest(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	  var body stablebr.AttestReq
	   if !mw.BindJSON(c, &body) {
		  return
	   }
	  got, err := h.eng.Bridge().Attest(body)
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, got)
  }

  func (h *handler) clear(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	  var body struct {
		   Kind  string `json:"kind"`
		  Asset string `json:"asset"`
		   Actor string `json:"actor"`
	   }
	   if !mw.BindJSON(c, &body) {
		   return
	   }
	  if err := h.eng.Bridge().ClearAttest(body.Kind, body.Asset, body.Actor); err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, gin.H{"cleared": true})
   }

   func (h *handler) issue(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	   var body struct {
		  Kind string `json:"kind"`
		  Asset string `json:"asset"`
		   Actor  string `json:"actor"`
		  UserID string `json:"user_id"`
		  Amount string `json:"amount"`
		   TTL string `json:"ttl"`
	  }
	  if !mw.BindJSON(c, &body) {
		  return
	   }
	   if body.Actor == "" {
		   body.Actor = perm.FromRequest(c.Request, "ops-stable").ID
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
	  lease, err := h.eng.IssueMintLease(c.Request.Context(), IssueReq{
		  Kind: body.Kind, Asset: body.Asset, Actor: body.Actor, UserID: body.UserID, Amount: body.Amount, TTL: ttl,
	  })
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	   }
	  c.JSON(http.StatusOK, gin.H{
		  "mint_lease_id": lease.LeaseID,
		  "lease":         lease,
	   })
   }

   func (h *handler) assertLease(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	  var body struct {
		   LeaseID string `json:"mint_lease_id"`
		   Kind string `json:"kind"`
		  Asset string `json:"asset"`
		  Actor string `json:"actor"`
		   UserID string `json:"user_id"`
		   Amount string `json:"amount"`
	  }
	  if !mw.BindJSON(c, &body) {
		   return
	  }
	  res, err := h.eng.AssertLease(c.Request.Context(), AssertInput{
		   LeaseID: body.LeaseID, Kind: body.Kind, Asset: body.Asset, Actor: body.Actor, UserID: body.UserID, Amount: body.Amount,
	  })
	  if err != nil {
		   c.JSON(http.StatusConflict, gin.H{
			  "allowed": false, "reason": reasonOf(res), "detail": detailOf(res),
			  "proof_id": proofOf(res), "mint_lease_id": body.LeaseID,
		  })
		   return
	  }
	   c.JSON(http.StatusOK, gin.H{"allowed": true, "mint_lease_id": body.LeaseID, "lease": res.Lease})
  }

   func (h *handler) mint(c *gin.Context) {
	   h.postKind(c, KindMint)
  }

  func (h *handler) redeem(c *gin.Context) {
	  h.postKind(c, KindRedeem)
  }

  func (h *handler) postKind(c *gin.Context, kind LeaseKind) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	   var body MintWithLeaseReq
	  if !mw.BindJSON(c, &body) {
		   return
	   }
	   if body.LeaseID == "" {
		   body.LeaseID = strings.TrimSpace(c.GetHeader("X-Mint-Lease-Id"))
	  }
	  body.Kind = string(kind)
	  var (
		  j     stablebr.Journal
		  lease *MintLease
		   err   error
	  )
	  if kind == KindRedeem {
		  j, lease, err = h.eng.RedeemWithLease(c.Request.Context(), body)
	   } else {
		   j, lease, err = h.eng.MintWithLease(c.Request.Context(), body)
	  }
	   if err != nil {
		  lid := body.LeaseID
		   if lease != nil {
			   lid = lease.LeaseID
		  }
		  c.JSON(http.StatusConflict, gin.H{
			   "error": err.Error(), "mint_lease_id": lid, "journal": j,
		   })
		   return
	   }
	   out := gin.H{"journal": j, "mint_lease_id": body.LeaseID}
	   if lease != nil {
		   out["mint_lease_id"] = lease.LeaseID
		   out["lease"] = lease
	   }
	  c.JSON(http.StatusOK, out)
  }

   func (h *handler) revoke(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	  var body struct {
		   LeaseID string `json:"mint_lease_id"`
		  Actor   string `json:"actor"`
		  Note string `json:"note"`
	   }
	   if !mw.BindJSON(c, &body) {
		  return
	   }
	   lease, err := h.eng.Revoke(c.Request.Context(), body.LeaseID, body.Actor, body.Note)
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, gin.H{"mint_lease_id": lease.LeaseID, "lease": lease})
   }

  func (h *handler) resolveHalf(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	   var body struct {
		   HalfID string `json:"half_id"`
		  Actor string `json:"actor"`
		  Note  string `json:"note"`
	   }
	   if !mw.BindJSON(c, &body) {
		   return
	   }
	   p, err := h.eng.ResolveHalf(c.Request.Context(), body.HalfID, body.Actor, body.Note)
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, p)
  }

  func (h *handler) getLease(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	   lease, err := h.eng.GetLease(c.Request.Context(), c.Param("lease_id"))
	   if err != nil {
		   mw.WriteProblem(c, err)
		   return
	   }
	  c.JSON(http.StatusOK, gin.H{"mint_lease_id": lease.LeaseID, "lease": lease})
  }

  func (h *handler) getProof(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	  p, err := h.eng.GetProof(c.Request.Context(), c.Param("proof_id"))
	  if err != nil {
		   mw.WriteProblem(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, p)
   }

   func (h *handler) listProofs(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	   limit, _ := strconv.Atoi(c.Query("limit"))
	   rows, err := h.eng.ListProofs(c.Request.Context(), c.Query("scope"), limit)
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, gin.H{"rows": rows})
   }

  func (h *handler) listHalf(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	   limit, _ := strconv.Atoi(c.Query("limit"))
	  rows, err := h.eng.ListHalf(c.Request.Context(), c.Query("mint_lease_id"), limit)
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, gin.H{"rows": rows})
  }

   func (h *handler) exportBundle(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	  b, err := h.eng.ExportProofBundle(c.Request.Context(), c.Param("lease_id"))
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, b)
   }

   func (h *handler) listAudit(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	   limit, _ := strconv.Atoi(c.Query("limit"))
	   rows, err := h.eng.ListAudit(c.Request.Context(), c.Query("mint_lease_id"), limit)
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, gin.H{"rows": rows})
   }

   func (h *handler) checklist(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	  rows, err := h.eng.Bridge().Checklist(c.Query("kind"), c.Query("asset"))
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, gin.H{"checks": rows})
   }

   func (h *handler) fingerprint(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	   fp, err := h.eng.Bridge().CurrentFingerprint(c.Query("kind"), c.Query("asset"))
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, FingerprintFrom(fp))
  }

  func reasonOf(r *AssertResult) string {
	   if r == nil {
		   return ""
	  }
	  return string(r.Reason)
   }

   func detailOf(r *AssertResult) string {
	   if r == nil {
		  return ""
	  }
	   return r.Detail
   }

   func proofOf(r *AssertResult) string {
	   if r == nil {
		  return ""
	  }
	  return r.ProofID
   }
