package loanbook

import (
	"account-assets.kucoin.net/funding/internal/app"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
)

   func Register(v1 *gin.RouterGroup,
	   rt *app.Runtime) {
	   svc := New()
	   if rt != nil {
		   svc.BindLedger(rt.Ledger)
	  }
	  v1.POST("/loan/open", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body OpenReq
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		  got, err := svc.Open(body)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, got)
	   })
	  v1.GET("/loan/schedule", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  rows, err := svc.ScheduleOf(c.Query("id"))
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, gin.H{"installments": rows, "host": "account-book.kucoin.net"})
	   })
	  v1.POST("/loan/pay", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			   ID string `json:"id"`
			  Seq int `json:"seq"`
			  Amount string `json:"amount"`
			  Actor  string `json:"actor"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  if body.Actor == "" {
			  body.Actor = perm.FromRequest(c.Request, "ops-loan").ID
		   }
		  got, err := svc.Pay(body.ID, body.Seq, body.Amount, body.Actor)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, got)
	   })
	  v1.POST("/loan/scan-overdue", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountFreeze); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body struct {
			   Actor string `json:"actor"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		  n,
			  err := svc.ScanOverdue(c.Request.Context(), body.Actor)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, gin.H{"frozen": n})
	   })
	   v1.POST("/loan/topup", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			  LoanID string `json:"loan_id"`
			   Actor  string `json:"actor"`
			   Note string `json:"note"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		  got,
			   err := svc.OpenTopUp(body.LoanID, body.Actor, body.Note)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, got)
	   })
	   v1.GET("/loans", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   c.JSON(200, gin.H{"loans": svc.List(c.Query("user_id"), 50)})
	   })
	  registerMore(v1, svc)
   }
