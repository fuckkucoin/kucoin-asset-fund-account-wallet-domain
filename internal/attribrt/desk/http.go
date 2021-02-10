package desk

import (
	"net/http"
	"time"
	"account-assets.kucoin.net/funding/internal/attribrt"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// Register 挂归因台席。eng 为空时不注册，避免空指针写成放行。
// 读要求 case.read，写要求 ops.admin。普通用户的 deposit.observe 不能改碰撞索引。

   func Register(v1 *gin.RouterGroup, eng *attribrt.Engine) {
	   if v1 == nil || eng == nil {
		   return
	   }
	   g := v1.Group("/attrib", requireAttribDesk)
	   g.POST("/resolve", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body attribrt.Sight
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  d, err := eng.Resolve(body)
		   if err != nil {
			  c.JSON(409, gin.H{"error": err.Error(), "decision": d})
			   return
		  }
		   c.JSON(200, d)
	   })
	  g.GET("/decision/:id", func(c *gin.Context) {
		  d, ok := eng.Get(c.Param("id"))
		   if !ok {
			   c.JSON(404, gin.H{"error": "decision not found"})
			  return
		   }
		  c.JSON(200, d)
	  })
	   g.GET("/by-tx", func(c *gin.Context) {
		   d, ok := eng.Find(c.Query("chain"), c.Query("tx_hash"), atoi(c.Query("output_index")))
		  if !ok {
			   c.JSON(404, gin.H{"error": "decision not found"})
			   return
		   }
		  c.JSON(200, d)
	   })
	  g.GET("/envelopes", func(c *gin.Context) {
		   c.JSON(200, gin.H{"envelopes": eng.Envelopes(atoiDefault(c.Query("n"), 50))})
	  })
	   g.GET("/queue", func(c *gin.Context) {
		   c.JSON(200, gin.H{"queue": eng.Queue()})
	   })
	  g.POST("/queue/:id/close", func(c *gin.Context) {
		  actor := c.GetHeader("X-Actor")
		  if err := perm.FromRequest(c.Request, actor).Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  if !eng.CloseQueue(c.Param("id"), actor) {
			   c.JSON(409, gin.H{"error": "queue item not open"})
			  return
		  }
		   c.JSON(200, gin.H{"closed": true})
	  })
	  g.GET("/policy", func(c *gin.Context) {
		   tab := eng.Policy()
		   if tab == nil {
			  c.JSON(503, gin.H{"error": "policy unavailable"})
			  return
		  }
		  ch := c.Query("chain")
		  if ch == "" {
			   c.JSON(200, gin.H{"chains": tab.Chains(), "versions": tab.Versions()})
			  return
		   }
		   row, ok := tab.Row(ch)
		  ev := tab.Evaluate(ch, c.Query("version"), c.Query("replay") == "1")
		   c.JSON(200, gin.H{"found": ok, "row": row, "eval": ev})
	  })
	  g.GET("/collide", func(c *gin.Context) {
		  c.JSON(200, gin.H{"cases": eng.Collisions().List()})
	  })
	   g.GET("/collide/:id", func(c *gin.Context) {
		   cs, ok := eng.Collisions().Get(c.Param("id"))
		   if !ok {
			   c.JSON(404, gin.H{"error": "collision not found"})
			   return
		   }
		  c.JSON(200, cs)
	   })
	  g.POST("/collide/:id/replay", func(c *gin.Context) {
		   cs, err := eng.Collisions().Replay(c.Param("id"))
		   if err != nil {
			   c.JSON(409, gin.H{"error": err.Error()})
			   return
		  }
		   c.JSON(200, cs)
	   })
	   postCase(g, eng, "/collide/:id/assign", func(id, actor, reason, winner, evidence string) (any, error) {
		   return eng.Assign(id, actor, reason)
	  })
	  postCase(g, eng, "/collide/:id/prove", func(id, actor, reason, winner, evidence string) (any, error) {
		   return eng.Prove(id, actor, evidence)
	   })
	   postCase(g, eng, "/collide/:id/accept", func(id, actor, reason, winner, evidence string) (any, error) {
		  return eng.AcceptProof(id, actor)
	   })
	   postCase(g, eng, "/collide/:id/reject-proof", func(id, actor, reason, winner, evidence string) (any, error) {
		  return eng.RejectProof(id, actor, reason)
	  })
	  postCase(g, eng, "/collide/:id/pick", func(id, actor, reason, winner, evidence string) (any, error) {
		   return eng.Pick(id, actor, winner, reason)
	  })
	   postCase(g, eng, "/collide/:id/pick-none", func(id, actor, reason, winner, evidence string) (any, error) {
		  return eng.PickNone(id, actor, reason)
	   })
	   g.POST("/book", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			  Kind  string   `json:"kind"`
			   Chain string   `json:"chain"`
			  Addrs []string `json:"addrs"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		   switch body.Kind {
		   case "hot":
			  eng.AddHot(body.Addrs...)
		  case "deny":
			  eng.AddDeny(body.Addrs...)
		   case "look":
			   eng.AddLook(body.Addrs...)
		  case "contract":
			   for _, a := range body.Addrs {
				   eng.AddContract(body.Chain, a)
			  }
		  default:
			  c.JSON(400, gin.H{"error": "book kind"})
			  return
		   }
		  hot, deny, look := eng.Books()
		  c.JSON(200, gin.H{"hot": hot, "deny": deny, "look": look})
	   })
	  g.POST("/tick", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  exp, rerun := eng.Tick(time.Time{})
		  c.JSON(200, gin.H{"expired": exp, "rerun": rerun})
	  })
	  g.POST("/replay/:name", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  d, err := attribrt.ReplayScenario(c.Param("name"))
		  if err != nil {
			  c.JSON(409, gin.H{"error": err.Error(), "decision": d})
			  return
		   }
		  c.JSON(200, d)
	  })
	   g.GET("/fixtures", func(c *gin.Context) {
		   c.JSON(200, gin.H{"fixtures": attribrt.Fixtures()})
	  })
  }

   func requireAttribDesk(c *gin.Context) {
	   act := perm.ActOpsAdmin
	   if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead {
		   act = perm.ActCaseRead
	   }
	   if err := perm.FromRequest(c.Request, c.GetHeader("X-Actor")).Check(act); err != nil {
		  c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return
	   }
	  c.Next()
   }

  func postCase(v1 *gin.RouterGroup, eng *attribrt.Engine, path string, fn func(id, actor, reason, winner, evidence string) (any, error)) {
	   v1.POST(path, func(c *gin.Context) {
		  actor := c.GetHeader("X-Actor")
		  if err := perm.FromRequest(c.Request, actor).Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   var body struct {
			  Reason string `json:"reason"`
			  Winner   string `json:"winner"`
			   Evidence string `json:"evidence"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  out, err := fn(c.Param("id"), actor, body.Reason, body.Winner, body.Evidence)
		  if err != nil {
			  c.JSON(409, gin.H{"error": err.Error(), "case": out})
			  return
		   }
		  _ = eng
		  c.JSON(200, out)
	   })
}

   func atoi(s string) int {
	   n := 0
	   for _, r := range s {
		   if r < '0' || r > '9' {
			  return 0
		   }
		   n = n*10 + int(r-'0')
	   }
	   return n
   }

  func atoiDefault(s string, def int) int {
	   if s == "" {
		   return def
	   }
	   n := atoi(s)
	  if n <= 0 {
		  return def
	   }
	  return n
   }
