package solvency

import (
	"os"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// SecretFrom 只认 SOLVENCY_HMAC_SECRET。内存剖面使用带种子的派生值，源码不写密钥。

   func SecretFrom(rt *app.Runtime) string {
	   if v := strings.TrimSpace(os.Getenv("SOLVENCY_HMAC_SECRET")); v != "" {
		  return v
	  }
	   if rt != nil && rt.Mem {
		   seed := "mem"
		  if rt.Cfg != nil && strings.TrimSpace(rt.Cfg.SeedUserID) != "" {
			  seed = rt.Cfg.SeedUserID
		  }
		   return "mem-solvency:" + seed
	  }
	   return ""
   }

// FromRuntime 用账本只读口与可选 SQL 仓储装配管线。观察覆盖只在内存剖面打开。

  func FromRuntime(rt *app.Runtime) *Pipeline {
	  opt := Options{}
	  if rt != nil {
		   opt.Books = rt.Store
		  opt.Bus = rt.Bus
		  opt.AllowOverride = rt.Mem
		  opt.Secret = SecretFrom(rt)
		   if rt.SQL != nil && rt.SQL.DB != nil {
			  opt.Store = NewSQLStore(rt.SQL.DB, rt.SQL.Dialect)
		   }
	  }
	  return New(opt)
   }

// Register 挂查询与运维路由，返回管线供进程内泵使用。

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) *Pipeline {
	   p := FromRuntime(rt)
	  mount(v1, p, rt)
	   return p
   }

  func mount(v1 *gin.RouterGroup, p *Pipeline, rt *app.Runtime) {
	  opt := callerid.Opt{}
	  if rt != nil && rt.Cfg != nil {
		  opt = callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile)
	  }
	   v1.GET("/solvency/attestations", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   list, err := p.List(c.Request.Context(), c.Query("schedule_id"), 100)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   if list == nil {
			   list = []Attestation{}
		  }
		   c.JSON(200, gin.H{"attestations": list})
	   })
	  v1.GET("/solvency/attestations/:id", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  att, err := p.Get(c.Request.Context(), c.Param("id"))
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   snaps, _ := p.store.ListReserve(c.Request.Context(), att.ID)
		  drifts, _ := p.store.ListDrifts(c.Request.Context(), "", 200)
		   mine := make([]DriftCase, 0)
		   for _, d := range drifts {
			   if d.AttestationID == att.ID {
				   mine = append(mine, d)
			   }
		  }
		   c.JSON(200, gin.H{
			   "attestation": att, "reserves": snaps, "drifts": mine,
			  "causal": att.CausalRef(), "solvency": att.Solvency, "deprecated": att.Deprecated,
		  })
	  })
	  v1.POST("/solvency/attestations/:id/proof", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   var body struct {
			  UserID string `json:"user_id"`
			  Book   string `json:"book"`
			   Asset string `json:"asset"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		   uid, err := callerid.Bind(c, body.UserID, opt)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  pr, att, err := p.Prove(c.Request.Context(), c.Param("id"), uid, body.Book, body.Asset)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, gin.H{
			  "attestation_id": att.ID, "status": att.Status, "class": att.Class,
			  "solvency": att.Solvency, "deprecated": att.Deprecated,
			   "root": att.Root, "proof": pr, "verified": true,
			   "read_only": att.Status == StatusPublished,
		  })
	   })
	  v1.POST("/solvency/verify", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			   Root  string `json:"root"`
			   Proof Proof  `json:"proof"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  if err := VerifyProof(body.Root, body.Proof); err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, gin.H{"ok": true, "read_only": true, "solvency": true, "deprecated": false})
	   })
	  v1.GET("/solvency/legacy", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   got := ClassifyDisclaimer(c.Query("disclaimer"))
		   c.JSON(200, got)
	  })
	  v1.POST("/solvency/ops/schedules", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body struct {
			   ID          string   `json:"id"`
			  Name        string   `json:"name"`
			   IntervalSec int64    `json:"interval_sec"`
			   WindowSec   int64    `json:"window_sec"`
			   Assets      []string `json:"assets"`
			   Enabled     *bool `json:"enabled"`
			  AutoPublish bool  `json:"auto_publish"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		  enabled := true
		   if body.Enabled != nil {
			  enabled = *body.Enabled
		   }
		  sch, err := p.PutSchedule(c.Request.Context(), Schedule{
			   ID: body.ID, Name: body.Name,
			  Interval: time.Duration(body.IntervalSec) * time.Second,
			  Window:  time.Duration(body.WindowSec) * time.Second,
			  Assets:  body.Assets,
			   Enabled: enabled, AutoPublish: body.AutoPublish,
		  })
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, sch)
	   })
	  v1.GET("/solvency/ops/schedules", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  list, err := p.store.ListSchedules(c.Request.Context())
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, gin.H{"schedules": list})
	  })
	  v1.POST("/solvency/ops/run", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			  ScheduleID string `json:"schedule_id"`
			   Override []ReserveLeg `json:"reserve_override"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   if body.Override != nil {
			  if err := p.SetReserveOverride(body.Override); err != nil {
				   mw.WriteProblem(c, err)
				   return
			  }
			  defer func() { _ = p.SetReserveOverride(nil) }()
		  }
		  att, err := p.RunSchedule(c.Request.Context(), body.ScheduleID)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, att)
	  })
	   v1.POST("/solvency/ops/tick", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   if err := p.Tick(c.Request.Context()); err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   rep, err := p.Patrol(c.Request.Context())
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  n, err := p.PublishDue(c.Request.Context())
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, gin.H{"patrol": rep, "auto_published": n})
	  })
	   v1.POST("/solvency/ops/publish", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body struct {
			   ID          string `json:"id"`
			  Acknowledge bool   `json:"acknowledge"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		   att, err := p.Publish(c.Request.Context(), body.ID, body.Acknowledge)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, att)
	   })
	  v1.POST("/solvency/ops/revoke", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			  ID     string `json:"id"`
			   Actor  string `json:"actor"`
			  Reason string `json:"reason"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  if body.Actor == "" {
			  body.Actor = perm.FromRequest(c.Request, "").ID
		   }
		  att,
			  err := p.Revoke(c.Request.Context(), body.ID, body.Actor, body.Reason)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, att)
	  })
	   v1.GET("/solvency/drifts", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   list, err := p.ListDrifts(c.Request.Context(), DriftStatus(c.Query("status")), 100)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   if list == nil {
			   list = []DriftCase{}
		   }
		   c.JSON(200, gin.H{"drifts": list})
	  })
	   v1.POST("/solvency/drifts/:id/ack", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			   Actor string `json:"actor"`
			   Note string `json:"note"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   if body.Actor == "" {
			   body.Actor = perm.FromRequest(c.Request, "solvency-desk").ID
		   }
		  d, err := p.AckDrift(c.Request.Context(), c.Param("id"), body.Actor, body.Note)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, d)
	  })
	  v1.POST("/solvency/drifts/:id/close", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			   Actor string `json:"actor"`
			   Note string `json:"note"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  if body.Actor == "" {
			   body.Actor = perm.FromRequest(c.Request, "solvency-desk").ID
		   }
		   d, err := p.CloseDrift(c.Request.Context(), c.Param("id"), body.Actor, body.Note)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, d)
	  })
  }
