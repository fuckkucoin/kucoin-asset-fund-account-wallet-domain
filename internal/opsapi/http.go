// 功能：opsapi/http.go
// 作者：Draven
// 日期：2021.01.02

package opsapi

import (
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/auditlog"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/deposit"
	"account-assets.kucoin.net/funding/internal/ha"
	"account-assets.kucoin.net/funding/internal/investigate"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/ledgersvc"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/opsfc"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/policy"
	"account-assets.kucoin.net/funding/internal/recon"
	"account-assets.kucoin.net/funding/internal/usage"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/internal/withdraw"
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
	"os"
	"strconv"
	"time"
)

  func status(err error) int {
	  if errors.Is(err, ha.ErrNotLeader) || errors.Is(err, ha.ErrLostLease) {
		  return http.StatusConflict
	   }
	   return apierr.HTTPStatus(err)
   }

   func requireOps(c *gin.Context, el *ha.Elector) (perm.Actor, bool) {
	   actor, code, err := opsfc.Check(c.Request, perm.ActOpsAdmin)
	  if err != nil {
		   c.JSON(code, gin.H{"error": err.Error()})
		   return actor, false
	   }
	  if el != nil {
		  if err := el.RequireLeader(); err != nil {
			   c.JSON(status(err), gin.H{"error": err.Error(), "ha": el.Status()})
			   return actor, false
		  }
	   }
	   return actor, true
  }

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   el := ha.FromEnv(rt.Redis)
	  if os.Getenv("HA_CAMPAIGN") != "0" {
		   _, _ = el.Campaign(context.Background())
	  }
	   hub := NewHub(rt, el)
	   v1.GET("/ops/ha", func(c *gin.Context) {
		   if opsfc.Reject(c, perm.ActOpsAdmin) {
			   return
		   }
		   if os.Getenv("HA_CAMPAIGN") != "0" {
			   _,
				  _ = el.Campaign(c.Request.Context())
		   }
		  body, code := rt.Ready(c.Request.Context())
		   c.JSON(200, gin.H{"ha": el.Status(), "ready": body, "ready_code": code})
	  })
	  v1.POST("/ops/admin/freeze", func(c *gin.Context) {
		   actor, ok := requireOps(c, el)
		   if !ok {
			  return
		  }
		  if err := el.RequireLeader(); err != nil {
			  c.JSON(status(err), gin.H{"error": err.Error(), "ha": el.Status()})
			  return
		   }
		  var body struct {
			   UserID string `json:"user_id"`
			   Frozen *bool  `json:"frozen"`
			  Reason string `json:"reason"`
			   Cascade bool `json:"cascade"`
		  }
		  if err := c.ShouldBindJSON(&body); err != nil {
			   c.JSON(400, gin.H{"error": err.Error()})
			  return
		   }
		   uid, err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  body.UserID = uid
		  if body.Frozen == nil {
			  c.JSON(400, gin.H{"error": "frozen required"})
			   return
		   }
		   if rt.Durable.Recon != nil {
			   if err := recon.GuardFreezeChange(recon.WrapBook(rt.Durable.Recon, 0), body.UserID); err != nil {
				  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
				  return
			  }
		   }
		  if err := rt.Ledger.SetFrozenGate(c.Request.Context(), body.UserID, *body.Frozen, body.Cascade && !*body.Frozen); err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		  }
		  if err := rt.Ledger.AppendAudit(c.Request.Context(), ledger.AuditEvent{
			  EntityType: "user", EntityID: body.UserID, Action: "ops_freeze", Actor: actor.ID,
			  Detail: map[string]any{"frozen": *body.Frozen, "reason": body.Reason, "cascade": body.Cascade},
			  CreatedAt: time.Now().UTC(),
		  }); err != nil {
			  c.JSON(503, gin.H{"error": err.Error()})
			  return
		  }
		   got, _ := rt.Ledger.Frozen(c.Request.Context(), body.UserID)
		  metrics.OpsAdmin.WithLabelValues("freeze").Inc()
		  c.JSON(200, gin.H{"user_id": body.UserID, "frozen": got, "reason": body.Reason, "actor": actor.ID, "ha": el.Status()})
	  })
	  v1.POST("/ops/admin/reconcile", func(c *gin.Context) {
		  actor := perm.FromRequest(c.Request, "")
		   if err := actor.Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   if err := el.RequireLeader(); err != nil {
			  c.JSON(status(err), gin.H{"error": err.Error(), "ha": el.Status()})
			  return
		  }
		   rep, err := ledgersvc.ReconcileUsers(c.Request.Context(), rt)
		  if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error(), "report": rep})
			   return
		  }
		   metrics.OpsAdmin.WithLabelValues("reconcile").Inc()
		   c.JSON(200, gin.H{"report": rep, "actor": actor.ID, "ha": el.Status()})
	   })
	   v1.GET("/ops/admin/status", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  body, code := rt.Ready(c.Request.Context())
		  c.JSON(200, gin.H{
			  "health":     gin.H{"ok": true},
			   "ready":      body,
			   "ready_code": code,
			  "ha":         el.Status(),
			   "region": hub.Pair.Status(),
			  "label":  "dual-region write fence",
		   })
	   })
	   // 出金跟踪表 + 入金 desk Hydrate 健康； SQL 行数与内存。
	  // desk 与 wd-track 同面暴露 hydrate_err，不得吞没 Attach 错误。
	   v1.GET("/ops/admin/wd-track-health", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  ok := true
		  var wd map[string]any
		   svc := withdraw.FromRuntime(rt)
		   if svc == nil {
			   wd = map[string]any{"ok": true, "bound": false, "hint": "withdraw service not attached"}
		   } else {
			  wd = svc.WDTrackHealth(c.Request.Context())
			  if v, okk := wd["ok"].(bool); okk && !v {
				  ok = false
			  }
		  }
		   var desk any = map[string]any{"ok": true, "bound": false}
		  if dep := deposit.FromRuntime(rt); dep != nil && dep.Desk() != nil {
			   h := dep.Desk().Health()
			  desk = h
			   if !h.OK {
				  ok = false
			  }
			   deposit.EmitDeskHydrateMetrics(h)
		   }
		  // 保留既有顶层字段（broadcasts/fee_bumps/...）并补 desk。
		   out := gin.H{"ok": ok, "dep_desk": desk}
		  for k, v := range wd {
			   out[k] = v
		  }
		  c.JSON(200, out)
	   })
	  v1.POST("/ops/admin/expire-cases", func(c *gin.Context) {
		   actor, ok := requireOps(c, el)
		   if !ok {
			  return
		  }
		  expired := hub.Pipe.ExpireStale()
		   for _, cs := range expired {
			  hub.SLA.Close(cs.ID,
				  string(cs.State))
		  }
		   metrics.OpsAdmin.WithLabelValues("expire_cases").Inc()
		  c.JSON(200, gin.H{"expired": expired, "actor": actor.ID})
	  })
	  v1.GET("/ops/admin/sla", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   c.JSON(200, gin.H{"open": hub.SLA.OpenCount(), "tickets": hub.SLA.List(50)})
	   })
	   v1.POST("/ops/admin/sla/scan", func(c *gin.Context) {
		  actor, ok := requireOps(c, el)
		   if !ok {
			  return
		   }
		  breached := hub.SLA.Scan()
		  metrics.OpsAdmin.WithLabelValues("sla_scan").Inc()
		  c.JSON(200, gin.H{"breached": breached, "actor": actor.ID})
	  })
	   v1.GET("/ops/admin/policy", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   c.JSON(200, gin.H{"packs": hub.Packs.Names()})
	   })
	  v1.GET("/ops/admin/usage", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   user := c.Query("user_id")
		  asset := c.Query("asset")
		   if user == "" {
			  if u, err := callerid.Bind(c, user, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile)); err != nil {
				   mw.WriteProblem(c, err)
				   return
			   } else {
				  user = u
			   }
		  }
		  if asset == "" {
			  asset = "USDT"
		  }
		   c.JSON(200, gin.H{
			  "user_id": user, "asset": asset,
			  "today_withdraw": hub.Use.TodaySum(usage.KindWithdraw, user, asset).String(),
			   "velocity_1m":    hub.Use.Velocity(usage.KindWithdraw, user, asset, 0),
		   })
	   })
	   v1.GET("/ops/admin/inventory", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  rows, err := hub.Inv.Observe(c.Request.Context(), rt.Store)
		  if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		  }
		   c.JSON(200, gin.H{"inventory": rows, "plans": hub.Sweep.Evaluate(rows)})
	  })
	  v1.POST("/ops/admin/sweep-eval", func(c *gin.Context) {
		  actor, ok := requireOps(c, el)
		  if !ok {
			  return
		   }
		  if err := haltGateOf(rt).CheckActive(c.Request.Context()); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error(), "sweep_blocked": true})
			  return
		   }
		   rows, err := hub.Inv.Observe(c.Request.Context(), rt.Store)
		   if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		   }
		  plans := hub.Sweep.Evaluate(rows)
		   metrics.OpsAdmin.WithLabelValues("sweep_eval").Inc()
		  c.JSON(200, gin.H{"plans": plans, "actor": actor.ID})
	   })
	  v1.GET("/ops/admin/region", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  c.JSON(200, hub.Pair.Status())
	   })
	   v1.POST("/ops/admin/failover", func(c *gin.Context) {
		  actor, ok := requireOps(c, el)
		  if !ok {
			   return
		   }
		  var body struct {
			   Reason string `json:"reason"`
		  }
		  _ = c.ShouldBindJSON(&body)
		   if _, err := hub.Pair.Campaign(c.Request.Context()); err != nil && !errors.Is(err, ha.ErrNotLeader) {
			   c.JSON(status(err), gin.H{"error": err.Error()})
			   return
		  }
		   rec, err := hub.Pair.Failover(c.Request.Context(), body.Reason)
		  if err != nil {
			   c.JSON(status(err), gin.H{"error": err.Error(), "region": hub.Pair.Status()})
			  return
		  }
		   metrics.OpsAdmin.WithLabelValues("failover").Inc()
		  c.JSON(200, gin.H{"failover": rec, "region": hub.Pair.Status(), "actor": actor.ID})
	  })
	  v1.GET("/ops/admin/sign-jobs", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  c.JSON(200, gin.H{"jobs": hub.Sign.List(50)})
	   })
	  v1.GET("/ops/admin/allowlist", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   user := c.Query("user_id")
		  if user == "" {
			   if u, err := callerid.Bind(c, user, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile)); err != nil {
				  mw.WriteProblem(c, err)
				  return
			  } else {
				   user = u
			  }
		   }
		   c.JSON(200, gin.H{"entries": hub.Allow.List(user)})
	  })
	   v1.POST("/ops/admin/allowlist", func(c *gin.Context) {
		   actor, ok := requireOps(c, el)
		  if !ok {
			   return
		   }
		   var body struct {
			  UserID string `json:"user_id"`
			   Chain   string `json:"chain"`
			  Address string `json:"address"`
			   Label   string `json:"label"`
		   }
		   if err := c.ShouldBindJSON(&body); err != nil {
			  c.JSON(400, gin.H{"error": err.Error()})
			  return
		   }
		  uid, err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   body.UserID = uid
		   e, err := hub.Allow.Add(body.UserID, body.Chain, body.Address, body.Label)
		   if err != nil {
			  c.JSON(400, gin.H{"error": err.Error()})
			   return
		  }
		   metrics.OpsAdmin.WithLabelValues("allowlist").Inc()
		   c.JSON(200, gin.H{"entry": e, "actor": actor.ID})
	  })
	  v1.GET("/ops/admin/runbook", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   c.JSON(200, gin.H{"steps": DefaultRunbook(), "label": "ops failover runbook"})
	   })
	   v1.POST("/ops/admin/runbook/:id/ack", func(c *gin.Context) {
		   actor, ok := requireOps(c, el)
		  if !ok {
			  return
		   }
		  step, found := RunbookByID(c.Param("id"))
		  if !found {
			  c.JSON(404, gin.H{"error": "runbook step not found"})
			  return
		  }
		  metrics.OpsAdmin.WithLabelValues("runbook_ack").Inc()
		   c.JSON(200, gin.H{"step": step, "actor": actor.ID, "acked": true})
	   })
	  registerExtra(v1, rt, el, hub)
	   registerIncident(v1, rt, el)
	   registerReconRead(v1, rt, el)
	  registerReconWrite(v1, rt, el)
	  registerHalt(v1, rt, el)
	   registerHaltStatus(v1, rt, el)
  }

  func registerExtra(v1 *gin.RouterGroup, rt *app.Runtime, el *ha.Elector, hub *Hub) {
	  _ = rt
	  audit := auditlog.New()
	  _ = hub.Pipe.Policy().Publish(policy.DefaultV2())
	  v1.GET("/ops/admin/audit", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  after, _ := strconv.ParseInt(c.Query("after"), 10, 64)
		  kind := c.Query("kind")
		  limit := validate.Limit(0,
			   50, 200)
		  if raw := c.Query("limit"); raw != "" {
			  n, _ := strconv.Atoi(raw)
			   limit = validate.Limit(n, 50, 200)
		   }
		  c.JSON(200,
			  audit.PageAfter(after, kind, limit))
	  })
	  v1.POST("/ops/admin/audit", func(c *gin.Context) {
		   actor, ok := requireOps(c, el)
		  if !ok {
			   return
		  }
		  var body struct {
			  Kind string `json:"kind"`
			  Subject string `json:"subject"`
			  Body    string `json:"body"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		   e, err := audit.Append(c.Request.Context(), auditlog.Entry{
			   Kind: body.Kind, Actor: actor.ID, Subject: body.Subject, Body: body.Body,
		   })
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, e)
	   })
	   inv := investigate.New(rt.Ledger)
	  v1.GET("/ops/admin/investigate", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  rawUID, err := callerid.Bind(c, c.Query("user_id"), callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  uid, err := validate.UserID(rawUID)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   look, err := inv.Inspect(c.Request.Context(), uid, c.Query("asset"))
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, look)
	  })
	   v1.GET("/ops/admin/fence", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   c.JSON(200, gin.H{
			   "ha": el.Status(),
			   "region": hub.Pair.Status(),
			   "split":  hub.Pair.SplitBrain(),
			  "label": "dual-region write fence",
		  })
	  })
	  mountFundDesk(v1, rt)
   }
