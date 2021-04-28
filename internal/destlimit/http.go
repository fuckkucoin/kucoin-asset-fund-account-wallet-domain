// 功能：目的地风险图谱与动态限额的运营接口。
// 写策略、连边、确认覆写与手动节拍要求运营管理员；占用查询对只读角色开放。
// 超限与双人失败走领域错误，由 apierr 映射状态码，不在这里拼文案码。

// 作者：Likeo
// 日期：2021.04.11

package destlimit

import (
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  eng := Attach(rt)
	  ops := func(c *gin.Context) bool {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return false
		   }
		  return true
	   }
	  read := func(c *gin.Context) bool {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return false
		   }
		   return true
	   }
	   v1.POST("/destlimit/graph/nodes", func(c *gin.Context) {
		  if !ops(c) {
			  return
		   }
		   var body struct {
			   Chain string `json:"chain"`
			   Address string `json:"address"`
			   Label   string `json:"label"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		  got, err := eng.Graph().UpsertNode(body.Chain, body.Address, body.Label)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, got)
	   })
	   v1.POST("/destlimit/graph/edges", func(c *gin.Context) {
		  if !ops(c) {
			   return
		   }
		   var body struct {
			   ChainA   string `json:"chain_a"`
			   AddressA string `json:"address_a"`
			   ChainB   string `json:"chain_b"`
			   AddressB string `json:"address_b"`
			   Weight string `json:"weight"`
			   Kind   string `json:"kind"`
			  Evidence string `json:"evidence"`
			  HalfLifeSec int64 `json:"half_life_sec"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		   half := time.Duration(body.HalfLifeSec) * time.Second
		   got, err := eng.Graph().Link(body.ChainA, body.AddressA, body.ChainB, body.AddressB, body.Weight, body.Kind, body.Evidence, half)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, got)
	  })
	   v1.POST("/destlimit/graph/kyt", func(c *gin.Context) {
		   if !ops(c) {
			  return
		   }
		  var body struct {
			  Chain   string `json:"chain"`
			   Address string `json:"address"`
			  Hint string `json:"hint"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  got, err := eng.Graph().NoteKYT(body.Chain, body.Address, body.Hint)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, got)
	  })
	   v1.GET("/destlimit/graph/node", func(c *gin.Context) {
		  if !read(c) {
			  return
		   }
		  got, ok := eng.Graph().Node(c.Query("chain"), c.Query("address"))
		   if !ok {
			   mw.WriteProblem(c, ErrNotFound)
			  return
		  }
		  c.JSON(200, got)
	   })
	   v1.GET("/destlimit/graph/edges", func(c *gin.Context) {
		  if !read(c) {
			  return
		  }
		  c.JSON(200, gin.H{"edges": eng.Graph().EdgesOf(c.Query("chain"), c.Query("address"))})
	   })
	   v1.GET("/destlimit/graph/cluster", func(c *gin.Context) {
		   if !read(c) {
			   return
		  }
		  got, ok := eng.Graph().Cluster(c.Query("id"), c.Query("asset"))
		  if !ok {
			  mw.WriteProblem(c, ErrNotFound)
			  return
		   }
		  c.JSON(200, got)
	   })
	  v1.GET("/destlimit/graph/audits", func(c *gin.Context) {
		   if !read(c) {
			   return
		   }
		   c.JSON(200, gin.H{"audits": eng.Graph().Audits(c.Query("kind"), 50)})
	  })
	  v1.POST("/destlimit/policies", func(c *gin.Context) {
		  if !ops(c) {
			  return
		   }
		   var body Policy
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  got, err := eng.PutPolicy(body)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, got)
	   })
	  v1.GET("/destlimit/policies", func(c *gin.Context) {
		   if !read(c) {
			  return
		   }
		   c.JSON(200, gin.H{"policies": eng.ListPolicies(), "generation": eng.Generation()})
	  })
	   v1.POST("/destlimit/policies/:id/disable", func(c *gin.Context) {
		   if !ops(c) {
			  return
		  }
		  actor := perm.FromRequest(c.Request, "")
		   got, err := eng.DisablePolicy(c.Param("id"), actor.ID)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, got)
	   })
	   v1.GET("/destlimit/holds", func(c *gin.Context) {
		  if !read(c) {
			  return
		   }
		  c.JSON(200, gin.H{"holds": eng.Holds(c.Query("user_id"), c.Query("asset"), 50)})
	  })
	   v1.GET("/destlimit/holds/:id", func(c *gin.Context) {
		  if !read(c) {
			   return
		   }
		  got, err := eng.Hold(c.Param("id"))
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, got)
	  })
	  v1.POST("/destlimit/holds/:id/consume", func(c *gin.Context) {
		   if !ops(c) {
			   return
		   }
		  var body struct {
			  Reason string `json:"reason"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   got, err := eng.Consume(c.Param("id"), body.Reason)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, got)
	   })
	  v1.POST("/destlimit/holds/:id/release", func(c *gin.Context) {
		   if !ops(c) {
			  return
		   }
		  var body struct {
			   Reason string `json:"reason"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  got, err := eng.Release(c.Param("id"), body.Reason)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, got)
	   })
	  v1.POST("/destlimit/overrides", func(c *gin.Context) {
		   if !ops(c) {
			   return
		   }
		  actor := perm.FromRequest(c.Request, "")
		   var body struct {
			   UserID    string `json:"user_id"`
			   Asset     string `json:"asset"`
			  ClusterID string `json:"cluster_id"`
			   Extra     string `json:"extra"`
			   Reason string `json:"reason"`
			  TTLSec int64 `json:"ttl_sec"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		  got, err := eng.ProposeOverride(body.UserID, body.Asset, body.ClusterID, body.Extra, actor.ID, body.Reason, time.Duration(body.TTLSec)*time.Second)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, got)
	  })
	  v1.POST("/destlimit/overrides/:id/confirm", func(c *gin.Context) {
		  if !ops(c) {
			   return
		   }
		  actor := perm.FromRequest(c.Request, "")
		  got, err := eng.ConfirmOverride(c.Param("id"), actor.ID)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, got)
	   })
	   v1.GET("/destlimit/overrides", func(c *gin.Context) {
		  if !read(c) {
			  return
		   }
		   c.JSON(200, gin.H{"overrides": eng.ListOverrides(c.Query("user_id"), c.Query("asset"))})
	   })
	  v1.POST("/destlimit/worker/tick", func(c *gin.Context) {
		  if !ops(c) {
			   return
		  }
		   got, err := eng.Tick(time.Time{})
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, got)
	  })
	  v1.GET("/destlimit/signals", func(c *gin.Context) {
		  if !read(c) {
			   return
		   }
		   c.JSON(200, gin.H{"signals": eng.OverLimitSignals(50)})
	  })
	  v1.GET("/destlimit/audits", func(c *gin.Context) {
		   if !read(c) {
			  return
		   }
		   c.JSON(200, gin.H{"audits": eng.Audits(c.Query("kind"), 50)})
	  })
  }
