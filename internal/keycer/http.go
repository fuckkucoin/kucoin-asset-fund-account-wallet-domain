package keycer

import (
	"account-assets.kucoin.net/funding/internal/app"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
)

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   svc := Process()
	   _ = rt
	   v1.POST("/keycer/load", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCeremony); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			   ID string `json:"id"`
			   Label string `json:"label"`
			   FP    string `json:"fingerprint"`
			   Actor string `json:"actor"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		   got, err := svc.Load(body.ID, body.Label, body.FP, body.Actor)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, got)
	  })
	  v1.POST("/keycer/lock", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCeremony); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			  ID    string `json:"id"`
			   Actor string `json:"actor"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  got, err := svc.Lock(body.ID, body.Actor)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, got)
	  })
	   v1.POST("/keycer/unlock-open", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCeremony); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			   SlotID string `json:"slot_id"`
			   Actor string `json:"actor"`
			  Note  string `json:"note"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   got, err := svc.OpenUnlock(body.SlotID, body.Actor, body.Note)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, got)
	  })
	   v1.POST("/keycer/unlock-approve", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCeremony); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body struct {
			   ID    string `json:"id"`
			  Actor string `json:"actor"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  got,
			  err := svc.Approve(body.ID, body.Actor)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, got)
	   })
	   v1.POST("/keycer/unlock", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCeremony); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			   ID string `json:"id"`
			   Actor string `json:"actor"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		   got, err := svc.Unlock(body.ID, body.Actor)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, got)
	   })
	   v1.POST("/keycer/rotate-open", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCeremony); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			  From string `json:"from"`
			  To   string `json:"to"`
			  Actor string `json:"actor"`
			   Note  string `json:"note"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		   got, err := svc.OpenRotate(body.From, body.To, body.Actor, body.Note)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, got)
	  })
	  v1.POST("/keycer/rotate-exec", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCeremony); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			  ID    string `json:"id"`
			   Actor string `json:"actor"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  got,
			  err := svc.ExecuteRotate(body.ID, body.Actor)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, got)
	  })
	   v1.GET("/keycer/slots", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   c.JSON(200, gin.H{"slots": svc.List(), "host": "custody-boundary-lab.kucoin.net"})
	   })
	   v1.GET("/keycer/journal", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  c.JSON(200, gin.H{"rotations": svc.Journal()})
	  })
   }
