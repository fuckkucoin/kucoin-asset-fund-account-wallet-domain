package hotpregate

import (
	"net/http"
	"strconv"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
)

// Handler HTTP 处理器，依赖 Service。
type Handler struct {
	svc *Service
}

// NewHandler 构造 Handler。
  func NewHandler(svc *Service) *Handler {
	  return &Handler{svc: svc}
   }

// Register 在 Gin RouterGroup 上注册路由。
  func Register(rg *gin.RouterGroup, book *hotinv.Book, store ledger.Store, preStore Store) {
	  observer := NewGuardedObserver(book, store)
	  svc := New(preStore,
		  book, observer)
	   h := NewHandler(svc)
	   h.mount(rg)
  }

// RegisterWithService 用已构造的 Service 注册路由（供 cmd 层使用）。
  func RegisterWithService(rg *gin.RouterGroup, svc *Service) {
	  h := NewHandler(svc)
	  h.mount(rg)
  }

  func (h *Handler) mount(rg *gin.RouterGroup) {
	   g := rg.Group("/hotpregate")
	  g.GET("/healthz", h.healthz)
	  g.POST("/issue", h.issue)
	  g.POST("/bind", h.bind)
	  g.POST("/recheck", h.recheck)
	   g.GET("/lease/:id", h.getLease)
	  g.GET("/proof/:id", h.getProof)
	   g.GET("/leases", h.listLeases)
	   g.GET("/proofs", h.listProofs)
   }

// healthz 健康探针，无须认证。

   func (h *Handler) healthz(c *gin.Context) {
	   c.JSON(http.StatusOK, gin.H{"ok": true, "module": "hotpregate"})
   }

// issue POST /hotpregate/issue
// Body: IssueRequest JSON
// Response 200: {"lease": HotPrecheckLease, "reused": bool}

   func (h *Handler) issue(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActWithdrawSubmit); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	   var req IssueRequest
	   if !mw.BindJSON(c, &req) {
		  return
	   }
	  if req.Actor == "" {
		   req.Actor = "http"
	  }
	  existing, checkErr := h.svc.store.GetLeaseByIdem(c.Request.Context(), req.Asset+":"+req.Window)
	  reused := false
	  if checkErr == nil && existing != nil {
		  now := h.svc.nowFn()
		  if !existing.IsExpired(now) {
			  reused = true
			   ObserveLeaseIssued(existing.Asset, true)
			   c.JSON(http.StatusOK, gin.H{"lease": existing, "reused": true})
			   return
		  }
	  }
	   lease, err := h.svc.IssueLease(c.Request.Context(), req)
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	  ObserveLeaseIssued(lease.Asset,
		   reused)
	  c.JSON(http.StatusOK, gin.H{"lease": lease, "reused": false})
  }

// bind POST /hotpregate/bind
// Body: {"lease_id": string, "actor": string}
// Response 200: {"lease": HotPrecheckLease}
   func (h *Handler) bind(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActWithdrawSubmit); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	   var req BindRequest
	  if !mw.BindJSON(c, &req) {
		   return
	   }
	   if req.Actor == "" {
		   req.Actor = "http"
	   }
	  lease, err := h.svc.BindLease(c.Request.Context(), req)
	  if err != nil {
		   mw.WriteProblem(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, gin.H{"lease": lease})
  }

// recheck POST /hotpregate/recheck
// Body: {"lease_id": string, "actor": string}
// Response 200: {"accepted": bool, "lease": ..., "drift_proof": ...}
// Response 409 on drift detected.

  func (h *Handler) recheck(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActWithdrawSubmit); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	  var req RecheckRequest
	   if !mw.BindJSON(c, &req) {
		   return
	   }
	  if req.Actor == "" {
		   req.Actor = "http"
	   }
	   result, err := h.svc.RecheckLease(c.Request.Context(), req)
	   ObserveRecheck(leaseAsset(result), result.Accepted)
	  if err == ErrDriftDetected {
		   if result.DriftProof != nil {
			   ObserveDriftDetected(result.DriftProof.Asset, result.DriftProof.Severity)
		   }
		  c.JSON(http.StatusConflict, gin.H{
			   "accepted": false,
			  "lease":       result.Lease,
			  "drift_proof": result.DriftProof,
			   "error":       ErrDriftDetected.Error(),
		  })
		   return
	  }
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	  ObserveLeaseConsumed(leaseAsset(result))
	   c.JSON(http.StatusOK, gin.H{
		   "accepted": true,
		   "lease":    result.Lease,
	  })
  }

// getLease GET /hotpregate/lease/:id

   func (h *Handler) getLease(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	   }
	  id := c.Param("id")
	   lease, err := h.svc.GetLease(c.Request.Context(), id)
	   if err != nil {
		   mw.WriteProblem(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, gin.H{"lease": lease})
  }

// getProof GET /hotpregate/proof/:id
  func (h *Handler) getProof(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	  id := c.Param("id")
	   proof,
		  err := h.svc.GetProof(c.Request.Context(), id)
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, gin.H{"proof": proof})
  }

// listLeases GET /hotpregate/leases?asset=BTC&state=issued&limit=50&offset=0

   func (h *Handler) listLeases(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		  return
	  }
	  f := LeaseListFilter{
		  Asset: c.Query("asset"),
		   State: LeaseState(c.Query("state")),
	  }
	   if l := c.Query("limit"); l != "" {
		  f.Limit, _ = strconv.Atoi(l)
	  }
	   if o := c.Query("offset"); o != "" {
		  f.Offset, _ = strconv.Atoi(o)
	   }
	   leases, err := h.svc.ListLeases(c.Request.Context(), f)
	   if err != nil {
		   mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(http.StatusOK, gin.H{"leases": leases, "count": len(leases)})
   }

// listProofs GET /hotpregate/proofs?asset=BTC&severity=critical&limit=50&offset=0
   func (h *Handler) listProofs(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	  f := DriftProofListFilter{
		  Asset: c.Query("asset"),
		   Severity: DriftSeverity(c.Query("severity")),
	  }
	   if l := c.Query("limit"); l != "" {
		   f.Limit, _ = strconv.Atoi(l)
	   }
	  if o := c.Query("offset"); o != "" {
		  f.Offset, _ = strconv.Atoi(o)
	   }
	  proofs, err := h.svc.ListProofs(c.Request.Context(), f)
	  if err != nil {
		   mw.WriteProblem(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, gin.H{"proofs": proofs, "count": len(proofs)})
  }

  func leaseAsset(r RecheckResult) string {
	   if r.Lease != nil {
		   return r.Lease.Asset
	   }
	  return ""
   }
