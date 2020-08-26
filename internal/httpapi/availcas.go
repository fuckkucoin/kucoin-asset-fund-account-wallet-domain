// 路由前缀 /v1/avail-cas，仅供内部服务与 desk 调用，不对外暴露。
// 调用方须通过 registerAvailCAS 将路由挂载到 gin.RouterGroup。

// Author: Vance
// Date: 2020.08.24

package httpapi

import (
	"account-assets.kucoin.net/funding/internal/availcas"
	"account-assets.kucoin.net/funding/internal/overwdbar"
	"context"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// registerAvailCAS 注册并发可用校验与超提竞态屏障 API 路由。
// svc 若为 nil，该路由组注册为 503 占位。

func registerAvailCAS(v1 *gin.RouterGroup, svc *availcas.Service, guard overwdbar.Guard) {
	g := v1.Group("/avail-cas")
	if svc == nil {
		g.Any("/*path", func(c *gin.Context) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "avail-cas service not configured"})
		})
		return
	}
	// ── 租约 ──────────────────────────────────────────────────────────────
	g.POST("/leases", handleIssueAvailLease(svc))
	g.GET("/leases/:id", handleGetAvailLease(svc))
	// ── 意图/屏障 ─────────────────────────────────────────────────────────
	g.POST("/intents", handleSubmitIntent(svc))
	g.GET("/intents/:id", handleGetDecision(svc))
	// ── 竞态证明（desk） ──────────────────────────────────────────────────
	g.GET("/race-proofs/:id", handleGetRaceProof(svc))
	g.GET("/race-proofs", handleListRaceProofs(svc))
	// ── 运营快照 ──────────────────────────────────────────────────────────
	g.GET("/snapshot", handleAvailSnapshot(svc))
	// ── 预检/提交双闸 ─────────────────────────────────────────────────────
	if guard != nil {
		g.POST("/precheck", handleOverdrawPreCheck(guard, svc))
		g.POST("/commit", handleOverdrawCommit(guard))
	}
}

// availCASWriteCtx 为写操作创建带 8s 超时的 context。

func availCASWriteCtx(c *gin.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(c.Request.Context(),
		8*time.Second)
}

// availCASReadCtx 为读操作创建带 5s 超时的 context。
func availCASReadCtx(c *gin.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(c.Request.Context(), 5*time.Second)
}

// ────────────────────────────────────────────────────────────────────────────
// 租约处理器
// ────────────────────────────────────────────────────────────────────────────
// handleIssueAvailLease 申请新可用快照租约。
// 请求体：
// { "user_id":"u1", "asset":"USDT", "book":"spot", "ttl_ms":30000 }

func handleIssueAvailLease(svc *availcas.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			UserID string `json:"user_id" binding:"required"`
			Asset  string `json:"asset"   binding:"required"`
			Book   string `json:"book"    binding:"required"`
			TTLMs  int64  `json:"ttl_ms"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		opts := availcas.LeaseOptions{}
		if req.TTLMs > 0 {
			opts.TTL = time.Duration(req.TTLMs) * time.Millisecond
		}
		ctx, cancel := availCASWriteCtx(c)
		defer cancel()
		lease, err := svc.IssueLease(ctx, req.UserID, req.Asset, req.Book, opts)
		if err != nil {
			writeAvailCASErr(c, err)
			return
		}
		c.JSON(http.StatusCreated, gin.H{"lease": lease})
	}
}

// handleGetAvailLease 查询租约。

func handleGetAvailLease(svc *availcas.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := strings.TrimSpace(c.Param("id"))
		if id == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "id required"})
			return
		}
		ctx, cancel := availCASReadCtx(c)
		defer cancel()
		lease, err := svc.GetLease(ctx, id)
		if err != nil {
			writeAvailCASErr(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"lease": lease})
	}
}

// ────────────────────────────────────────────────────────────────────────────
// 意图/屏障处理器
// ────────────────────────────────────────────────────────────────────────────
// handleSubmitIntent 提交扣减/冻结意图，执行 CAS 裁决。
// 请求体：
// {
// "idem_key": "wdreq:xxx", "user_id": "u1", "asset": "USDT",
// "book": "spot", "amount": "100.00", "kind": "freeze",
// "reason": "withdraw_freeze", "lease_id": "uuid",
// "lease_gen": 5, "expected_avail": "500.00", "actor_id": "withdraw-svc"
// }

func handleSubmitIntent(svc *availcas.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			IdemKey       string `json:"idem_key"`
			UserID        string `json:"user_id"        binding:"required"`
			Asset         string `json:"asset"          binding:"required"`
			Book          string `json:"book"           binding:"required"`
			Amount        string `json:"amount"         binding:"required"`
			Kind          string `json:"kind"           binding:"required"`
			Reason        string `json:"reason"`
			LeaseID       string `json:"lease_id"       binding:"required"`
			LeaseGen      int64  `json:"lease_gen"`
			ExpectedAvail string `json:"expected_avail" binding:"required"`
			ActorID       string `json:"actor_id"`
			RequestIP     string `json:"request_ip"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		amt, err := decimal.NewFromString(req.Amount)
		if err != nil || !amt.IsPositive() {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid amount"})
			return
		}
		expAvail, err := decimal.NewFromString(req.ExpectedAvail)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid expected_avail"})
			return
		}
		intent := &availcas.DebitIntent{
			IdemKey:       req.IdemKey,
			UserID:        req.UserID,
			Asset:         req.Asset,
			Book:          req.Book,
			Amount:        amt,
			Kind:          req.Kind,
			Reason:        req.Reason,
			LeaseID:       req.LeaseID,
			LeaseGen:      req.LeaseGen,
			ExpectedAvail: expAvail,
			ActorID:       req.ActorID,
			RequestIP:     req.RequestIP,
		}
		ctx, cancel := availCASWriteCtx(c)
		defer cancel()
		decision,
			err := svc.Submit(ctx, intent)
		if err != nil {
			writeAvailCASErr(c, err)
			return
		}
		status := http.StatusOK
		if decision.Accepted {
			status = http.StatusCreated
		}
		c.JSON(status, gin.H{"decision": decision})
	}
}

// handleGetDecision 查询意图裁决结果。
func handleGetDecision(svc *availcas.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := strings.TrimSpace(c.Param("id"))
		if id == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "id required"})
			return
		}
		ctx, cancel := availCASReadCtx(c)
		defer cancel()
		decision, err := svc.GetDecision(ctx, id)
		if err != nil {
			writeAvailCASErr(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"decision": decision})
	}
}

// ────────────────────────────────────────────────────────────────────────────
// 竞态证明处理器
// ────────────────────────────────────────────────────────────────────────────
// handleGetRaceProof 查询单条竞态证明。

func handleGetRaceProof(svc *availcas.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := strings.TrimSpace(c.Param("id"))
		if id == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "id required"})
			return
		}
		ctx, cancel := availCASReadCtx(c)
		defer cancel()
		proof,
			err := svc.GetProof(ctx, id)
		if err != nil {
			writeAvailCASErr(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"proof": proof, "summary": proof.ToSummary()})
	}
}

// handleListRaceProofs 列出用户×资产竞态证明（desk 面板）。
// 查询参数：user_id, asset, limit（默认 50，最大 500）。
func handleListRaceProofs(svc *availcas.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := strings.TrimSpace(c.Query("user_id"))
		asset := strings.TrimSpace(c.Query("asset"))
		if userID == "" || asset == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "user_id and asset required"})
			return
		}
		limit := 50
		if l := c.Query("limit"); l != "" {
			if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 500 {
				limit = n
			}
		}
		ctx, cancel := availCASReadCtx(c)
		defer cancel()
		proofs, err := svc.ListProofs(ctx, userID, asset, limit)
		if err != nil {
			writeAvailCASErr(c, err)
			return
		}
		summaries := make([]availcas.ProofSummary,
			0, len(proofs))
		for _, p := range proofs {
			summaries = append(summaries, p.ToSummary())
		}
		c.JSON(http.StatusOK, gin.H{
			"user_id":   userID,
			"asset":     asset,
			"proofs":    proofs,
			"summaries": summaries,
			"count":     len(proofs),
		})
	}
}

// ────────────────────────────────────────────────────────────────────────────
// 运营快照处理器
// ────────────────────────────────────────────────────────────────────────────
// handleAvailSnapshot 即时可用余额快照（不颁发租约，仅 desk 展示）。
// 查询参数：user_id, asset, book。
func handleAvailSnapshot(svc *availcas.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := strings.TrimSpace(c.Query("user_id"))
		asset := strings.TrimSpace(c.Query("asset"))
		book := strings.TrimSpace(c.Query("book"))
		if userID == "" || asset == "" || book == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "user_id, asset, book required"})
			return
		}
		ctx, cancel := availCASReadCtx(c)
		defer cancel()
		avail, err := svc.SnapshotAvailable(ctx, userID, asset, book)
		if err != nil {
			writeAvailCASErr(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"user_id":   userID,
			"asset":     asset,
			"book":      book,
			"available": avail.String(),
		})
	}
}

// ────────────────────────────────────────────────────────────────────────────
// 预检/提交双闸处理器
// ────────────────────────────────────────────────────────────────────────────
// handleOverdrawPreCheck 预检阶段：dry-run 检验可用额是否覆盖请求额。
// 请求体：
// { "user_id":"u1", "asset":"USDT", "book":"spot",
// "lease_id":"uuid", "amount":"100.00", "actor_id":"withdraw-svc" }
func handleOverdrawPreCheck(guard overwdbar.Guard, svc *availcas.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			UserID     string `json:"user_id"   binding:"required"`
			Asset      string `json:"asset"     binding:"required"`
			Book       string `json:"book"      binding:"required"`
			LeaseID    string `json:"lease_id"  binding:"required"`
			Amount     string `json:"amount"    binding:"required"`
			ActorID    string `json:"actor_id"`
			IsLiveHash bool   `json:"is_live_hash"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		amt, err := decimal.NewFromString(req.Amount)
		if err != nil || !amt.IsPositive() {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid amount"})
			return
		}
		ctx, cancel := availCASWriteCtx(c)
		defer cancel()
		decision, err := guard.PreCheck(ctx, overwdbar.PreCheckRequest{
			UserID:     req.UserID,
			Asset:      req.Asset,
			Book:       req.Book,
			LeaseID:    req.LeaseID,
			Amount:     amt,
			ActorID:    req.ActorID,
			IsLiveHash: req.IsLiveHash,
		})
		if err != nil {
			if err == overwdbar.ErrOverdrawBlocked {
				c.JSON(http.StatusConflict, gin.H{"error": "overdraw blocked", "decision": decision})
				return
			}
			writeAvailCASErr(c, err)
			return
		}
		// 预检通过：附带租约快照，供调用方取 lease_gen+expected_avail 用于 Commit。
		lease, _ := svc.GetLease(ctx, req.LeaseID)
		c.JSON(http.StatusOK, gin.H{"decision": decision, "lease": lease})
	}
}

// handleOverdrawCommit 提交阶段：binding CAS 意图，绑定租约。
// 请求体：
// { "user_id":"u1", "asset":"USDT", "book":"spot",
// "lease_id":"uuid", "lease_gen":5, "expected_avail":"500.00",
// "amount":"100.00", "kind":"freeze", "reason":"withdraw_freeze",
// "idem_key":"wdreq:xxx", "actor_id":"withdraw-svc" }
func handleOverdrawCommit(guard overwdbar.Guard) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			UserID        string `json:"user_id"        binding:"required"`
			Asset         string `json:"asset"          binding:"required"`
			Book          string `json:"book"           binding:"required"`
			LeaseID       string `json:"lease_id"       binding:"required"`
			LeaseGen      int64  `json:"lease_gen"`
			ExpectedAvail string `json:"expected_avail" binding:"required"`
			Amount        string `json:"amount"         binding:"required"`
			Kind          string `json:"kind"           binding:"required"`
			Reason        string `json:"reason"`
			IdemKey       string `json:"idem_key"`
			ActorID       string `json:"actor_id"`
			RequestIP     string `json:"request_ip"`
			IsLiveHash    bool   `json:"is_live_hash"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		amt, err := decimal.NewFromString(req.Amount)
		if err != nil || !amt.IsPositive() {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid amount"})
			return
		}
		expAvail, err := decimal.NewFromString(req.ExpectedAvail)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid expected_avail"})
			return
		}
		ctx, cancel := availCASWriteCtx(c)
		defer cancel()
		decision, err := guard.Commit(ctx, overwdbar.CommitRequest{
			UserID:        req.UserID,
			Asset:         req.Asset,
			Book:          req.Book,
			LeaseID:       req.LeaseID,
			LeaseGen:      req.LeaseGen,
			ExpectedAvail: expAvail,
			Amount:        amt,
			Kind:          req.Kind,
			Reason:        req.Reason,
			IdemKey:       req.IdemKey,
			ActorID:       req.ActorID,
			RequestIP:     req.RequestIP,
			IsLiveHash:    req.IsLiveHash,
		})
		if err != nil {
			if err == overwdbar.ErrOverdrawBlocked {
				c.JSON(http.StatusConflict, gin.H{"error": "overdraw blocked", "decision": decision})
				return
			}
			writeAvailCASErr(c, err)
			return
		}
		c.JSON(http.StatusCreated, gin.H{"decision": decision})
	}
}

// ────────────────────────────────────────────────────────────────────────────
// 错误映射
// ────────────────────────────────────────────────────────────────────────────
// writeAvailCASErr 将 availcas/overwdbar 错误映射为 HTTP 状态。
func writeAvailCASErr(c *gin.Context, err error) {
	switch err {
	case availcas.ErrLeaseNotFound, availcas.ErrIntentNotFound, availcas.ErrProofNotFound:
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case availcas.ErrLeaseExpired, availcas.ErrLeaseRevoked:
		c.JSON(http.StatusGone, gin.H{"error": err.Error(), "retryable": true})
	case availcas.ErrCASConflict:
		c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "retryable": true})
	case availcas.ErrInsufficientAvailable:
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
	case availcas.ErrUserRequired, availcas.ErrAssetRequired, availcas.ErrBookRequired,
		availcas.ErrAmountNonPositive:
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case availcas.ErrStoreUnavailable:
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}
