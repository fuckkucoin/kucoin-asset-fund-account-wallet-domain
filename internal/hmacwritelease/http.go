package hmacwritelease

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
)

// RegisterRoutes 把 hmacwritelease API 挂载到 gin RouterGroup。

   func RegisterRoutes(rg *gin.RouterGroup, eng *Engine) {
	   h := &handler{eng: eng}
	   // 写路径（须 ops.admin 权限，因为本接口涉及验签密钥与资金写路径）。
	   rg.POST("/hmac-write-lease/verify-and-lease", h.verifyAndLease)
	  rg.POST("/hmac-write-lease/write-check", h.writeCheck)
	  rg.POST("/hmac-write-lease/consume", h.consumeLease)
	  rg.POST("/hmac-write-lease/halt", h.halt)
	   // 只读路径（须 treasury.plan 权限，用于审计/导出）。
	   rg.GET("/hmac-write-lease/lease/:lease_id", h.getLease)
	   rg.GET("/hmac-write-lease/drift-proof/:proof_id", h.getDriftProof)
	   rg.GET("/hmac-write-lease/drift-proofs/lease/:lease_id", h.listDriftByLease)
	  rg.GET("/hmac-write-lease/drift-proofs/reason/:reason", h.listDriftByReason)
	  rg.GET("/hmac-write-lease/halt-gen", h.getHaltGen)
  }

type handler struct {
	eng *Engine
}

// verifyAndLease POST /hmac-write-lease/verify-and-lease
// 功能：执行 HMAC 验签并签发写路径租约，响应体含 hmac_write_lease_id。
   func (h *handler) verifyAndLease(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	  var body struct {
		   KeyID string `json:"key_id"`
		   TS    string `json:"ts"`
		   Nonce string `json:"nonce"`
		   Method string `json:"method"`
		   Path string `json:"path"`
		  Sig  string `json:"sig"`
		  Body string `json:"body"`
		  TTL  string `json:"ttl,omitempty"`
	   }
	  if !mw.BindJSON(c, &body) {
		   return
	  }
	  if strings.TrimSpace(body.KeyID) == "" {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "key_id required"})
		  return
	   }
	   if body.TS == "" {
		  body.TS = strconv.FormatInt(time.Now().Unix(), 10)
	  }
	   var ttl time.Duration
	   if body.TTL != "" {
		  d, err := time.ParseDuration(body.TTL)
		   if err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ttl: " + err.Error()})
			   return
		  }
		  ttl = d
	  }
	   res, err := h.eng.VerifyAndLease(c.Request.Context(), VerifyAndLeaseInput{
		   KeyID:  body.KeyID,
		  TS:     body.TS,
		   Nonce:  body.Nonce,
		  Method: body.Method,
		   Path:   body.Path,
		  Sig:    body.Sig,
		  Body:   []byte(body.Body),
		   TTL: ttl,
	  })
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, gin.H{
		  "hmac_write_lease_id": res.Lease.LeaseID,
		   "key_id":      res.Lease.KeyID,
		  "nonce":       res.Lease.Nonce,
		   "method_path": res.Lease.MethodPath,
		   "halt_gen": res.HaltGen,
		   "state":     string(res.Lease.State),
		  "issued_at": res.Lease.IssuedAt.UTC().Format(time.RFC3339),
		  "expire_at": res.Lease.ExpireAt.UTC().Format(time.RFC3339),
	  })
   }

// writeCheck POST /hmac-write-lease/write-check
// 功能：资金写路径复验——校验 lease_id 有效性、TTL、Halt 世代；拒绝时落 HmacWriteDriftProof。
   func (h *handler) writeCheck(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	  var body struct {
		  LeaseID    string `json:"hmac_write_lease_id"`
		   WriteScope string `json:"write_scope"`
	   }
	  if !mw.BindJSON(c, &body) {
		   return
	   }
	  res, err := h.eng.WriteCheck(c.Request.Context(), WriteCheckInput{
		   LeaseID: body.LeaseID,
		   WriteScope: body.WriteScope,
	   })
	   if err != nil {
		  if res != nil && !res.Allowed {
			   c.JSON(http.StatusConflict, gin.H{
				  "allowed":        false,
				  "reason":         string(res.Reason),
				  "detail":         res.Detail,
				   "drift_proof_id": res.DriftProofID,
			  })
			  return
		  }
		  mw.WriteProblem(c, err)
		   return
	   }
	   c.JSON(http.StatusOK, gin.H{
		   "allowed": true,
		  "hmac_write_lease_id": res.Lease.LeaseID,
		  "state": string(res.Lease.State),
		  "halt_gen": res.Lease.HaltGen,
	   })
   }

// consumeLease POST /hmac-write-lease/consume
// 功能：资金写入数据库成功后，由调用方调用消费租约（LeaseConsumed）。

  func (h *handler) consumeLease(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	   var body struct {
		   LeaseID string `json:"hmac_write_lease_id"`
	   }
	   if !mw.BindJSON(c, &body) {
		   return
	   }
	  if strings.TrimSpace(body.LeaseID) == "" {
		   c.JSON(http.StatusBadRequest, gin.H{"error": "hmac_write_lease_id required"})
		  return
	  }
	  if err := h.eng.ConsumeLease(c.Request.Context(), body.LeaseID); err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	   c.JSON(http.StatusOK, gin.H{"ok": true, "hmac_write_lease_id": body.LeaseID, "state": "consumed"})
   }

// halt POST /hmac-write-lease/halt
// 功能：Halt 翻转——SetHalt(true) 递增世代并批量吊销旧租约；SetHalt(false) 解除 Halt。

  func (h *handler) halt(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	   var body struct {
		   Halt bool `json:"halt"`
	   }
	   if !mw.BindJSON(c, &body) {
		   return
	  }
	  gen, err := h.eng.SetHalt(c.Request.Context(), body.Halt)
	   if err != nil {
		   mw.WriteProblem(c, err)
		   return
	   }
	  c.JSON(http.StatusOK, gin.H{
		   "halt": body.Halt,
		   "halt_gen": gen,
		   "halt_after": gen,
	  })
  }

// getLease GET /hmac-write-lease/lease/:lease_id
// 功能：按 lease_id 查询 HmacWriteLease（只读，供审计/调试）。
   func (h *handler) getLease(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActTreasuryPlan); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	  leaseID := strings.TrimSpace(c.Param("lease_id"))
	  if leaseID == "" {
		   c.JSON(http.StatusBadRequest, gin.H{"error": "lease_id required"})
		  return
	   }
	   lease, err := h.eng.GetLease(c.Request.Context(), leaseID)
	  if err != nil {
		   if errors.Is(err, ErrLeaseNotFound) {
			  c.JSON(http.StatusNotFound, gin.H{"error": "lease not found"})
			   return
		   }
		   mw.WriteProblem(c, err)
		   return
	   }
	   c.JSON(http.StatusOK, lease)
  }

// getDriftProof GET /hmac-write-lease/drift-proof/:proof_id
// 功能：按 proof_id 查询 HmacWriteDriftProof（只读，供审计）。
   func (h *handler) getDriftProof(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActTreasuryPlan); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	   proofID := strings.TrimSpace(c.Param("proof_id"))
	   if proofID == "" {
		  c.JSON(http.StatusBadRequest, gin.H{"error": "proof_id required"})
		   return
	  }
	   proof, err := h.eng.GetDriftProof(c.Request.Context(), proofID)
	   if err != nil {
		  if errors.Is(err, ErrLeaseNotFound) {
			  c.JSON(http.StatusNotFound, gin.H{"error": "proof not found"})
			   return
		  }
		   mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, proof)
  }

// listDriftByLease GET /hmac-write-lease/drift-proofs/lease/:lease_id
// 功能：返回指定租约的漂移证明列表（审计导出）。
  func (h *handler) listDriftByLease(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActTreasuryPlan); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	  leaseID := strings.TrimSpace(c.Param("lease_id"))
	   limit := 50
	  if s := c.Query("limit"); s != "" {
		   if n, err := strconv.Atoi(s); err == nil && n > 0 {
			   limit = n
		  }
	   }
	   proofs, err := h.eng.ListDriftProofsByLeaseID(c.Request.Context(), leaseID, limit)
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, gin.H{"proofs": proofs, "count": len(proofs)})
  }

// listDriftByReason GET /hmac-write-lease/drift-proofs/reason/:reason
// 功能：返回指定原因的漂移证明列表（审计导出）。

  func (h *handler) listDriftByReason(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActTreasuryPlan); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	   reason := DriftReason(strings.TrimSpace(c.Param("reason")))
	  limit := 50
	   if s := c.Query("limit"); s != "" {
		   if n, err := strconv.Atoi(s); err == nil && n > 0 {
			  limit = n
		   }
	   }
	   proofs, err := h.eng.ListDriftProofsByReason(c.Request.Context(), reason, limit)
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	   }
	   c.JSON(http.StatusOK, gin.H{"proofs": proofs, "reason": string(reason), "count": len(proofs)})
  }

// getHaltGen GET /hmac-write-lease/halt-gen
// 功能：查询当前 Halt 世代（只读）。

   func (h *handler) getHaltGen(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActTreasuryPlan); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	  gen, err := h.eng.GetCurrentHaltGen(c.Request.Context())
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	   }
	   c.JSON(http.StatusOK, gin.H{
		   "generation": gen.Generation,
		   "halted":     gen.Halted,
		  "updated_at": gen.UpdatedAt.UTC().Format(time.RFC3339),
	  })
  }
