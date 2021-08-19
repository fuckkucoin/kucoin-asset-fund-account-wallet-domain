package ledorphan

import (
	"net/http"
	"strconv"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// Handlers 持有 Service 引用，提供 gin handler 函数。

type Handlers struct {
	svc *Service
}

// NewHandlers 构造 HTTP 处理器。

  func NewHandlers(svc *Service) *Handlers {
	  return &Handlers{svc: svc}
   }

// ── GET /ledorphan/reports ────────────────────────────────────────────────────

   func (h *Handlers) ListReports(c *gin.Context) {
	  start := time.Now()
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	   limit, _ := strconv.Atoi(c.Query("limit"))
	   req := ReportListReq{
		  BatchID: strings.TrimSpace(c.Query("batch_id")),
		  Status:  strings.TrimSpace(c.Query("status")),
		  Limit: limit,
		   After: strings.TrimSpace(c.Query("after")),
	  }
	  reports, err := h.svc.ListReports(c.Request.Context(), req)
	  recordAPI(c.FullPath(),
		   http.StatusOK, start)
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	   }
	   c.JSON(http.StatusOK, gin.H{"reports": reports, "count": len(reports)})
  }

// ── GET /ledorphan/reports/:report_id ────────────────────────────────────────

  func (h *Handlers) GetReport(c *gin.Context) {
	  start := time.Now()
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	   reportID := strings.TrimSpace(c.Param("report_id"))
	  report, err := h.svc.GetReport(c.Request.Context(), reportID)
	  recordAPI(c.FullPath(), httpStatus(err), start)
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	   c.JSON(http.StatusOK, report)
   }

// ── POST /ledorphan/scan ──────────────────────────────────────────────────────

   func (h *Handlers) TriggerScan(c *gin.Context) {
	   start := time.Now()
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	  var req ScanReq
	   if !mw.BindJSON(c, &req) {
		   return
	  }
	   resp,
		   err := h.svc.Scan(c.Request.Context(), req)
	  recordAPI(c.FullPath(), httpStatus(err), start)
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, resp)
   }

// ── POST /ledorphan/seal ──────────────────────────────────────────────────────

  func (h *Handlers) SealOrphans(c *gin.Context) {
	  start := time.Now()
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	  var req SealReq
	   if !mw.BindJSON(c, &req) {
		   return
	  }
	   action, err := h.svc.Seal(c.Request.Context(), req)
	  recordAPI(c.FullPath(), httpStatus(err), start)
	   if err != nil {
		  if err == ErrDualAuth {
			  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			  return
		  }
		  mw.WriteProblem(c, err)
		   return
	   }
	  // 封堵成功后更新 Prometheus 指标。
	   SealAppliedTotal.WithLabelValues(string(req.Kind)).Inc()
	   SealOrphanCount.Add(float64(len(action.OrphanIDs)))
	  c.JSON(http.StatusOK, action)
   }

// ── GET /ledorphan/seal/:seal_id ──────────────────────────────────────────────

  func (h *Handlers) GetSeal(c *gin.Context) {
	  start := time.Now()
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	   sealID := strings.TrimSpace(c.Param("seal_id"))
	   action, err := h.svc.GetSeal(c.Request.Context(), sealID)
	   recordAPI(c.FullPath(), httpStatus(err), start)
	   if err != nil {
		   mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, action)
  }

// ── GET /ledorphan/proof/:batch_id ────────────────────────────────────────────

   func (h *Handlers) ExportProof(c *gin.Context) {
	   start := time.Now()
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	  batchID := strings.TrimSpace(c.Param("batch_id"))
	   proof, err := h.svc.ExportProof(c.Request.Context(), batchID)
	   recordAPI(c.FullPath(),
		  httpStatus(err), start)
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, proof)
  }

// ── GET /ledorphan/proof/id/:proof_id ────────────────────────────────────────

   func (h *Handlers) GetProof(c *gin.Context) {
	  start := time.Now()
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	  proofID := strings.TrimSpace(c.Param("proof_id"))
	   proof,
		  err := h.svc.GetProof(c.Request.Context(), proofID)
	  recordAPI(c.FullPath(), httpStatus(err), start)
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	   c.JSON(http.StatusOK, proof)
  }

// ── POST /ledorphan/proof/verify ─────────────────────────────────────────────

  func (h *Handlers) VerifyProof(c *gin.Context) {
	   start := time.Now()
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	   var body struct {
		  ProofID string `json:"proof_id"`
	  }
	  if !mw.BindJSON(c, &body) {
		   return
	   }
	   proof, err := h.svc.proof.Verify(c.Request.Context(), strings.TrimSpace(body.ProofID))
	   recordAPI(c.FullPath(), httpStatus(err), start)
	   if err == ErrProofMismatch {
		   c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "proof": proof})
		   return
	   }
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, proof)
  }

// ── GET /ledorphan/gen/:batch_id ──────────────────────────────────────────────

   func (h *Handlers) GetGen(c *gin.Context) {
	   start := time.Now()
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	  batchID := strings.TrimSpace(c.Param("batch_id"))
	  gen,
		   err := h.svc.GetGen(c.Request.Context(), batchID)
	   recordAPI(c.FullPath(), httpStatus(err), start)
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, gen)
  }

// ── POST /ledorphan/gen/acquire ───────────────────────────────────────────────

  func (h *Handlers) AcquireGen(c *gin.Context) {
	  start := time.Now()
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	  }
	   var req AcquireGenReq
	  if !mw.BindJSON(c, &req) {
		  return
	   }
	  gen, err := h.svc.AcquireGen(c.Request.Context(), req)
	   recordAPI(c.FullPath(), httpStatus(err), start)
	   if err == ErrGenLocked {
		   c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		  return
	  }
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	   }
	  c.JSON(http.StatusOK, gen)
   }

// ── POST /ledorphan/gen/release ───────────────────────────────────────────────

   func (h *Handlers) ReleaseGen(c *gin.Context) {
	  start := time.Now()
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	   var body struct {
		  BatchID  string `json:"batch_id"`
		  WorkerID string `json:"worker_id"`
		   Generation int64 `json:"generation"`
	   }
	   if !mw.BindJSON(c, &body) {
		   return
	   }
	   err := h.svc.ReleaseGen(c.Request.Context(), body.BatchID, body.WorkerID, body.Generation)
	   recordAPI(c.FullPath(), httpStatus(err), start)
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, gin.H{"released": true})
  }

// ── 辅助 ──────────────────────────────────────────────────────────────────────

  func httpStatus(err error) int {
	  if err == nil {
		   return http.StatusOK
	   }
	   return http.StatusInternalServerError
  }

   func recordAPI(path string, status int, start time.Time) {
	   OrphanAPIRequests.WithLabelValues(path, strconv.Itoa(status)).Inc()
	  OrphanAPIDuration.WithLabelValues(path).Observe(time.Since(start).Seconds())
  }
