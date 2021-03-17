package brokerbook

import (
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
)

   func registerMore(v1 *gin.RouterGroup, svc *Service) {
	  v1.POST("/broker/credit", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			   MasterID string `json:"master_id"`
			  Asset    string `json:"asset"`
			   Amount   string `json:"amount"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		  if err := svc.CreditMaster(body.MasterID, body.Asset, body.Amount); err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, gin.H{"ok": true})
	   })
	  v1.POST("/broker/fill", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			   SubID    string `json:"sub_id"`
			   Asset    string `json:"asset"`
			   Realized string `json:"realized"`
			   Note     string `json:"note"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		   got, err := svc.RecordFill(body.SubID, body.Asset, body.Realized, body.Note)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, got)
	  })
	   v1.GET("/broker/book", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  got, err := svc.BookOf(c.Query("owner"), c.Query("asset"))
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, got)
	  })
   }
