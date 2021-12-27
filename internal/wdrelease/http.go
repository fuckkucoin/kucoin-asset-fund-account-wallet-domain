package wdrelease

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   d := Attach(rt, nil)
	   v1.POST("/ops/wd-release/eval", func(c *gin.Context) {
		   if !allow(c, perm.ActQueryRead, perm.ActWithdrawSubmit) {
			   c.JSON(403, gin.H{"error": "permission denied"})
			   return
		   }
		  var in Input
		  if !mw.BindJSON(c, &in) {
			   return
		   }
		   dec, err := d.Eval(in)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, dec)
	   })
	   v1.POST("/ops/wd-release/open", func(c *gin.Context) {
		   if !allow(c, perm.ActWithdrawSubmit) {
			   c.JSON(403, gin.H{"error": "permission denied"})
			  return
		  }
		   var in Input
		   if !mw.BindJSON(c, &in) {
			  return
		  }
		  if in.Applicant == "" {
			  in.Applicant = perm.FromRequest(c.Request, "").ID
		   }
		  got, err := d.Open(c.Request.Context(), in)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, got)
	  })
	   v1.POST("/ops/wd-release/submit", func(c *gin.Context) {
		  if !allow(c, perm.ActWithdrawSubmit) {
			   c.JSON(403, gin.H{"error": "permission denied"})
			   return
		   }
		  var body struct {
			  ID    string `json:"id"`
			   Actor string `json:"actor"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		   if body.Actor == "" {
			  body.Actor = perm.FromRequest(c.Request, "").ID
		   }
		   got, err := d.Submit(c.Request.Context(), body.ID, body.Actor)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, got)
	  })
	   v1.POST("/ops/wd-release/advance", func(c *gin.Context) {
		   if !allow(c, perm.ActWithdrawDecide) {
			   c.JSON(403, gin.H{"error": "permission denied"})
			   return
		  }
		   var body struct {
			   ID    string `json:"id"`
			   Actor string `json:"actor"`
			  Role  string `json:"role"`
			  Allow *bool `json:"allow"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		   if body.Actor == "" {
			  body.Actor = perm.FromRequest(c.Request, "").ID
		   }
		  allowStep := true
		   if body.Allow != nil {
			   allowStep = *body.Allow
		  }
		  got, err := d.Advance(c.Request.Context(), body.ID, body.Actor, body.Role, allowStep)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, got)
	   })
	   v1.POST("/ops/wd-release/reject", func(c *gin.Context) {
		  if !allow(c, perm.ActWithdrawDecide) {
			  c.JSON(403, gin.H{"error": "permission denied"})
			   return
		  }
		  var body struct {
			  ID    string `json:"id"`
			  Actor string `json:"actor"`
			  Role  string `json:"role"`
			  Code  string `json:"code"`
			   Note  string `json:"note"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		  if body.Actor == "" {
			   body.Actor = perm.FromRequest(c.Request, "").ID
		  }
		   got,
			   err := d.Reject(c.Request.Context(), body.ID, body.Actor, body.Role, body.Code, body.Note)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, got)
	   })
	   v1.POST("/ops/wd-release/segments/authorize", func(c *gin.Context) {
		  if !allow(c, perm.ActWithdrawDecide) {
			  c.JSON(403, gin.H{"error": "permission denied"})
			   return
		  }
		  var body struct {
			  ID       string `json:"id"`
			   Actor    string `json:"actor"`
			   Seq      int    `json:"seq"`
			  Override bool   `json:"override"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		   if body.Actor == "" {
			   body.Actor = perm.FromRequest(c.Request, "").ID
		  }
		  got, err := d.AuthorizeSegment(c.Request.Context(), body.ID, body.Actor, body.Seq, body.Override)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, got)
	  })
	  v1.GET("/ops/wd-release/queue", func(c *gin.Context) {
		  if !allow(c, perm.ActQueryRead) {
			  c.JSON(403, gin.H{"error": "permission denied"})
			   return
		   }
		   c.JSON(200, gin.H{"queue": d.Queue(c.Query("state"), 50), "host": config.HostDepositWithdraw})
	  })
	  v1.GET("/ops/wd-release/tickets/:id", func(c *gin.Context) {
		  if !allow(c, perm.ActQueryRead) {
			   c.JSON(403, gin.H{"error": "permission denied"})
			   return
		  }
		  got, err := d.Get(c.Param("id"))
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  aud, _ := d.Audits(got.ID)
		  c.JSON(200, gin.H{"ticket": got, "audit": aud})
	   })
	  v1.POST("/ops/wd-release/tick", func(c *gin.Context) {
		  if !allow(c, perm.ActWithdrawDecide, perm.ActOpsAdmin) {
			  c.JSON(403, gin.H{"error": "permission denied"})
			   return
		  }
		   res, err := d.Tick(c.Request.Context())
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, res)
	   })
	  v1.GET("/ops/wd-release/metrics", func(c *gin.Context) {
		  if !allow(c, perm.ActQueryRead) {
			   c.JSON(403, gin.H{"error": "permission denied"})
			  return
		   }
		  c.JSON(200, d.Snapshot())
	  })
	  v1.GET("/ops/wd-release/codes", func(c *gin.Context) {
		   if !allow(c, perm.ActQueryRead) {
			   c.JSON(403, gin.H{"error": "permission denied"})
			  return
		  }
		  c.JSON(200, gin.H{"codes": CodeCatalog()})
	   })
	   v1.GET("/ops/wd-release/policy", func(c *gin.Context) {
		   if !allow(c, perm.ActQueryRead) {
			  c.JSON(403, gin.H{"error": "permission denied"})
			   return
		  }
		   ver, err := d.Version(c.Query("version"))
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, ver)
	   })
	   v1.POST("/ops/wd-release/publish", func(c *gin.Context) {
		  if !allow(c, perm.ActOpsAdmin) {
			   c.JSON(403, gin.H{"error": "permission denied"})
			  return
		   }
		  var body struct {
			  Version string `json:"version"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		  if err := d.Publish(body.Version); err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, gin.H{"active": d.ActiveVersion()})
	  })
	   v1.POST("/ops/wd-release/replay", func(c *gin.Context) {
		   if !allow(c, perm.ActOpsAdmin) {
			  c.JSON(403, gin.H{"error": "permission denied"})
			  return
		   }
		   var fx Fixture
		  if !mw.BindJSON(c, &fx) {
			  return
		   }
		  got, err := Replay(c.Request.Context(), fx)
		  if err != nil {
			  c.JSON(200, gin.H{"ticket": got, "error": err.Error(), "code": CodeOf(err)})
			   return
		   }
		  c.JSON(200, got)
	   })
  }

   func allow(c *gin.Context, acts ...perm.Action) bool {
	   a := perm.FromRequest(c.Request, "")
	   if strings.TrimSpace(a.ID) == "" {
		   a.ID = "desk-user"
	  }
	  for _, act := range acts {
		  if a.Check(act) == nil {
			   return true
		  }
	   }
	   return false
  }
