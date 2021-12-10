package travelvasp

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/mw"
	"github.com/gin-gonic/gin"
)

  func RegisterCustomer(v1 *gin.RouterGroup, rt *app.Runtime) {
	  if v1 == nil {
		  return
	   }
	   opt := callerid.FromRuntime(false, "", "", "")
	   if rt != nil && rt.Cfg != nil {
		  opt = callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile)
	  }
	  g := v1.Group("/travel/customer")
	  g.GET("/requirements", func(c *gin.Context) {
		  uid, err := callerid.Require(c, opt)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   gate := GateForRuntime(rt)
		   view, err := gate.Requirements(RequirementsReq{
			  Asset: c.Query("currency"), Amount: c.Query("amount"),
			   BeneficiaryVASP: c.Query("beneficiaryVasp"),
			   OriginCountry:   c.Query("originCountry"),
			  PeerCountry: c.Query("peerCountry"),
		   })
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, gin.H{"user_id": uid, "requirements": view, "host": "kyt.kucoin.net"})
	   })
	   g.POST("/outbound", func(c *gin.Context) {
		   var body struct {
			   Currency string `json:"currency"`
			   Amount   string `json:"amount"`
			   Chain    string `json:"chain"`
			  ToAddress string `json:"toAddress"`
			   BeneficiaryName string `json:"beneficiaryName"`
			  BeneficiaryVasp string `json:"beneficiaryVasp"`
			  ClientOid       string `json:"clientOid"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  uid,
			  err := callerid.Require(c, opt)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   msg, err := GateForRuntime(rt).OpenOutbound(c.Request.Context(), OpenOutboundReq{
			   UserID: uid, ClientOid: body.ClientOid, Asset: body.Currency, Amount: body.Amount,
			   Chain: body.Chain, ToAddress: body.ToAddress, BeneficiaryName: body.BeneficiaryName,
			  BeneficiaryVASP: body.BeneficiaryVasp, Actor: "user:" + uid,
		   })
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, gin.H{"message": msg})
	  })
	  g.GET("/outbound/:id", func(c *gin.Context) {
		  _, err := callerid.Require(c, opt)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  msg, err := GateForRuntime(rt).GetOutbound(strings.TrimSpace(c.Param("id")))
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, gin.H{"message": msg})
	   })
	   g.POST("/outbound/:id/submit", func(c *gin.Context) {
		   uid, err := callerid.Require(c, opt)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   msg, err := GateForRuntime(rt).SubmitOutbound(strings.TrimSpace(c.Param("id")), "user:"+uid)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, gin.H{"message": msg})
	  })
   }
