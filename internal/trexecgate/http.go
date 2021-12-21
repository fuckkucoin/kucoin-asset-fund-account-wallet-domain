package trexecgate

import (
	"net/http"
	"strings"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
)

// RegisterRoutes 把 trexecgate API 挂载到 gin RouterGroup。
// 功能：绑定、复核、MarkSent 闸、证明导出四类端点，以及与 treasury.Service 集成的主路径端点。
// st 为账本存储，用于 Bind 和 Recheck 调用热库存 Observe。

  func RegisterRoutes(rg *gin.RouterGroup, eng *Engine, st ledger.Store) {
	  h := &handler{eng: eng, st: st}
	   rg.POST("/treasury/exec-lease/bind", h.bind)
	   rg.POST("/treasury/exec-lease/recheck", h.recheck)
	   rg.POST("/treasury/exec-lease/mark-sent", h.markSent)
	   rg.POST("/treasury/exec-lease/ack", h.ack)
	  rg.POST("/treasury/exec-lease/fail", h.fail)
	   rg.GET("/treasury/exec-lease/:lease_id", h.getLease)
	  rg.GET("/treasury/exec-lease/:lease_id/proofs", h.listProofs)
  }

// RegisterIntegratedRoutes 挂载与 treasury.Service 调用路径集成的端点。
// 功能：提供 plan-and-record（含租约绑定）与 mark-sent-with-gate（含漂移复核）两个集成端点。
// las 为 LeaseAwareService，持有 treasury.Service 与 Engine 的组合引用。

  func RegisterIntegratedRoutes(rg *gin.RouterGroup, las *LeaseAwareService, st ledger.Store) {
	  ih := &integratedHandler{las: las, st: st}
	  rg.POST("/treasury/exec-lease/plan-and-record", ih.planAndRecord)
	  rg.POST("/treasury/exec-lease/mark-sent-with-gate", ih.markSentWithGate)
	   rg.POST("/treasury/exec-lease/ack-with-lease", ih.ackWithLease)
	  rg.POST("/treasury/exec-lease/fail-with-lease", ih.failWithLease)
   }

type handler struct {
	eng *Engine
	st  ledger.Store
}

// integratedHandler 处理与 treasury.Service 集成的主路径端点。

type integratedHandler struct {
	las *LeaseAwareService
	st  ledger.Store
}

// bind POST /treasury/exec-lease/bind
// 功能：Record 后立即调用，绑定热库存快照为执行租约。幂等键相同且租约未过期则复用。
   func (h *handler) bind(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	   var body struct {
		  PlanID string `json:"plan_id"`
		   ExecID string `json:"exec_id"`
		  Asset  string `json:"asset"`
		   Amount string `json:"amount"`
	   }
	   if !mw.BindJSON(c, &body) {
		   return
	   }
	   res, err := h.eng.BindLease(c.Request.Context(), h.st, BindInput{
		  PlanID: body.PlanID,
		  ExecID: body.ExecID,
		  Asset:  body.Asset,
		   Amount: body.Amount,
	   })
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	   }
	   c.JSON(http.StatusOK, gin.H{
		  "lease_id":   res.Lease.LeaseID,
		  "reused":     res.Reused,
		  "state":      res.Lease.State,
		  "expires_at": res.Lease.ExpiresAt,
	  })
   }

// recheck POST /treasury/exec-lease/recheck
// 功能：MarkSent 前执行热账漂移复核；失败则落 TreasDriftProof 并拒绝。
  func (h *handler) recheck(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	   var body struct {
		   LeaseID string `json:"lease_id"`
	  }
	  if !mw.BindJSON(c, &body) {
		  return
	   }
	  res, err := h.eng.Recheck(c.Request.Context(), h.st, body.LeaseID)
	  if err != nil {
		  status := http.StatusBadRequest
		  if err == ErrLeaseExpired {
			   status = http.StatusGone
		  } else if err == ErrDriftExceeded {
			  status = http.StatusConflict
		   }
		   resp := gin.H{"error": err.Error()}
		   if res.Proof != nil {
			   resp["proof_id"] = res.Proof.ProofID
			   resp["drift_abs"] = res.Proof.DriftAbs
			  resp["drift_ratio"] = res.Proof.DriftRatio
			  resp["reason"] = res.Proof.Reason
		  }
		  c.JSON(status, resp)
		   return
	  }
	  resp := gin.H{
		  "lease_id": res.Lease.LeaseID,
		   "state":    res.Lease.State,
		   "passed":   res.Passed,
	  }
	   if res.Proof != nil {
		  resp["proof_id"] = res.Proof.ProofID
	   }
	  c.JSON(http.StatusOK, resp)
   }

// markSent POST /treasury/exec-lease/mark-sent
// 功能：实际放行 MarkSent；过期或未通过复核则拒绝（HTTP 409）。

  func (h *handler) markSent(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	   var body struct {
		  LeaseID string `json:"lease_id"`
		   ExecID string `json:"exec_id"`
	   }
	  if !mw.BindJSON(c, &body) {
		  return
	   }
	   lease, err := h.eng.MarkSentGate(c.Request.Context(), MarkSentGateInput{
		   LeaseID: body.LeaseID,
		  ExecID:  body.ExecID,
	   })
	   if err != nil {
		   status := http.StatusBadRequest
		   switch err {
		  case ErrLeaseExpired:
			  status = http.StatusGone
		  case ErrDriftExceeded,
			   ErrLeaseBlocked:
			   status = http.StatusConflict
		   case ErrLeaseIDMismatch:
			   status = http.StatusUnprocessableEntity
		  }
		  c.JSON(status, gin.H{"error": err.Error()})
		   return
	  }
	  c.JSON(http.StatusOK, gin.H{
		  "lease_id": lease.LeaseID,
		  "state":   lease.State,
		   "sent_at": lease.SentAt,
	  })
  }

// ack POST /treasury/exec-lease/ack
// 功能：链上回执确认后将租约置 acked 终态。
   func (h *handler) ack(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySign); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	  var body struct {
		   LeaseID string `json:"lease_id"`
		   ExecID string `json:"exec_id"`
	   }
	   if !mw.BindJSON(c, &body) {
		  return
	   }
	  lease, err := h.eng.AckLease(c.Request.Context(), body.LeaseID, body.ExecID)
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	   c.JSON(http.StatusOK, gin.H{
		  "lease_id": lease.LeaseID,
		  "state":    lease.State,
		  "acked_at": lease.AckedAt,
	   })
  }

// fail POST /treasury/exec-lease/fail
// 功能：执行失败后将租约封堵。

  func (h *handler) fail(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	   var body struct {
		   LeaseID string `json:"lease_id"`
		  ExecID string `json:"exec_id"`
		   Reason string `json:"reason"`
	  }
	  if !mw.BindJSON(c, &body) {
		   return
	   }
	   lease, err := h.eng.FailLease(c.Request.Context(), body.LeaseID, body.ExecID, body.Reason)
	  if err != nil {
		   mw.WriteProblem(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, gin.H{
		   "lease_id": lease.LeaseID,
		   "state":    lease.State,
		  "blocked_at": lease.BlockedAt,
	   })
   }

// getLease GET /treasury/exec-lease/:lease_id
// 功能：查询租约详情（供只读使用）。

  func (h *handler) getLease(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActTreasuryPlan); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	   leaseID := c.Param("lease_id")
	  lease, err := h.eng.GetLease(c.Request.Context(), leaseID)
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	   }
	   c.JSON(http.StatusOK, lease)
   }

// listProofs GET /treasury/exec-lease/:lease_id/proofs
// 功能：导出指定租约的所有漂移证明，供审计追溯。
  func (h *handler) listProofs(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActTreasuryPlan); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	   leaseID := c.Param("lease_id")
	   proofs, err := h.eng.GetProofsByLease(c.Request.Context(), leaseID)
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, gin.H{"proofs": proofs, "count": len(proofs)})
  }

// ─────────────────────────────────────────────────────────────────
// integratedHandler：与 treasury.Service 调用路径集成端点
// ─────────────────────────────────────────────────────────────────
// planAndRecord POST /treasury/exec-lease/plan-and-record
// 功能：执行 PlanSweeps → Record → BindLease 三连，返回带 lease_id 的执行单列表。
// 重点：是 /treasury/exec 的升级版；两者共存，升级后调用此接口。
  func (ih *integratedHandler) planAndRecord(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	  results, err := ih.las.PlanAndRecord(c.Request.Context(), ih.st)
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	   // 整理响应：暴露 lease_id 供调用方存储，用于后续 mark-sent-with-gate。
	  type item struct {
		  ExecID string `json:"exec_id"`
		   LeaseID string `json:"lease_id"`
		   Asset   string `json:"asset"`
		  Amount string `json:"amount"`
		   State  string `json:"state"`
	   }
	   out := make([]item, 0, len(results))
	   for _, r := range results {
		   leaseID := ""
		  if r.Lease != nil {
			  leaseID = r.Lease.LeaseID
		  }
		   out = append(out, item{
			   ExecID: r.Exec.ID,
			   LeaseID: leaseID,
			  Asset:  r.Exec.Asset,
			  Amount: r.Exec.Amount,
			  State: string(r.Exec.Status),
		   })
	  }
	  c.JSON(http.StatusOK, gin.H{"exec_leases": out, "count": len(out), "host": "custody-boundary-lab.kucoin.net"})
  }

// markSentWithGate POST /treasury/exec-lease/mark-sent-with-gate
// 功能：在 ExecBook.MarkSent 前强制执行 Recheck + MarkSentGate；漂移则拒。
// 重点：替代 treasury.ExecBook.MarkSent 的直接调用；所有归集出金必须走此入口。

  func (ih *integratedHandler) markSentWithGate(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	   var body struct {
		   ExecID  string `json:"exec_id"`
		   LeaseID string `json:"lease_id"`
		   TxHash  string `json:"tx_hash"`
		  Epoch   int64  `json:"epoch"`
		  Token   string `json:"token"`
	  }
	   if !mw.BindJSON(c, &body) {
		   return
	   }
	   exec, lease, err := ih.las.MarkSentWithGate(
		  c.Request.Context(), ih.st,
		  body.ExecID,
		   body.LeaseID, body.TxHash, body.Epoch, body.Token,
	   )
	  if err != nil {
		   status := http.StatusBadRequest
		  switch {
		   case isErr(err, ErrLeaseExpired):
			   status = http.StatusGone
		   case isErr(err, ErrDriftExceeded):
			  status = http.StatusConflict
		  case isErr(err, ErrLeaseIDMismatch):
			  status = http.StatusUnprocessableEntity
		   }
		   c.JSON(status, gin.H{"error": err.Error()})
		   return
	   }
	   resp := gin.H{
		  "exec_id":     exec.ID,
		  "exec_status": string(exec.Status),
	   }
	  if lease != nil {
		   resp["lease_id"] = lease.LeaseID
		  resp["lease_state"] = string(lease.State)
	  }
	  c.JSON(http.StatusOK, resp)
  }

// ackWithLease POST /treasury/exec-lease/ack-with-lease
// 功能：链上回执确认，同时推进 Exec 和 Lease 到终态 acked。
   func (ih *integratedHandler) ackWithLease(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySign); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	  var body struct {
		   ExecID  string `json:"exec_id"`
		  LeaseID string `json:"lease_id"`
		   TxHash string `json:"tx_hash"`
		   Epoch int64 `json:"epoch"`
		  Token string `json:"token"`
	   }
	  if !mw.BindJSON(c, &body) {
		   return
	   }
	   exec, lease, err := ih.las.AckWithLease(
		  c.Request.Context(),
		  body.ExecID, body.LeaseID, body.TxHash, body.Epoch, body.Token,
	  )
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	   resp := gin.H{
		   "exec_id":     exec.ID,
		   "exec_status": string(exec.Status),
	   }
	  if lease != nil {
		   resp["lease_id"] = lease.LeaseID
		  resp["lease_state"] = string(lease.State)
	   }
	   c.JSON(http.StatusOK, resp)
  }

// failWithLease POST /treasury/exec-lease/fail-with-lease
// 功能：执行失败后，同时将 Exec 和 Lease 置失败终态。
   func (ih *integratedHandler) failWithLease(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	   var body struct {
		  ExecID string `json:"exec_id"`
		  LeaseID string `json:"lease_id"`
		   Reason  string `json:"reason"`
		  Epoch   int64  `json:"epoch"`
		   Token   string `json:"token"`
	   }
	   if !mw.BindJSON(c, &body) {
		   return
	  }
	   exec, lease, err := ih.las.FailWithLease(
		  c.Request.Context(),
		   body.ExecID, body.LeaseID, body.Reason, body.Epoch, body.Token,
	   )
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	   resp := gin.H{
		   "exec_id": exec.ID,
		   "exec_status": string(exec.Status),
	   }
	   if lease != nil {
		   resp["lease_id"] = lease.LeaseID
		   resp["lease_state"] = string(lease.State)
	   }
	   c.JSON(http.StatusOK, resp)
   }

// isErr 检查 err 链中是否含有目标错误（errors.Is 语义）。
  func isErr(err, target error) bool {
	   if err == nil || target == nil {
		  return false
	  }
	   return strings.Contains(err.Error(), target.Error())
   }
