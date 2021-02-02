package airimportgate

import (
	"net/http"
	"strings"
	"account-assets.kucoin.net/funding/internal/app"
	"github.com/gin-gonic/gin"
)

// Register 挂载 airimportgate API 路由。

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  var store Store
	  if rt != nil && rt.SQL != nil && rt.SQL.DB != nil {
		  store = NewSQLStore(rt.SQL.DB)
	   } else {
		  store = NewMemStore()
	  }
	  engine := NewEngine(store, nil)
	  h := &handler{engine: engine, store: store}
	   g := v1.Group("/airimportgate")
	   {
		  // 导入租约
		   g.POST("/leases", h.beginImport)
		  g.GET("/leases/:lease_id", h.getLease)
		  g.POST("/leases/:lease_id/commit", h.commitImport)
		  g.POST("/leases/:lease_id/rollback", h.rollbackImport)
		   g.POST("/leases/:lease_id/force-abort", h.forceAbort)
		   g.GET("/sessions/:session_id/leases", h.listLeases)
		  // 密封 blob 摘要
		  g.GET("/digests/:digest_hex", h.getDigest)
		  g.GET("/sessions/:session_id/digests", h.listDigests)
		  // 相位冲突证明
		  g.GET("/conflicts/:conflict_id", h.getConflict)
		   g.POST("/conflicts/:conflict_id/prove", h.proveConflict)
		   g.POST("/conflicts/:conflict_id/close", h.closeConflict)
		   g.GET("/sessions/:session_id/conflicts", h.listConflicts)
		   // 法定人数绑定
		  g.GET("/binds/by-lease/:lease_id", h.getBindByLease)
	  }
  }

// handler HTTP 处理器。

type handler struct {
	engine *Engine
	store Store
}

  func errResp(c *gin.Context, status int, err error) {
	  c.JSON(status, gin.H{"error": err.Error()})
  }

// ── 导入租约 ──────────────────────────────────────────────────────────────────

type beginImportReq struct {
	SessionID string `json:"session_id"`
	ImportGen string `json:"import_gen"`
	BlobDigest string `json:"blob_digest"`
	Actor      string `json:"actor"`
}

   func (h *handler) beginImport(c *gin.Context) {
	  var req beginImportReq
	   if err := c.ShouldBindJSON(&req); err != nil {
		   errResp(c, http.StatusBadRequest, err)
		  return
	  }
	  result, err := h.engine.BeginImport(c.Request.Context(), BeginImportReq{
		  SessionID: req.SessionID,
		   ImportGen: req.ImportGen,
		  BlobDigest: req.BlobDigest,
		   Actor: req.Actor,
	   })
	   if err != nil {
		  status := mapErrStatus(err)
		   errResp(c, status, err)
		  return
	   }
	  c.JSON(http.StatusCreated, result.Lease)
   }

   func (h *handler) getLease(c *gin.Context) {
	   id := strings.TrimSpace(c.Param("lease_id"))
	  lease, err := h.engine.GetLease(c.Request.Context(), id)
	  if err != nil {
		   errResp(c, mapErrStatus(err), err)
		   return
	   }
	  c.JSON(http.StatusOK, lease)
  }

type commitImportReq struct {
	SessionID    string `json:"session_id"`
	ActualQuorum int    `json:"actual_quorum"`
	Threshold int `json:"threshold"`
	BlobDigest string `json:"blob_digest"`
	Actor string `json:"actor"`
}

  func (h *handler) commitImport(c *gin.Context) {
	  leaseID := strings.TrimSpace(c.Param("lease_id"))
	   var req commitImportReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		  errResp(c, http.StatusBadRequest, err)
		   return
	   }
	   err := h.engine.CommitImport(c.Request.Context(), CommitImportReq{
		   LeaseID:      leaseID,
		  SessionID:    req.SessionID,
		  ActualQuorum: req.ActualQuorum,
		  Threshold:    req.Threshold,
		  BlobDigest: req.BlobDigest,
		  Actor:      req.Actor,
	   })
	  if err != nil {
		   errResp(c, mapErrStatus(err), err)
		  return
	  }
	   c.JSON(http.StatusOK, gin.H{"status": "imported"})
  }

type rollbackReq struct {
	Actor string `json:"actor"`
	Reason string `json:"reason"`
}

  func (h *handler) rollbackImport(c *gin.Context) {
	  leaseID := strings.TrimSpace(c.Param("lease_id"))
	  var req rollbackReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		  errResp(c, http.StatusBadRequest, err)
		  return
	  }
	  err := h.engine.RollbackImport(c.Request.Context(), RollbackImportReq{
		   LeaseID: leaseID,
		   Actor: req.Actor,
		  Reason: req.Reason,
	   })
	   if err != nil {
		  errResp(c,
			  mapErrStatus(err), err)
		   return
	  }
	   c.JSON(http.StatusOK, gin.H{"status": "aborted"})
  }

type forceAbortReq struct {
	ConfirmerOne string `json:"confirmer_one"`
	ConfirmerTwo string `json:"confirmer_two"`
	Reason string `json:"reason"`
}

  func (h *handler) forceAbort(c *gin.Context) {
	   leaseID := strings.TrimSpace(c.Param("lease_id"))
	   var req forceAbortReq
	  if err := c.ShouldBindJSON(&req); err != nil {
		   errResp(c, http.StatusBadRequest, err)
		  return
	  }
	  err := h.engine.ForceAbort(c.Request.Context(), ForceAbortReq{
		  LeaseID:      leaseID,
		  ConfirmerOne: req.ConfirmerOne,
		   ConfirmerTwo: req.ConfirmerTwo,
		   Reason:       req.Reason,
	   })
	  if err != nil {
		  errResp(c, mapErrStatus(err), err)
		  return
	  }
	  c.JSON(http.StatusOK, gin.H{"status": "force_aborted"})
  }

   func (h *handler) listLeases(c *gin.Context) {
	   sessionID := strings.TrimSpace(c.Param("session_id"))
	   leases,
		   err := h.store.ListLeasesBySession(c.Request.Context(), sessionID, 50)
	  if err != nil {
		   errResp(c, http.StatusInternalServerError, err)
		  return
	  }
	   c.JSON(http.StatusOK, gin.H{"leases": leases})
   }

// ── 摘要 ──────────────────────────────────────────────────────────────────────

  func (h *handler) getDigest(c *gin.Context) {
	  hex := strings.TrimSpace(c.Param("digest_hex"))
	  d, err := h.engine.GetDigestByHex(c.Request.Context(), hex)
	   if err != nil {
		   errResp(c, mapErrStatus(err), err)
		   return
	   }
	  c.JSON(http.StatusOK, d)
  }

   func (h *handler) listDigests(c *gin.Context) {
	   sessionID := strings.TrimSpace(c.Param("session_id"))
	  digests, err := h.store.ListDigestsBySession(c.Request.Context(), sessionID)
	   if err != nil {
		  errResp(c, http.StatusInternalServerError, err)
		   return
	  }
	  c.JSON(http.StatusOK, gin.H{"digests": digests})
  }

// ── 冲突证明 ──────────────────────────────────────────────────────────────────

   func (h *handler) getConflict(c *gin.Context) {
	   id := strings.TrimSpace(c.Param("conflict_id"))
	  p,
		  err := h.engine.GetConflict(c.Request.Context(), id)
	  if err != nil {
		   errResp(c,
			  mapErrStatus(err), err)
		  return
	   }
	   c.JSON(http.StatusOK, p)
  }

   func (h *handler) proveConflict(c *gin.Context) {
	   id := strings.TrimSpace(c.Param("conflict_id"))
	  if err := h.engine.ProveConflict(c.Request.Context(), id); err != nil {
		  errResp(c, mapErrStatus(err), err)
		   return
	  }
	  c.JSON(http.StatusOK, gin.H{"status": "proven"})
  }

  func (h *handler) closeConflict(c *gin.Context) {
	  id := strings.TrimSpace(c.Param("conflict_id"))
	  if err := h.engine.CloseConflict(c.Request.Context(), id); err != nil {
		  errResp(c, mapErrStatus(err), err)
		  return
	  }
	  c.JSON(http.StatusOK, gin.H{"status": "closed"})
   }

  func (h *handler) listConflicts(c *gin.Context) {
	   sessionID := strings.TrimSpace(c.Param("session_id"))
	   conflicts, err := h.engine.ListOpenConflicts(c.Request.Context(), sessionID)
	  if err != nil {
		  errResp(c, http.StatusInternalServerError, err)
		   return
	  }
	  c.JSON(http.StatusOK, gin.H{"conflicts": conflicts})
   }

// ── 法定人数绑定 ──────────────────────────────────────────────────────────────

  func (h *handler) getBindByLease(c *gin.Context) {
	   leaseID := strings.TrimSpace(c.Param("lease_id"))
	  b, err := h.engine.GetBindByLease(c.Request.Context(), leaseID)
	   if err != nil {
		   errResp(c, mapErrStatus(err), err)
		   return
	   }
	  c.JSON(http.StatusOK, b)
  }

// mapErrStatus 将业务错误映射为 HTTP 状态码。
  func mapErrStatus(err error) int {
	  switch err {
	  case ErrLeaseNotFound, ErrConflictNotFound, ErrDigestNotFound, ErrBindNotFound, ErrSessionNotFound:
		  return http.StatusNotFound
	  case ErrBlobReplay, ErrConcurrentImport, ErrPhaseConflict, ErrQuorumDrift:
		  return http.StatusConflict
	   case ErrImportGenExpired, ErrLeaseExpired:
		   return http.StatusGone
	   case ErrImportGenInvalid, ErrInvalidActor, ErrEmptySessionID, ErrEmptyBlob, ErrBlobDigestMismatch:
		   return http.StatusBadRequest
	   case ErrLeaseTerminal,
		   ErrInvalidTransition:
		  return http.StatusUnprocessableEntity
	   case ErrUnavailable:
		  return http.StatusServiceUnavailable
	  default:
		   return http.StatusInternalServerError
	  }
  }
