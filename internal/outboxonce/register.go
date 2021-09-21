package outboxonce

import (
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
)

// Register 注册所有路由到 v1 路由组。

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  svc := New()
	   _ = rt
	  // ── 原有端点 ──────────────────────────────────────────────────────────────
	   v1.POST("/outboxonce/enqueue", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			   Key string `json:"key"`
			  Kind string `json:"kind"`
			  Payload string `json:"payload"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  ev, err := svc.Enqueue(c.Request.Context(), body.Key, body.Kind, body.Payload)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, ev)
	  })
	  v1.POST("/outboxonce/deliver", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			  Key string `json:"key"`
			  Epoch int64 `json:"epoch"`
			  Token string `json:"token"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		  got, err := svc.Deliver(c.Request.Context(), body.Key, body.Epoch, body.Token)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, got)
	   })
	  v1.GET("/outboxonce/pending", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   c.JSON(200, gin.H{
			   "pending": svc.Pending(),
			   "host": "ledger-transfer-freeze-reconciliation.kucoin.net",
		   })
	   })
	  v1.POST("/outboxonce/halt", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			   On bool `json:"on"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   lease, revoked := svc.SetHaltByActor(body.On, "")
		  c.JSON(200, gin.H{
			  "halt":    svc.Halted(),
			   "lease":   lease,
			   "revoked": len(revoked),
		  })
	  })
	   // ── halt_gen 扩展端点 ─────────────────────────────────────────────────────
	   // POST /outboxonce/halt-actor — 带操作者标识的停机切换（审计用）。
	   v1.POST("/outboxonce/halt-actor", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body struct {
			   On    bool   `json:"on"`
			   Actor string `json:"actor"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   lease, revoked := svc.SetHaltByActor(body.On, body.Actor)
		  c.JSON(200, gin.H{
			  "halt": svc.Halted(),
			  "lease": lease,
			  "revoked": len(revoked),
		  })
	  })
	   // GET /outboxonce/halt-gen — 停机世代只读视图。
	   v1.GET("/outboxonce/halt-gen", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  c.JSON(200, svc.HaltGenView())
	   })
	  // GET /outboxonce/halt-lease — 当前停机世代租约。
	  v1.GET("/outboxonce/halt-lease", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   lease := svc.CurrentLease()
		  if lease == nil {
			  c.JSON(404, gin.H{"error": ErrLeaseNotFound.Error()})
			   return
		  }
		  c.JSON(200, lease)
	  })
	  // GET /outboxonce/cases — 双投/漏投证明列表。
	   v1.GET("/outboxonce/cases", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  cases := svc.Cases()
		   c.JSON(200, gin.H{
			   "cases": cases,
			   "total": len(cases),
		  })
	   })
	  // GET /outboxonce/proof — 证明导出摘要。
	   v1.GET("/outboxonce/proof", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   c.JSON(200, svc.ExportProof())
	  })
   }
