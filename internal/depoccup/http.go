// http.go
// 用户只读自己的批次；失败和重组仍走已有回滚端口。
// 不变量：读接口不改桶；写接口返回落账后的批次。
// CreateDate: 2021.08.15

// Author: Cason
// Date: 2021.04.01

package depoccup

import (
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

   func idOpt(rt *app.Runtime) callerid.Opt {
	   if rt == nil || rt.Cfg == nil {
		   return callerid.Opt{}
	   }
	  return callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile)
  }

  func deny(c *gin.Context, act perm.Action) bool {
	  if err := perm.FromRequest(c.Request, "").Check(act); err != nil {
		   c.JSON(403, gin.H{"error": err.Error()})
		   return true
	   }
	   return false
  }

   func writeErr(c *gin.Context, err error) {
	   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
  }

  func Routes(v1 *gin.RouterGroup, rt *app.Runtime, s *Service) {
	   if s == nil {
		  s = New()
		  s.UseDefaults()
	   }
	  v1.GET("/deposits/occupancy", func(c *gin.Context) {
		   uid, err := callerid.Bind(c, c.Query("user_id"), idOpt(rt))
		  if err != nil {
			  writeErr(c, err)
			  return
		   }
		   view, err := s.User(uid)
		  if err != nil {
			   writeErr(c, err)
			   return
		   }
		  c.JSON(200, view)
	  })
	   v1.GET("/deposits/occupancy/:id", func(c *gin.Context) {
		  uid, err := callerid.Bind(c, "", idOpt(rt))
		  if err != nil {
			  writeErr(c, err)
			  return
		  }
		   lot, err := s.Lot(c.Param("id"))
		  if err != nil {
			   writeErr(c, err)
			   return
		  }
		   if lot.UserID != uid {
			   c.JSON(403, gin.H{"error": "occupancy lot belongs to another user"})
			   return
		  }
		   c.JSON(200, gin.H{"lot": lot, "release_conditions": s.Conditions(lot, s.now())})
	   })
	  ops := v1.Group("/ops/dep-occupancy")
	   ops.GET("/health", func(c *gin.Context) {
		  if deny(c, perm.ActQueryRead) {
			  return
		   }
		   c.JSON(200, s.Health())
	   })
	   ops.GET("/users/:user", func(c *gin.Context) {
		  if deny(c, perm.ActCaseRead) {
			  return
		   }
		  view, err := s.CS(c.Param("user"))
		  if err != nil {
			   writeErr(c, err)
			   return
		   }
		   c.JSON(200, view)
	  })
	  ops.GET("/lots/:id", func(c *gin.Context) {
		   if deny(c, perm.ActCaseRead) {
			  return
		   }
		   lot, err := s.Lot(c.Param("id"))
		  if err != nil {
			  writeErr(c, err)
			  return
		  }
		   c.JSON(200, gin.H{"lot": lot, "release_conditions": s.Conditions(lot, s.now())})
	  })
	   ops.GET("/policies", func(c *gin.Context) {
		   if deny(c, perm.ActCaseRead) {
			  return
		   }
		   c.JSON(200, gin.H{"policies": s.ListPolicies(c.Query("kyc"), c.Query("asset"), c.Query("source"), c.Query("chain"))})
	   })
	  ops.POST("/policies", func(c *gin.Context) {
		  if deny(c, perm.ActOpsAdmin) {
			   return
		  }
		   var body PolicyIn
		   if err := c.ShouldBindJSON(&body); err != nil {
			  c.JSON(400, gin.H{"error": err.Error()})
			   return
		  }
		  if body.Actor == "" {
			  body.Actor = "ops"
		   }
		   got, err := s.UpsertPolicy(body)
		   if err != nil {
			   writeErr(c, err)
			  return
		   }
		  c.JSON(200, gin.H{"policy": got})
	   })
	   ops.POST("/signals", func(c *gin.Context) {
		  if deny(c, perm.ActCaseRead) {
			  return
		   }
		  var body SignalIn
		   if err := c.ShouldBindJSON(&body); err != nil {
			   c.JSON(400, gin.H{"error": err.Error()})
			   return
		   }
		  if body.Actor == "" {
			   body.Actor = "cs"
		   }
		  st, err := s.Ingest(body)
		   if err != nil {
			  writeErr(c, err)
			   return
		   }
		   c.JSON(200, gin.H{"tier": st})
	   })
	   ops.POST("/observe", func(c *gin.Context) {
		  if deny(c, perm.ActOpsAdmin) {
			   return
		   }
		  var body ObserveIn
		  if err := c.ShouldBindJSON(&body); err != nil {
			  c.JSON(400, gin.H{"error": err.Error()})
			   return
		   }
		  if body.Actor == "" {
			   body.Actor = "ops"
		  }
		   lot,
			  err := s.Observe(c.Request.Context(), body)
		  if err != nil {
			   writeErr(c, err)
			  return
		   }
		   c.JSON(200, gin.H{"lot": lot, "release_conditions": s.Conditions(lot, s.now())})
	   })
	   ops.POST("/confirm", func(c *gin.Context) {
		   if deny(c, perm.ActOpsAdmin) {
			   return
		   }
		  var body ConfirmIn
		   if err := c.ShouldBindJSON(&body); err != nil {
			   c.JSON(400, gin.H{"error": err.Error()})
			  return
		   }
		  lot, err := s.Confirm(c.Request.Context(), body)
		  if err != nil {
			  writeErr(c, err)
			   return
		  }
		  c.JSON(200, gin.H{"lot": lot, "release_conditions": s.Conditions(lot, s.now())})
	   })
	  ops.POST("/fail", func(c *gin.Context) {
		  if deny(c, perm.ActOpsAdmin) {
			   return
		   }
		   var body FailIn
		   if err := c.ShouldBindJSON(&body); err != nil {
			  c.JSON(400, gin.H{"error": err.Error()})
			   return
		   }
		  lot, err := s.Fail(c.Request.Context(), body)
		   if err != nil {
			   writeErr(c, err)
			   return
		  }
		   c.JSON(200, gin.H{"lot": lot})
	  })
	  ops.POST("/reorg", func(c *gin.Context) {
		  if deny(c, perm.ActOpsAdmin) {
			  return
		  }
		   var body FailIn
		   if err := c.ShouldBindJSON(&body); err != nil {
			   c.JSON(400, gin.H{"error": err.Error()})
			  return
		   }
		   lot, err := s.Reorg(c.Request.Context(), body)
		  if err != nil {
			  writeErr(c, err)
			  return
		   }
		   c.JSON(200, gin.H{"lot": lot})
	  })
	  ops.POST("/reclass", func(c *gin.Context) {
		   if deny(c, perm.ActCaseRead) {
			  return
		  }
		   var body struct {
			   TransferID string `json:"transfer_id"`
			   Source     string `json:"source"`
			   Actor string `json:"actor"`
		   }
		   if err := c.ShouldBindJSON(&body); err != nil {
			   c.JSON(400, gin.H{"error": err.Error()})
			  return
		  }
		   if body.Actor == "" {
			  body.Actor = "cs"
		  }
		  lot, err := s.Reclass(c.Request.Context(), body.TransferID, body.Source, body.Actor)
		   if err != nil {
			  writeErr(c, err)
			  return
		  }
		  c.JSON(200, gin.H{"lot": lot, "release_conditions": s.Conditions(lot, s.now())})
	  })
	  ops.POST("/refresh", func(c *gin.Context) {
		  if deny(c, perm.ActCaseRead) {
			   return
		  }
		   var body struct {
			   UserID string `json:"user_id"`
			   Actor string `json:"actor"`
		  }
		   if err := c.ShouldBindJSON(&body); err != nil {
			  c.JSON(400, gin.H{"error": err.Error()})
			   return
		  }
		   if body.Actor == "" {
			  body.Actor = "cs"
		   }
		  lots, err := s.RefreshUser(c.Request.Context(), body.UserID, body.Actor)
		  if err != nil {
			   writeErr(c, err)
			   return
		  }
		  c.JSON(200, gin.H{"lots": lots})
	  })
	  ops.POST("/sources", func(c *gin.Context) {
		   if deny(c, perm.ActOpsAdmin) {
			  return
		  }
		   var body struct {
			   Address string `json:"address"`
			   Source string `json:"source"`
		   }
		   if err := c.ShouldBindJSON(&body); err != nil {
			  c.JSON(400, gin.H{"error": err.Error()})
			  return
		   }
		   src, err := ParseSource(body.Source)
		  if err != nil {
			   writeErr(c, err)
			  return
		   }
		   if err := s.Sources().PutAddress(body.Address, src); err != nil {
			  writeErr(c, err)
			  return
		  }
		   c.JSON(200, gin.H{"address": body.Address, "source": src})
	  })
	  ops.GET("/explain", func(c *gin.Context) {
		  if deny(c, perm.ActCaseRead) {
			  return
		  }
		   hit, err := s.Explain(Query{
			   KYC: c.Query("kyc"), Asset: c.Query("asset"), Source: Source(c.Query("source")), Chain: c.Query("chain"),
		  }, Tier(c.Query("tier")))
		   if err != nil {
			   writeErr(c, err)
			  return
		  }
		   c.JSON(200, hit)
	   })
	   ops.GET("/audits", func(c *gin.Context) {
		   if deny(c, perm.ActCaseRead) {
			   return
		   }
		   c.JSON(200, gin.H{"audits": s.Audits(c.Query("lot_id"), 50)})
	  })
	  ops.POST("/tick", func(c *gin.Context) {
		  if deny(c, perm.ActOpsAdmin) {
			  return
		  }
		  n := s.Tick(c.Request.Context())
		  c.JSON(200, gin.H{"breaches": n})
	   })
   }
