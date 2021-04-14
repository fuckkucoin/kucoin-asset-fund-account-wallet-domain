package disputedesk

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  RegisterWith(v1, rt, New())
   }

   func RegisterWith(v1 *gin.RouterGroup, rt *app.Runtime, svc *Service) {
	  if svc == nil {
		   svc = New()
	  }
	   _ = rt
	  v1.POST("/dispute/open", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAMLOpen); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body OpenReq
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  if body.OpenedBy == "" {
			   body.OpenedBy = perm.FromRequest(c.Request, "ops-desk").ID
		  }
		   got, err := svc.Open(body)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, got)
	  })
	   v1.POST("/dispute/evidence", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAMLOpen); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			   ID string `json:"id"`
			   Actor string `json:"actor"`
			   Kind string `json:"kind"`
			  Note    string `json:"note"`
			   SHA256  string `json:"sha256"`
			   Payload string `json:"payload,omitempty"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		  var (
			   got Evidence
			  err error
		   )
		  if body.Payload != "" {
			  got, err = svc.AttachPayload(body.ID, body.Actor, body.Kind, body.Note, []byte(body.Payload))
		   } else {
			   got, err = svc.Attach(body.ID, body.Actor, body.Kind, body.Note, body.SHA256)
		  }
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, got)
	  })
	   v1.POST("/dispute/review", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAMLOpen); err != nil {
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
		  got, err := svc.MarkReview(body.ID, body.Actor)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, got)
	  })
	   v1.POST("/dispute/resolve", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAMLClose); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			  ID    string `json:"id"`
			  Actor string `json:"actor"`
			   Decision string `json:"decision"`
			   Note     string `json:"note"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  if hook := currentPayHook(); hook != nil {
			  got, leaseID, err := hook(body.ID, body.Actor, body.Decision, body.Note)
			   if err != nil {
				   mw.WriteProblem(c, err)
				   return
			  }
			   c.JSON(200, gin.H{"case": got, "dispute_pay_lease_id": leaseID})
			   return
		   }
		   got,
			   err := svc.ResolveFourEye(body.ID, body.Actor, body.Decision, body.Note)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, got)
	  })
	  v1.GET("/dispute/cases", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   c.JSON(200, gin.H{"cases": svc.List(c.Query("channel"), c.Query("user_id"), 50), "host": "kyt.kucoin.net"})
	  })
	   v1.GET("/dispute/timeline", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  c.JSON(200, gin.H{"events": svc.Timeline(c.Query("id"))})
	  })
	   v1.GET("/dispute/evidence", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  c.JSON(200, gin.H{"evidence": svc.EvidenceOf(c.Query("id"))})
	  })
   }
