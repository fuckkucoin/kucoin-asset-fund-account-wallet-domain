package dualrelease

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/dualctl"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
)

// Handler HTTP 处理器；持有 Engine 单例与 dualctl.Book 只读引用。
type Handler struct {
	engine *Engine
	book   *dualctl.Book // 只读：读取 QuorumFacts / TicketSummary
}

// NewHandler 初始化 Handler。
  func NewHandler(engine *Engine, book *dualctl.Book) *Handler {
	   return &Handler{engine: engine,
		  book: book}
  }

// Register 将所有路由注册到 gin RouterGroup。

   func (h *Handler) Register(rg *gin.RouterGroup) {
	   rg.GET("/dualrelease/status/:request_id", h.getStatus)
	  rg.GET("/dualrelease/proof/:request_id/:gen", h.getProof)
	   rg.GET("/dualrelease/proofs/export/:request_id", h.exportProofs)
	   rg.POST("/dualrelease/open", h.openCase)
	  rg.POST("/dualrelease/acquire-lease", h.acquireLease)
	   rg.POST("/dualrelease/confirm-unlock", h.confirmUnlock)
	   rg.POST("/dualrelease/revert-unlock", h.revertUnlock)
	  rg.POST("/dualrelease/execute", h.execute)
	  rg.POST("/dualrelease/force-abort", h.forceAbort)
   }

// ──────────────────────────────────────────────────
// 读取接口
// ──────────────────────────────────────────────────
// getStatus GET /dualrelease/status/:request_id

   func (h *Handler) getStatus(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	  requestID := strings.TrimSpace(c.Param("request_id"))
	  if requestID == "" {
		   c.JSON(http.StatusBadRequest, gin.H{"error": "request_id required"})
		   return
	  }
	   rc,
		  err := h.engine.GetCase(requestID)
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	   resp := buildStatusResp(rc)
	  // 附加 dualctl 票证摘要（只读）。
	   if h.book != nil {
		  now := time.Now().UTC()
		   if facts, ferr := h.book.ReadQuorumFacts(rc.TicketID, now); ferr == nil {
			   resp["ticket_quorum"] = facts
		  }
	  }
	   c.JSON(http.StatusOK, resp)
  }

// getProof GET /dualrelease/proof/:request_id/:gen

   func (h *Handler) getProof(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	  requestID := strings.TrimSpace(c.Param("request_id"))
	  genStr := strings.TrimSpace(c.Param("gen"))
	  gen, err := strconv.ParseInt(genStr, 10, 64)
	  if err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": "gen must be integer"})
		  return
	  }
	  proof, err := h.engine.GetProof(requestID, gen)
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, proof)
  }

// exportProofs GET /dualrelease/proofs/export/:request_id
// 导出全部世代效果证明（供审计）；依赖 store 扩展接口 ProofLister。
  func (h *Handler) exportProofs(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	   requestID := strings.TrimSpace(c.Param("request_id"))
	  // 尝试走 ProofLister 扩展（sqlrepo.DualReleaseRepo 实现）。
	   if lister, ok := h.engine.store.(ProofLister); ok {
		   proofs, err := lister.ListProofs(requestID)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(http.StatusOK, gin.H{"request_id": requestID, "proofs": proofs, "count": len(proofs)})
		  return
	  }
	  // 回退：只查当前世代。
	   rc, err := h.engine.GetCase(requestID)
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	  proof, err := h.engine.GetProof(requestID, rc.Gen)
	   if err != nil {
		  c.JSON(http.StatusOK, gin.H{"request_id": requestID, "proofs": []any{}, "count": 0})
		  return
	  }
	  c.JSON(http.StatusOK, gin.H{"request_id": requestID, "proofs": []*EffectProof{proof}, "count": 1})
   }

// ProofLister 可选扩展接口；sqlrepo.DualReleaseRepo 实现。
type ProofLister interface {
	ListProofs(requestID string) ([]*EffectProof, error)
}

// ──────────────────────────────────────────────────
// 写入接口
// ──────────────────────────────────────────────────
// openCaseReq POST /dualrelease/open

type openCaseReq struct {
	TicketID  string `json:"ticket_id"`
	RequestID string `json:"request_id"`
	EpochAt int64 `json:"epoch_at"`
	TTL string `json:"ttl"` // RFC3339
	OpenedBy string `json:"opened_by"`
	Actor    string `json:"actor"`
}

   func (h *Handler) openCase(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodyApprove); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	   var body openCaseReq
	   if !mw.BindJSON(c, &body) {
		   return
	  }
	   ttl, err := time.Parse(time.RFC3339, body.TTL)
	   if err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("ttl parse error: %v", err)})
		  return
	   }
	  facts := TicketFacts{
		  TicketID: body.TicketID,
		  RequestID: body.RequestID,
		  EpochAt: body.EpochAt,
		   TTL:      ttl,
		   OpenedBy: body.OpenedBy,
	  }
	   rc,
		  err := h.engine.OpenCase(facts, body.Actor)
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, rc)
  }

// acquireLeaseReq POST /dualrelease/acquire-lease

type acquireLeaseReq struct {
	RequestID string `json:"request_id"`
	Holder    string `json:"holder"`
}

   func (h *Handler) acquireLease(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodyApprove); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	  var body acquireLeaseReq
	  if !mw.BindJSON(c, &body) {
		   return
	   }
	  lease, err := h.engine.AcquireLease(body.RequestID, body.Holder)
	  if err != nil {
		   mw.WriteProblem(c, err)
		   return
	   }
	   c.JSON(http.StatusOK, lease)
   }

// confirmUnlockReq POST /dualrelease/confirm-unlock

type confirmUnlockReq struct {
	RequestID string `json:"request_id"`
	Gen       int64  `json:"gen"`
	LeaseToken string `json:"lease_token"`
}

   func (h *Handler) confirmUnlock(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodyApprove); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	   var body confirmUnlockReq
	   if !mw.BindJSON(c, &body) {
		  return
	   }
	   rc, err := h.engine.ConfirmUnlock(body.RequestID, body.Gen, body.LeaseToken)
	   if err != nil {
		   mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, rc)
  }

// revertUnlockReq POST /dualrelease/revert-unlock

type revertUnlockReq struct {
	RequestID string `json:"request_id"`
	Gen       int64  `json:"gen"`
}

   func (h *Handler) revertUnlock(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodyApprove); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	  var body revertUnlockReq
	   if !mw.BindJSON(c, &body) {
		  return
	  }
	   rc,
		  err := h.engine.RevertUnlock(body.RequestID, body.Gen)
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, rc)
   }

// executeReq POST /dualrelease/execute

type executeReq struct {
	RequestID string `json:"request_id"`
	Actor      string `json:"actor"`
	Gen        int64  `json:"gen"`
	LeaseToken string `json:"lease_token"`
	TicketID string `json:"ticket_id"`
	EpochAt      int64  `json:"epoch_at"`
	TTL          string `json:"ttl"`
	OpenedBy     string `json:"opened_by"`
	FundActionID string `json:"fund_action_id"`
	Amount string `json:"amount"`
}

  func (h *Handler) execute(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodyApprove); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	  var body executeReq
	  if !mw.BindJSON(c, &body) {
		   return
	   }
	  ttl, err := time.Parse(time.RFC3339, body.TTL)
	  if err != nil {
		   c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("ttl parse error: %v", err)})
		  return
	   }
	   params := ExecuteParams{
		  RequestID: body.RequestID,
		  Actor:     body.Actor,
		  Gen:       body.Gen,
		  LeaseToken: body.LeaseToken,
		  TicketFacts: TicketFacts{
			  TicketID:  body.TicketID,
			  RequestID: body.RequestID,
			   EpochAt:   body.EpochAt,
			  TTL:       ttl,
			  OpenedBy: body.OpenedBy,
		  },
		   FundObs: FundObservation{
			   FundActionID: body.FundActionID,
			   Amount:     body.Amount,
			  ObservedAt: time.Now().UTC(),
		   },
	   }
	  proof, rc, err := h.engine.Execute(params)
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	   c.JSON(http.StatusOK, gin.H{"case": rc, "proof": proof})
  }

// forceAbortReq POST /dualrelease/force-abort

type forceAbortReq struct {
	RequestID string `json:"request_id"`
	Actor1    string `json:"actor1"`
	Actor2    string `json:"actor2"`
	Reason    string `json:"reason"`
}

  func (h *Handler) forceAbort(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	  var body forceAbortReq
	   if !mw.BindJSON(c, &body) {
		   return
	  }
	   rc, err := h.engine.ForceAbort(body.RequestID, body.Actor1, body.Actor2, body.Reason)
	   if err != nil {
		   mw.WriteProblem(c, err)
		   return
	   }
	  c.JSON(http.StatusOK, rc)
   }

// ──────────────────────────────────────────────────
// 响应构造辅助
// ──────────────────────────────────────────────────

  func buildStatusResp(rc *ReleaseCase) gin.H {
	   return gin.H{
		  "id":         rc.ID,
		   "request_id": rc.RequestID,
		   "ticket_id":  rc.TicketID,
		   "gen":   rc.Gen,
		  "state": rc.State,
		  "state_summary": StateSummary(rc.State),
		  "actor":          rc.Actor,
		  "fund_action_id": rc.FundActionID,
		   "effect_proof_id": rc.EffectProofID,
		  "abort_reason":    rc.AbortReason,
		  "fence_epoch": rc.Fence.Epoch,
		  "created_at":  rc.CreatedAt,
		   "updated_at":  rc.UpdatedAt,
	  }
   }
// temporary guard removed after the production check
