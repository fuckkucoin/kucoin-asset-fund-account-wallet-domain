// 路径挂 /destlife/*，与 /destallow/* 不交叉。
// 提供生命周期查询、强制 revoking、绕过证明查询、世代对账、冷却策略更新。
// 不变量：
// - 所有写操作需运营权限（perm.OpsWrite）。
// - 返回 entry_life_id / bypass_proof_id 供他包只读引用。
// - 不暴露风险簇图谱字段；不读写动态限额占用表。

// Author: Cason
// Date: 2021.04.11

package destlife

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

  func init() {
	   apierr.Register(ErrInflightBlock, http.StatusConflict)
	  apierr.Register(ErrCooldownActive, http.StatusForbidden)
	   apierr.Register(ErrStateRevoked, http.StatusGone)
	  apierr.Register(ErrBypassBlocked,
		  http.StatusForbidden)
	   apierr.Register(ErrEntryNotFound, http.StatusNotFound)
	  apierr.Register(ErrDuplicateRef, http.StatusConflict)
  }

// Handler destlife HTTP 处理器集合。
type Handler struct {
	cal *Calendar
	lock *InflightLock
	guard *BypassGuard
	id    callerid.Opt
}

// NewHandler 构造 Handler。
   func NewHandler(cal *Calendar, lock *InflightLock, guard *BypassGuard, id callerid.Opt) *Handler {
	   return &Handler{cal: cal,
		   lock: lock, guard: guard, id: id}
  }

// Register 注册 destlife 路由到 /destlife/* 前缀。

  func Register(v1 *gin.RouterGroup, cal *Calendar, lock *InflightLock, guard *BypassGuard, id callerid.Opt) {
	  if v1 == nil {
		  return
	  }
	   h := NewHandler(cal, lock, guard, id)
	   h.register(v1)
   }

  func (h *Handler) register(v1 *gin.RouterGroup) {
	  // 只读查询。
	  v1.GET("/destlife/entry", h.getEntry)
	   v1.GET("/destlife/entries", h.listEntries)
	  v1.GET("/destlife/cooldown", h.checkCooldown)
	   v1.GET("/destlife/audit", h.listAudit)
	   v1.GET("/destlife/inflight", h.listInflight)
	   v1.GET("/destlife/bypass-proofs", h.listBypassProofs)
	   v1.GET("/destlife/gen-check", h.genCheck)
	   // 写操作（运营）。
	  v1.POST("/destlife/force-revoking", h.forceRevoking)
	  v1.POST("/destlife/release-ref", h.releaseRef)
	  v1.POST("/destlife/window", h.setWindow)
	   v1.POST("/destlife/acquire-ref", h.acquireRef)
   }

// GET /destlife/entry?entry_life_id=xxx

   func (h *Handler) getEntry(c *gin.Context) {
	   id := strings.TrimSpace(c.Query("entry_life_id"))
	  if id == "" {
		   mw.WriteProblem(c, errors.New("entry_life_id required"))
		  return
	   }
	   e, err := h.cal.GetLifeByID(c.Request.Context(), id)
	   if err != nil {
		   mw.WriteProblem(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, gin.H{"entry": e})
  }

// GET /destlife/entries?user_id=xxx

  func (h *Handler) listEntries(c *gin.Context) {
	   userID, err := callerid.Bind(c, c.Query("user_id"), h.id)
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	   entries, err := h.cal.ListUser(c.Request.Context(), userID)
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	   c.JSON(http.StatusOK, gin.H{"user_id": userID, "entries": entries, "total": len(entries)})
  }

// GET /destlife/cooldown?user_id=xxx&chain=BTC&address=abc
   func (h *Handler) checkCooldown(c *gin.Context) {
	   userID, err := callerid.Bind(c, c.Query("user_id"), h.id)
	  if err != nil {
		   mw.WriteProblem(c, err)
		   return
	   }
	  chain := strings.ToUpper(strings.TrimSpace(c.Query("chain")))
	   addr := strings.TrimSpace(c.Query("address"))
	   remaining, state, err := h.cal.CheckCooldown(c.Request.Context(), userID, chain, addr)
	  if err != nil {
		  if errors.Is(err, ErrEntryNotFound) {
			  c.JSON(http.StatusNotFound, gin.H{"found": false})
			   return
		   }
		  mw.WriteProblem(c, err)
		  return
	  }
	   c.JSON(http.StatusOK, gin.H{
		   "user_id": userID,
		  "chain": chain,
		   "address":           addr,
		  "state":             state,
		   "cooling_remaining": remaining.String(),
		   "ready": remaining <= 0 && state == StateActive,
	  })
  }

// GET /destlife/audit?entry_life_id=xxx&limit=50
   func (h *Handler) listAudit(c *gin.Context) {
	  entryLifeID := strings.TrimSpace(c.Query("entry_life_id"))
	   userID := strings.TrimSpace(c.Query("user_id"))
	  limit := parseLimit(c.Query("limit"), 50)
	   ctx := c.Request.Context()
	   var rows []LifecycleAuditRow
	  var err error
	  if entryLifeID != "" {
		  rows, err = h.cal.ListAuditTrail(ctx, entryLifeID, limit)
	   } else if userID != "" {
		   rows,
			  err = h.cal.ListAuditByUser(ctx, userID, limit)
	   } else {
		  mw.WriteProblem(c, errors.New("entry_life_id or user_id required"))
		   return
	   }
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, gin.H{"audit": rows, "total": len(rows)})
   }

// GET /destlife/inflight?entry_life_id=xxx&limit=20
  func (h *Handler) listInflight(c *gin.Context) {
	   entryLifeID := strings.TrimSpace(c.Query("entry_life_id"))
	  if entryLifeID == "" {
		  mw.WriteProblem(c, errors.New("entry_life_id required"))
		   return
	  }
	  limit := parseLimit(c.Query("limit"), 20)
	   ctx := c.Request.Context()
	   summary, err := h.lock.Summary(ctx, entryLifeID)
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	  refs, err := h.lock.ListByEntry(ctx, entryLifeID, limit)
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, gin.H{"summary": summary, "refs": refs})
   }

// GET /destlife/bypass-proofs?entry_life_id=xxx&limit=20

  func (h *Handler) listBypassProofs(c *gin.Context) {
	   entryLifeID := strings.TrimSpace(c.Query("entry_life_id"))
	  if entryLifeID == "" {
		   mw.WriteProblem(c, errors.New("entry_life_id required"))
		  return
	  }
	  limit := parseLimit(c.Query("limit"), 20)
	  proofs,
		  err := h.guard.ListProofs(c.Request.Context(), entryLifeID, limit)
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	   var reports []DriftReport
	  for _, p := range proofs {
		  reports = append(reports, h.guard.BuildDriftReport(p))
	   }
	   c.JSON(http.StatusOK, gin.H{"proofs": proofs, "drift_reports": reports, "total": len(proofs)})
  }

// GET /destlife/gen-check?entry_life_id=xxx&book_gen=42
  func (h *Handler) genCheck(c *gin.Context) {
	  entryLifeID := strings.TrimSpace(c.Query("entry_life_id"))
	   if entryLifeID == "" {
		  mw.WriteProblem(c, errors.New("entry_life_id required"))
		   return
	  }
	  bookGen, _ := strconv.ParseInt(c.Query("book_gen"), 10, 64)
	   err := h.cal.VerifyGenAlignment(c.Request.Context(), entryLifeID, bookGen)
	  if err != nil {
		  var mm *GenMismatch
		  if errors.As(err, &mm) {
			   GenMismatches.WithLabelValues("api").Inc()
			   c.JSON(http.StatusConflict, gin.H{"aligned": false, "mismatch": mm})
			  return
		   }
		  mw.WriteProblem(c, err)
		   return
	  }
	   c.JSON(http.StatusOK, gin.H{"aligned": true, "entry_life_id": entryLifeID, "book_gen": bookGen})
   }

// POST /destlife/force-revoking
// body: {"entry_life_id":"xxx","actor":"ops","note":"..."}
   func (h *Handler) forceRevoking(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	   var req RevokingRequest
	  if err := c.ShouldBindJSON(&req); err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	   ctx := c.Request.Context()
	  e,
		  err := h.cal.GetLifeByID(ctx, req.EntryLifeID)
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	   }
	   updated, err := h.cal.BeginRevoking(ctx, req.EntryLifeID, req.Actor, req.Note, e.Generation)
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	   StateTransitions.WithLabelValues(string(e.State), string(StateRevoking)).Inc()
	   c.JSON(http.StatusOK, gin.H{"entry": updated, "prev_state": e.State})
   }

// POST /destlife/acquire-ref
// body: {"entry_life_id":"xxx","user_id":"u","chain":"BTC","address":"abc","movement_id":"m123"}
  func (h *Handler) acquireRef(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	  var body struct {
		   EntryLifeID string `json:"entry_life_id"`
		  UserID      string `json:"user_id"`
		   Chain string `json:"chain"`
		  Address    string `json:"address"`
		  MovementID string `json:"movement_id"`
	   }
	  if err := c.ShouldBindJSON(&body); err != nil {
		  mw.WriteProblem(c, err)
		   return
	   }
	  ref, err := h.lock.AcquireRef(
		   c.Request.Context(),
		  body.EntryLifeID, body.UserID, body.Chain, body.Address, body.MovementID,
	  )
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	  InflightAcquire.WithLabelValues("ok").Inc()
	   ActiveInflightGauge.Inc()
	  c.JSON(http.StatusOK, gin.H{"ref": ref})
  }

// POST /destlife/release-ref
// body: {"ref_id":"xxx","actor":"worker","book_gen":42}

   func (h *Handler) releaseRef(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	   var body struct {
		  RefID string `json:"ref_id"`
		  Actor string `json:"actor"`
		   BookGen int64 `json:"book_gen"`
	   }
	  if err := c.ShouldBindJSON(&body); err != nil {
		   mw.WriteProblem(c, err)
		  return
	  }
	  if err := h.lock.ReleaseRef(c.Request.Context(), body.RefID, body.Actor, body.BookGen); err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	   InflightRelease.WithLabelValues("false").Inc()
	  ActiveInflightGauge.Dec()
	  c.JSON(http.StatusOK, gin.H{"released": true, "ref_id": body.RefID})
  }

// POST /destlife/window
// body: {"scope":"global","seconds":86400}

   func (h *Handler) setWindow(c *gin.Context) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	  var body struct {
		  Scope   string `json:"scope"`
		   Seconds int64  `json:"seconds"`
	   }
	   if err := c.ShouldBindJSON(&body); err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	   ctx := c.Request.Context()
	  var err error
	  if body.Scope == "global" || body.Scope == "" {
		  err = h.cal.SetGlobalWindow(ctx, body.Seconds)
	   } else {
		  err = h.cal.SetUserWindow(ctx, body.Scope, body.Seconds)
	  }
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	  c.JSON(http.StatusOK, gin.H{"scope": body.Scope, "seconds": body.Seconds, "ok": true})
   }

// LifecycleGate 出金闸生命周期检验（供 destallow 调用）。
// 返回非 nil 则拒绝出金；同时触发 BypassGuard 检验。
  func LifecycleGate(ctx context.Context,
	   cal *Calendar, guard *BypassGuard, lock *InflightLock,
	   entryLifeID, userID, chain, addr string, bookGen int64, gatePassedAt ...time.Time) error {
	   // 1. 检查世代一致。
	   if err := cal.VerifyGenAlignment(ctx, entryLifeID, bookGen); err != nil {
		   var mm *GenMismatch
		   if errors.As(err, &mm) {
			  GenMismatches.WithLabelValues("strict").Inc()
			   if guard != nil {
				  _ = guard.ProveGenMismatch(ctx, mm, userID, chain, addr)
			  }
		  }
		   return err
	   }
	   // 2. 冷却检查。
	  remaining, state, err := cal.CheckCooldown(ctx, userID, chain, addr)
	   if err != nil {
		   CooldownChecks.WithLabelValues("error").Inc()
		  return err
	   }
	  if state == StateRevoked {
		  return ErrStateRevoked
	   }
	  if remaining > 0 || state == StateCooling {
		  CooldownChecks.WithLabelValues("block").Inc()
		   return ErrCooldownActive
	  }
	  // 3. BypassGuard 检验（捕获闸逻辑 bug 或时钟漂移）。
	   if guard != nil {
		   var passedAt = cal.clock()
		   if len(gatePassedAt) > 0 {
			  passedAt = gatePassedAt[0]
		  }
		  if err := guard.CheckAndProve(ctx, entryLifeID, userID, chain, addr, passedAt); err != nil {
			   BypassDetected.WithLabelValues(ReasonCooldownSkip).Inc()
			   return err
		  }
	  }
	   CooldownChecks.WithLabelValues("pass").Inc()
	  return nil
  }

  func parseLimit(s string, def int) int {
	   if s == "" {
		  return def
	   }
	  n, err := strconv.Atoi(s)
	  if err != nil || n <= 0 {
		  return def
	   }
	  if n > 500 {
		   return 500
	   }
	  return n
   }
