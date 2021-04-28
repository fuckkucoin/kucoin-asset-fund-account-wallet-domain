package compliance

import (
	"account-assets.kucoin.net/funding/internal/kyt"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

  func Mount(v1 *gin.RouterGroup, pipe *Pipeline) {
	  if pipe == nil {
		  pipe = New(kyt.Static{Mode: kyt.Deny, Err: kyt.ErrUnavailable}, nil)
	   }
	  v1.POST("/kyt/screen", func(c *gin.Context) {
		  var body kyt.Request
		   if err := c.ShouldBindJSON(&body); err != nil {
			   c.JSON(400, gin.H{"error": err.Error()})
			  return
		   }
		  out, err := pipe.GateWithdraw(c.Request.Context(), GateReq{
			  UserID:     body.UserID,
			   MovementID: body.MovementID, Asset: body.Asset,
			  Amount: body.Amount, Chain: body.Chain, ToAddress: body.ToAddress, Allowlisted: true,
		   })
		  if err != nil && err != ErrReview {
			   c.JSON(HTTPStatus(err), gin.H{"error": err.Error(), "decision": out.Decision, "reason": out.Reason})
			   return
		   }
		  c.JSON(200, out)
	  })
	   v1.GET("/kyt/cases", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  c.JSON(200, gin.H{"cases": pipe.Cases().Queue(50)})
	  })
	   v1.POST("/kyt/cases/:id/decide", func(c *gin.Context) {
		  actor := perm.FromRequest(c.Request, "")
		  if err := actor.Check(perm.ActKYTDecide); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			  Decision string `json:"decision"`
		   }
		  if err := c.ShouldBindJSON(&body); err != nil {
			  c.JSON(400, gin.H{"error": err.Error()})
			  return
		  }
		   got, err := pipe.Decide(c.Param("id"), actor.ID, body.Decision)
		   if err != nil {
			  c.JSON(HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		   }
		  c.JSON(200, got)
	  })
	   v1.GET("/kyt/audit", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  c.JSON(200, gin.H{
			   "trail": pipe.Trail().List(c.Query("movement_id"), 100),
			   "cases": pipe.Cases().Audit(c.Query("case_id"), 100),
		  })
	  })
	   v1.POST("/kyt/cases/expire", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActKYTDecide); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   c.JSON(200, gin.H{"expired": pipe.ExpireStale()})
	   })
	  v1.GET("/kyt/policy", func(c *gin.Context) {
		  doc, err := pipe.Policy().Current()
		   if err != nil {
			  c.JSON(HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		   }
		   c.JSON(200, gin.H{"policy": doc, "hooks": pipe.Engine().Names(), "label": "fail-closed rules engine"})
	   })
   }
