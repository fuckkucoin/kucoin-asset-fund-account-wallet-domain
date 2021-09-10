package oncehaltgate

import (
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/outboxonce"
	"account-assets.kucoin.net/funding/internal/perm"
)

// Register 注册路由到 v1 路由组。

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  svc := outboxonce.New()
	   gate := NewGate(svc)
	   workers := NewWorkers(gate,
		  500*time.Millisecond, 2*time.Second)
	   _ = rt
	  _ = workers
	   // ── Halt 切换 ─────────────────────────────────────────────────────────────
	  v1.POST("/oncehaltgate/switch", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var req SwitchRequest
		  if !mw.BindJSON(c, &req) {
			  return
		  }
		  result, err := gate.Switch(c.Request.Context(), req)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, result)
	   })
	  // ── 投递 ──────────────────────────────────────────────────────────────────
	  v1.POST("/oncehaltgate/deliver", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var req DeliverRequest
		  if !mw.BindJSON(c, &req) {
			  return
		   }
		  resp, err := gate.Deliver(c.Request.Context(), req)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, resp)
	   })
	  // ── 状态查询 ──────────────────────────────────────────────────────────────
	  v1.GET("/oncehaltgate/status", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   c.JSON(200, gate.Status())
	  })
	  v1.GET("/oncehaltgate/halt-gen", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   c.JSON(200, gate.HaltGenView())
	  })
	   // ── 证明导出 ──────────────────────────────────────────────────────────────
	  v1.GET("/oncehaltgate/proof", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var q ProofQuery
		   _ = c.ShouldBindQuery(&q)
		  c.JSON(200, gate.ExportProof(q))
	  })
	  // ── 工人状态 ──────────────────────────────────────────────────────────────
	  v1.GET("/oncehaltgate/workers", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   c.JSON(200, workers.Status())
	   })
   }
