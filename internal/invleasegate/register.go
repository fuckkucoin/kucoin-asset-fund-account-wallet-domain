package invleasegate

import (
	"net/http"
	"strconv"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"github.com/gin-gonic/gin"
)

// Register 挂载 invleasegate API 路由。
  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  var store Store
	  if rt != nil && rt.SQL != nil && rt.SQL.DB != nil {
		  store = NewSQLStore(rt.SQL.DB)
	   } else {
		   store = NewMemStore()
	   }
	   engine := NewEngine(store, nil)
	  h := &handler{engine: engine,
		   store: store}
	  worker := NewWorker(engine, store, WorkerConfig{})
	   // Worker 随路由注册启动（中应在 svcboot 的 Start 钩子中启动）
	  // 此处通过 gin 引擎注册时启动，适合嵌入式服务模式
	   _ = worker // 避免未使用警告；实际需在启动流程中调用 worker.Start(ctx)
	  g := v1.Group("/invleasegate")
	   {
		   // 预检租约
		  g.POST("/precheck", h.precheckBind)
		  g.GET("/leases/:lease_id", h.getLease)
		  g.GET("/assets/:asset/leases", h.listLeasesByAsset)
		   // 提交复验
		   g.POST("/commit", h.commitRecheck)
		   // 漂移证明
		  g.GET("/proofs/:proof_id", h.getProof)
		   g.GET("/leases/:lease_id/proof", h.getProofByLease)
		  g.GET("/proofs", h.listOpenProofs)
		   g.POST("/proofs/:proof_id/close", h.closeProof)
	  }
   }

// handler HTTP 处理器。

type handler struct {
	engine *Engine
	store  Store
}

   func errResp(c *gin.Context, status int, err error) {
	   c.JSON(status, gin.H{"error": err.Error()})
  }

// ── 预检租约 ──────────────────────────────────────────────────────────────────

type precheckBindReqBody struct {
	Asset string `json:"asset"`
	Actor string `json:"actor"`
	LeaseTTL int `json:"lease_ttl_seconds,omitempty"` // 0 表示使用默认值
}

  func (h *handler) precheckBind(c *gin.Context) {
	  var req precheckBindReqBody
	  if err := c.ShouldBindJSON(&req); err != nil {
		  errResp(c, http.StatusBadRequest, err)
		   return
	   }
	   res, err := h.engine.PrecheckBind(c.Request.Context(), PrecheckBindReq{
		  Asset: req.Asset,
		   Actor: req.Actor,
		   LeaseTTL: durationFromSeconds(req.LeaseTTL),
	   })
	   if err != nil {
		  switch err {
		   case ErrCheckerDown:
			  errResp(c,
				   http.StatusServiceUnavailable, err)
		   case ErrActiveAdvice:
			   errResp(c, http.StatusConflict, err)
		   case ErrAsset, ErrActor:
			  errResp(c, http.StatusBadRequest, err)
		   default:
			   errResp(c, http.StatusInternalServerError, err)
		   }
		   return
	   }
	  c.JSON(http.StatusCreated, gin.H{"lease": res.Lease, "inv_precheck_id": res.Lease.LeaseID})
   }

   func (h *handler) getLease(c *gin.Context) {
	  leaseID := c.Param("lease_id")
	   lease,
		   err := h.engine.GetLease(c.Request.Context(), leaseID)
	  if err != nil {
		  if err == ErrLeaseNotFound {
			  errResp(c, http.StatusNotFound, err)
		   } else {
			  errResp(c, http.StatusInternalServerError, err)
		   }
		  return
	  }
	  c.JSON(http.StatusOK, gin.H{"lease": lease})
  }

   func (h *handler) listLeasesByAsset(c *gin.Context) {
	   asset := c.Param("asset")
	   limit := 50
	  if l := c.Query("limit"); l != "" {
		  if n, err := strconv.Atoi(l); err == nil && n > 0 {
			   limit = n
		  }
	  }
	   leases,
		   err := h.engine.ListLeasesByAsset(c.Request.Context(), asset, limit)
	  if err != nil {
		   errResp(c, http.StatusInternalServerError, err)
		  return
	   }
	  c.JSON(http.StatusOK, gin.H{"leases": leases, "count": len(leases)})
  }

// ── 提交复验 ──────────────────────────────────────────────────────────────────

type commitRecheckReqBody struct {
	LeaseID   string `json:"lease_id"`
	Asset     string `json:"asset,omitempty"`
	Actor     string `json:"actor,omitempty"`
	CommitRef string `json:"commit_ref,omitempty"`
}

   func (h *handler) commitRecheck(c *gin.Context) {
	  var req commitRecheckReqBody
	   if err := c.ShouldBindJSON(&req); err != nil {
		   errResp(c, http.StatusBadRequest, err)
		  return
	  }
	  res, err := h.engine.CommitRecheck(c.Request.Context(), CommitRecheckReq{
		   LeaseID: req.LeaseID,
		  Asset:     req.Asset,
		  Actor:     req.Actor,
		  CommitRef: req.CommitRef,
	   })
	  if err != nil {
		  switch err {
		  case ErrNoLease, ErrLeaseNotFound:
			   errResp(c,
				  http.StatusBadRequest, err)
		   case ErrLeaseExpired:
			   errResp(c, http.StatusGone, err)
		  case ErrLeaseBlocked, ErrAdviceFlip, ErrFingerprintMismatch:
			   errResp(c, http.StatusConflict, err)
		   case ErrCheckerDown:
			   errResp(c, http.StatusServiceUnavailable, err)
		  case ErrLeaseTerminal:
			  errResp(c, http.StatusConflict, err)
		   default:
			  errResp(c,
				   http.StatusInternalServerError, err)
		   }
		  return
	  }
	  c.JSON(http.StatusOK, gin.H{"lease": res.Lease, "committed": true})
  }

// ── 漂移证明 ──────────────────────────────────────────────────────────────────

   func (h *handler) getProof(c *gin.Context) {
	   proofID := c.Param("proof_id")
	  proof, err := h.engine.GetProof(c.Request.Context(), proofID)
	  if err != nil {
		  if err == ErrProofNotFound {
			   errResp(c, http.StatusNotFound, err)
		   } else {
			   errResp(c, http.StatusInternalServerError, err)
		   }
		   return
	  }
	   c.JSON(http.StatusOK, gin.H{"proof": proof})
  }

   func (h *handler) getProofByLease(c *gin.Context) {
	  leaseID := c.Param("lease_id")
	  proof, err := h.engine.GetProofByLease(c.Request.Context(), leaseID)
	   if err != nil {
		  if err == ErrProofNotFound {
			   errResp(c, http.StatusNotFound, err)
		   } else {
			  errResp(c, http.StatusInternalServerError, err)
		  }
		   return
	  }
	  c.JSON(http.StatusOK, gin.H{"proof": proof})
   }

  func (h *handler) listOpenProofs(c *gin.Context) {
	   limit := 100
	  if l := c.Query("limit"); l != "" {
		   if n, err := strconv.Atoi(l); err == nil && n > 0 {
			   limit = n
		  }
	  }
	  proofs, err := h.engine.ListOpenProofs(c.Request.Context(), limit)
	  if err != nil {
		  errResp(c, http.StatusInternalServerError, err)
		  return
	   }
	  c.JSON(http.StatusOK, gin.H{"proofs": proofs, "count": len(proofs)})
   }

type closeProofReqBody struct {
	Actor string `json:"actor"`
}

  func (h *handler) closeProof(c *gin.Context) {
	   proofID := c.Param("proof_id")
	  var req closeProofReqBody
	  if err := c.ShouldBindJSON(&req); err != nil {
		   errResp(c, http.StatusBadRequest, err)
		   return
	  }
	   proof, err := h.engine.CloseProof(c.Request.Context(), proofID, req.Actor)
	  if err != nil {
		  if err == ErrProofNotFound {
			   errResp(c, http.StatusNotFound, err)
		   } else {
			  errResp(c, http.StatusInternalServerError, err)
		   }
		   return
	  }
	  c.JSON(http.StatusOK, gin.H{"proof": proof})
   }

// durationFromSeconds 将秒数转为 time.Duration；0 或无效值返回 0（使用引擎默认）。

   func durationFromSeconds(seconds int) time.Duration {
	   if seconds <= 0 {
		   return 0
	  }
	  return time.Duration(seconds) * time.Second
  }
