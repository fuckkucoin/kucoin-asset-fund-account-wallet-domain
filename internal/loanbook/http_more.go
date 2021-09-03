package loanbook

import (
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

  func registerMore(v1 *gin.RouterGroup, svc *Service) {
	   v1.GET("/loan/due", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  got, err := svc.DueNext(c.Query("id"))
		   if err != nil {
	  v1.POST("/loan/topup/fill", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			  ID     string `json:"id"`
			   Amount string `json:"amount"`
			   Actor string `json:"actor"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  got, err := svc.FillTopUp(body.ID, body.Amount, body.Actor)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, got)
	  })
	   v1.GET("/loan/tickets", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  c.JSON(200, gin.H{"tickets": svc.Tickets(c.Query("loan_id"))})
	  })
  }
