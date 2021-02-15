package airgap

import (
	"encoding/base64"
	"account-assets.kucoin.net/funding/internal/app"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  svc := Process()
	  _ = rt
	  v1.POST("/airgap/open", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCeremony); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			   Actor string `json:"actor"`
			   Label string `json:"label"`
			  Shares    int     `json:"shares"`
			   Threshold int     `json:"threshold"`
			  Payload   Payload `json:"payload"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		  got,
			   err := svc.Open(body.Actor, body.Label, body.Shares, body.Threshold, body.Payload)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, got)
	  })
	  v1.POST("/airgap/sign", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCeremony); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			   ID    string `json:"id"`
			   Actor string `json:"actor"`
			  Note  string `json:"note"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		  got, err := svc.Sign(body.ID, body.Actor, body.Note)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, got)
	  })
	  v1.POST("/airgap/seal", func(c *gin.Context) {
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
			   err := svc.Seal(body.ID, body.Actor)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, got)
	   })
	   v1.POST("/airgap/export", func(c *gin.Context) {
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
		  got, err := svc.Export(body.ID, body.Actor)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, got)
	   })
	  v1.POST("/airgap/export-sealed", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCeremony); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			   ID string `json:"id"`
			   Actor string `json:"actor"`
			   Pass  string `json:"passphrase"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		   blob, err := svc.ExportSealed(body.ID, body.Actor, []byte(body.Pass))
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, gin.H{"blob": base64.StdEncoding.EncodeToString(blob), "host": "custody-boundary-lab.kucoin.net"})
	  })
	  v1.POST("/airgap/import-sealed", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCeremony); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body struct {
			   Actor string `json:"actor"`
			  Pass  string `json:"passphrase"`
			  Blob  []byte `json:"blob"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		   got, err := svc.ImportSealed([]byte(body.Pass), body.Blob, body.Actor)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, got)
	   })
	  v1.POST("/airgap/import", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCeremony); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body struct {
			  Envelope Envelope `json:"envelope"`
			   Actor    string   `json:"actor"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		   got, err := svc.Import(body.Envelope, body.Actor)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, got)
	  })
	  v1.GET("/airgap/sessions", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  c.JSON(200, gin.H{"sessions": svc.List(50), "host": "custody-boundary-lab.kucoin.net"})
	   })
  }
