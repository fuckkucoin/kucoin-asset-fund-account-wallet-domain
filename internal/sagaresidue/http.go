package sagaresidue

import (
	"net/http"
	"strconv"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// Handlers HTTP 处理器，持有 Service 引用。
type Handlers struct {
	svc *Service
}

// NewHandlers 构造 HTTP 处理器。
   func NewHandlers(svc *Service) *Handlers {
	   return &Handlers{svc: svc}
  }

// RegisterRoutes 注册所有路由到指定 RouterGroup。

  func RegisterRoutes(v1 *gin.RouterGroup, svc *Service) {
	   h := NewHandlers(svc)
	   v1.GET("/sagaresidue/runs", h.ListRuns)
	   v1.GET("/sagaresidue/runs/:run_id", h.GetRun)
	   v1.POST("/sagaresidue/detect", h.Detect)
	   v1.POST("/sagaresidue/runs/:run_id/confirm-items", h.ConfirmItems)
	   v1.POST("/sagaresidue/runs/:run_id/closeout", h.InitiateCloseout)
	  v1.POST("/sagaresidue/plans/:plan_id/skip-leg", h.SkipLeg)
	  v1.GET("/sagaresidue/plans/:plan_id", h.GetPlan)
	  v1.GET("/sagaresidue/plans/:plan_id/legs", h.ListLegs)
	  v1.GET("/sagaresidue/proofs/:proof_id", h.GetProof)
	   v1.POST("/sagaresidue/proofs/verify", h.VerifyProof)
	  v1.GET("/sagaresidue/mutex/:saga_run_id", h.GetMutexStatus)
  }

// ── GET /sagaresidue/runs ─────────────────────────────────────────────────────
// ListRuns 列出 ResidueRun，支持按 state/severity/user_id 筛选和游标翻页。
   func (h *Handlers) ListRuns(c *gin.Context) {
	   start := time.Now()
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	  limit, _ := strconv.Atoi(c.Query("limit"))
	   req := RunListReq{
		  SagaRunID: strings.TrimSpace(c.Query("saga_run_id")),
		   State:    RunState(strings.TrimSpace(c.Query("state"))),
		  Severity: Severity(strings.TrimSpace(c.Query("severity"))),
		  UserID: strings.TrimSpace(c.Query("user_id")),
		   Limit: limit,
		  After: strings.TrimSpace(c.Query("after")),
	  }
	   runs, err := h.svc.ListRuns(c.Request.Context(), req)
	  recordAPIMetrics("/sagaresidue/runs", httpStatus(err), start)
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	   c.JSON(http.StatusOK, gin.H{
		  "runs": runs,
		   "count": len(runs),
		  "host":  "ledger-transfer-freeze-reconciliation.kucoin.net",
	  })
   }

// ── GET /sagaresidue/runs/:run_id ─────────────────────────────────────────────
// GetRun 查询指定 ResidueRun。

  func (h *Handlers) GetRun(c *gin.Context) {
	  start := time.Now()
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	  runID := strings.TrimSpace(c.Param("run_id"))
	  run, err := h.svc.GetRun(c.Request.Context(), runID)
	   recordAPIMetrics("/sagaresidue/runs/:run_id", httpStatus(err), start)
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, run)
  }

// ── POST /sagaresidue/detect ──────────────────────────────────────────────────
// Detect 触发残留扫描。

  func (h *Handlers) Detect(c *gin.Context) {
	  start := time.Now()
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	   var body struct {
		  SagaRunID string `json:"saga_run_id"`
		  ForceRescan bool `json:"force_rescan"`
	   }
	  if !mw.BindJSON(c, &body) {
		  return
	   }
	  actor := perm.FromRequest(c.Request, "").ID
	   req := DetectReq{
		   SagaRunID:   strings.TrimSpace(body.SagaRunID),
		   ForceRescan: body.ForceRescan,
		  ScanActor: actor,
	  }
	  report, rrun, err := h.svc.DetectResidue(c.Request.Context(), req)
	   recordAPIMetrics("/sagaresidue/detect", httpStatus(err), start)
	   if err == ErrNoResidues {
		  c.JSON(http.StatusOK, gin.H{"message": "no residues detected", "saga_run_id": body.SagaRunID})
		   return
	  }
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, gin.H{"report": report, "residue_run": rrun})
   }

// ── POST /sagaresidue/runs/:run_id/confirm-items ──────────────────────────────
// ConfirmItems 双人授权确认残留科目。
   func (h *Handlers) ConfirmItems(c *gin.Context) {
	   start := time.Now()
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	  runID := strings.TrimSpace(c.Param("run_id"))
	   var body struct {
		   ItemIDs     []string `json:"item_ids"`
		  Authorizer1 string   `json:"authorizer1"`
		  Authorizer2 string `json:"authorizer2"`
	   }
	   if !mw.BindJSON(c, &body) {
		   return
	   }
	  req := ConfirmResidueReq{
		   ResidueRunID: runID,
		  ItemIDs:     body.ItemIDs,
		   Authorizer1: strings.TrimSpace(body.Authorizer1),
		   Authorizer2: strings.TrimSpace(body.Authorizer2),
	  }
	   err := h.svc.ConfirmItems(c.Request.Context(), req)
	   recordAPIMetrics("/sagaresidue/runs/:run_id/confirm-items", httpStatus(err), start)
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, gin.H{"ok": true, "confirmed": len(body.ItemIDs)})
   }

// ── POST /sagaresidue/runs/:run_id/closeout ───────────────────────────────────
// InitiateCloseout 双人授权启动强制收口。

   func (h *Handlers) InitiateCloseout(c *gin.Context) {
	  start := time.Now()
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	  runID := strings.TrimSpace(c.Param("run_id"))
	   var body struct {
		  Authorizer1 string `json:"authorizer1"`
		   Authorizer2 string `json:"authorizer2"`
		  IdemPrefix string `json:"idem_prefix"`
	   }
	   if !mw.BindJSON(c, &body) {
		  return
	   }
	  req := CloseoutReq{
		  ResidueRunID: runID,
		   Authorizer1: strings.TrimSpace(body.Authorizer1),
		  Authorizer2: strings.TrimSpace(body.Authorizer2),
		   IdemPrefix:  strings.TrimSpace(body.IdemPrefix),
	  }
	  plan, err := h.svc.InitiateCloseout(c.Request.Context(), req)
	  recordAPIMetrics("/sagaresidue/runs/:run_id/closeout", httpStatus(err), start)
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, gin.H{"plan": plan, "message": "closeout initiated"})
  }

// ── POST /sagaresidue/plans/:plan_id/skip-leg ─────────────────────────────────
// SkipLeg 双人授权跳过一条 pending 腿。

  func (h *Handlers) SkipLeg(c *gin.Context) {
	  start := time.Now()
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	  planID := strings.TrimSpace(c.Param("plan_id"))
	   var body struct {
		   LegID string `json:"leg_id"`
		  SkipReason  string `json:"skip_reason"`
		  Authorizer1 string `json:"authorizer1"`
		  Authorizer2 string `json:"authorizer2"`
	   }
	  if !mw.BindJSON(c, &body) {
		  return
	   }
	   req := SkipLegReq{
		   PlanID: planID,
		   LegID: strings.TrimSpace(body.LegID),
		  SkipReason:  strings.TrimSpace(body.SkipReason),
		   Authorizer1: strings.TrimSpace(body.Authorizer1),
		   Authorizer2: strings.TrimSpace(body.Authorizer2),
	  }
	  err := h.svc.SkipLeg(c.Request.Context(), req)
	   recordAPIMetrics("/sagaresidue/plans/:plan_id/skip-leg", httpStatus(err), start)
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, gin.H{"ok": true, "leg_id": body.LegID, "skipped": true})
   }

// ── GET /sagaresidue/plans/:plan_id ───────────────────────────────────────────
// GetPlan 查询 CloseoutPlan。

  func (h *Handlers) GetPlan(c *gin.Context) {
	  start := time.Now()
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	   planID := strings.TrimSpace(c.Param("plan_id"))
	  plan, err := h.svc.GetPlan(c.Request.Context(), planID)
	   recordAPIMetrics("/sagaresidue/plans/:plan_id", httpStatus(err), start)
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, plan)
   }

// ── GET /sagaresidue/plans/:plan_id/legs ──────────────────────────────────────
// ListLegs 列举收口腿。
  func (h *Handlers) ListLegs(c *gin.Context) {
	   start := time.Now()
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	   planID := strings.TrimSpace(c.Param("plan_id"))
	   limit, _ := strconv.Atoi(c.Query("limit"))
	  req := LegListReq{
		  PlanID: planID,
		   State: LegState(strings.TrimSpace(c.Query("state"))),
		   Limit: limit,
	  }
	  legs, err := h.svc.ListLegs(c.Request.Context(), req)
	   recordAPIMetrics("/sagaresidue/plans/:plan_id/legs", httpStatus(err), start)
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	  c.JSON(http.StatusOK, gin.H{"legs": legs, "count": len(legs)})
   }

// ── GET /sagaresidue/proofs/:proof_id ─────────────────────────────────────────
// GetProof 读取 CloseProof。

  func (h *Handlers) GetProof(c *gin.Context) {
	  start := time.Now()
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	  proofID := strings.TrimSpace(c.Param("proof_id"))
	  proof, err := h.svc.GetProof(c.Request.Context(), proofID)
	   recordAPIMetrics("/sagaresidue/proofs/:proof_id", httpStatus(err), start)
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, proof)
  }

// ── POST /sagaresidue/proofs/verify ───────────────────────────────────────────
// VerifyProof 重校验 CloseProof。
  func (h *Handlers) VerifyProof(c *gin.Context) {
	  start := time.Now()
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	   var body ProofVerifyReq
	  if !mw.BindJSON(c, &body) {
		  return
	   }
	   ResidueProofVerifyTotal.Inc()
	  proof, err := h.svc.VerifyProof(c.Request.Context(), body)
	  recordAPIMetrics("/sagaresidue/proofs/verify", httpStatus(err), start)
	  if err == ErrProofInvalid {
		   ResidueProofVerifyInvalid.Inc()
		  c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error(), "proof": proof})
		   return
	   }
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	   }
	   c.JSON(http.StatusOK, gin.H{"proof": proof, "valid": true})
  }

// ── GET /sagaresidue/mutex/:saga_run_id ───────────────────────────────────────
// GetMutexStatus 查询互斥锁状态。

  func (h *Handlers) GetMutexStatus(c *gin.Context) {
	  start := time.Now()
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	  sagaRunID := strings.TrimSpace(c.Param("saga_run_id"))
	   locked, mg, err := h.svc.GetMutexStatus(c.Request.Context(), sagaRunID)
	   recordAPIMetrics("/sagaresidue/mutex/:saga_run_id", httpStatus(err), start)
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	   c.JSON(http.StatusOK, gin.H{
		   "saga_run_id": sagaRunID,
		   "locked": locked,
		  "mutex": mg,
	   })
   }

// ── 辅助函数 ─────────────────────────────────────────────────────────────────
// httpStatus 将业务错误映射为 HTTP 状态码。

  func httpStatus(err error) int {
	  if err == nil {
		  return http.StatusOK
	   }
	   switch err {
	  case ErrResidueRunNotFound, ErrItemNotFound, ErrLegNotFound, ErrProofNotFound, ErrSagaRunNotFound:
		  return http.StatusNotFound
	  case ErrDualAuth, ErrIdemEmpty, ErrAmountInvalid:
		   return http.StatusBadRequest
	  case ErrAlreadyClosed, ErrLegNotPending, ErrPlanConflict, ErrStaleGen, ErrGenLocked,
		   ErrCompensateBlock, ErrBadRunState:
		   return http.StatusConflict
	   case ErrProofMissing, ErrProofInvalid:
		   return http.StatusUnprocessableEntity
	  case ErrUnavailable, ErrStorageNil:
		  return http.StatusServiceUnavailable
	  case ErrNoResidues:
		   return http.StatusOK
	  }
	   return http.StatusInternalServerError
  }
