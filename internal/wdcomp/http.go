package wdcomp

import (
	"strconv"
	"strings"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// RegisterService 把编排器挂到 /v1/wdcomp。

   func RegisterService(v1 *gin.RouterGroup, svc *Orchestrator) {
	   if svc == nil {
		   svc = New()
	  }
	   g := v1.Group("/wdcomp")
	   g.POST("/failures", func(c *gin.Context) {
		  if !allow(c, perm.ActOpsAdmin) {
			  return
		  }
		  var fact FailureFact
		  if !mw.BindJSON(c, &fact) {
			  return
		  }
		  got, err := svc.Ingest(c.Request.Context(), fact)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, got)
	  })
	  g.GET("/failures", func(c *gin.Context) {
		   if !allow(c, perm.ActQueryRead) {
			  return
		  }
		  c.JSON(200, gin.H{"failures": svc.Failures(filterOf(c))})
	  })
	  g.GET("/failures/:id", func(c *gin.Context) {
		  if !allow(c, perm.ActQueryRead) {
			   return
		   }
		   got, err := svc.Failure(c.Param("id"))
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, got)
	   })
	   g.POST("/plans/preview", func(c *gin.Context) {
		   if !allow(c, perm.ActOpsAdmin) {
			  return
		  }
		  var body struct {
			  FailureID string `json:"failure_id"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		   got, err := svc.Preview(c.Request.Context(), body.FailureID)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, got)
	   })
	   g.GET("/plans", func(c *gin.Context) {
		   if !allow(c, perm.ActQueryRead) {
			   return
		   }
		  c.JSON(200, gin.H{"plans": svc.Plans(filterOf(c))})
	   })
	   g.GET("/plans/:id", func(c *gin.Context) {
		   if !allow(c, perm.ActQueryRead) {
			   return
		  }
		   got, err := svc.Plan(c.Param("id"))
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, got)
	  })
	  g.POST("/plans/:id/execute", func(c *gin.Context) {
		  if !allow(c, perm.ActOpsAdmin) {
			   return
		  }
		   got, err := svc.Execute(c.Request.Context(), c.Param("id"))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, got)
	  })
	   g.POST("/plans/:id/confirm", func(c *gin.Context) {
		  if !allow(c, perm.ActWithdrawDecide) {
			   return
		  }
		   var body struct {
			   Actor    string `json:"actor"`
			  Approver string `json:"approver"`
			   Reason string `json:"reason"`
			  Release bool `json:"release"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		  got, err := svc.Confirm(c.Request.Context(), c.Param("id"), body.Actor, body.Approver, body.Reason, body.Release)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, got)
	   })
	   g.POST("/plans/:id/abort", func(c *gin.Context) {
		  if !allow(c, perm.ActOpsAdmin) {
			   return
		  }
		   var body struct {
			  Actor  string `json:"actor"`
			   Reason string `json:"reason"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  got, err := svc.Abort(c.Request.Context(), c.Param("id"), body.Actor, body.Reason)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, got)
	   })
	  g.GET("/rollback", func(c *gin.Context) {
		  if !allow(c, perm.ActQueryRead) {
			   return
		  }
		  rows := svc.Rollback(c.Query("user_id"), c.Query("asset"), c.Query("transfer_id"))
		   c.JSON(200, gin.H{"entries": rows})
	  })
	   g.GET("/audits", func(c *gin.Context) {
		   if !allow(c, perm.ActQueryRead) {
			   return
		  }
		  c.JSON(200, gin.H{"audits": svc.Audits()})
	   })
	  g.GET("/incidents", func(c *gin.Context) {
		  if !allow(c, perm.ActQueryRead) {
			  return
		   }
		   c.JSON(200, gin.H{"incidents": svc.Incidents()})
	   })
	  g.GET("/positions", func(c *gin.Context) {
		  if !allow(c, perm.ActQueryRead) {
			   return
		   }
		  got, err := svc.Position(c.Query("user_id"), c.Query("asset"))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, got)
	  })
	  g.GET("/policies", func(c *gin.Context) {
		  if !allow(c, perm.ActQueryRead) {
			  return
		  }
		  c.JSON(200, gin.H{"policies": svc.Policies()})
	   })
	  g.PUT("/policies/:code", func(c *gin.Context) {
		   if !allow(c, perm.ActOpsAdmin) {
			   return
		   }
		   var body AutoPolicy
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		   body.Code = FailureCode(c.Param("code"))
		   if body.UpdatedBy == "" {
			   body.UpdatedBy = perm.FromRequest(c.Request, "").ID
		  }
		   got, err := svc.PutPolicy(c.Request.Context(), body)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, got)
	   })
	  g.POST("/workers/tick", func(c *gin.Context) {
		   if !allow(c, perm.ActOpsAdmin) {
			   return
		   }
		   got, err := svc.Tick(c.Request.Context(), svc.now())
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, got)
	   })
	  g.POST("/shard-failures", func(c *gin.Context) {
		  if !allow(c, perm.ActOpsAdmin) {
			   return
		   }
		  var ev ShardFailEvent
		  if !mw.BindJSON(c, &ev) {
			  return
		   }
		  got, err := svc.IngestShard(c.Request.Context(), ev)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, got)
	  })
	   g.POST("/hooks/stuck", func(c *gin.Context) {
		   if !hook(c, svc, SourceStuck) {
			  return
		   }
	   })
	   g.POST("/hooks/cancel", func(c *gin.Context) {
		   if !hook(c, svc, SourceCancel) {
			   return
		  }
	  })
	  g.POST("/hooks/recover", func(c *gin.Context) {
		  if !hook(c, svc, SourceRecover) {
			  return
		   }
	   })
   }

   func hook(c *gin.Context, svc *Orchestrator, source string) bool {
	   if !allow(c, perm.ActOpsAdmin) {
		   return false
	  }
	  var fact FailureFact
	  if !mw.BindJSON(c, &fact) {
		  return false
	   }
	   fact.Source = source
	  var (
		   got IngestResult
		   err error
	   )
	  switch source {
	  case SourceCancel:
		   got, err = svc.IngestCancel(c.Request.Context(), fact)
	   case SourceRecover:
		   got, err = svc.IngestRecover(c.Request.Context(), fact)
	  default:
		  got, err = svc.IngestStuck(c.Request.Context(), fact)
	  }
	  if err != nil {
		   mw.WriteProblem(c, err)
		   return false
	  }
	   c.JSON(200, got)
	  return true
   }

  func allow(c *gin.Context, act perm.Action) bool {
	   if err := perm.FromRequest(c.Request, "").Check(act); err != nil {
		  c.JSON(403, gin.H{"error": err.Error()})
		  return false
	  }
	  return true
  }

   func filterOf(c *gin.Context) ListFilter {
	  limit, _ := strconv.Atoi(strings.TrimSpace(c.Query("limit")))
	   return ListFilter{
		   TransferID: c.Query("transfer_id"),
		  UserID: c.Query("user_id"),
		  Asset:  c.Query("asset"),
		   Status: c.Query("status"),
		   Code:  c.Query("code"),
		  Limit: limit,
	  }
  }
