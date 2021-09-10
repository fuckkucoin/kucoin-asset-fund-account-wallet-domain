// @FileName: http.go
// @Summary: 运维鉴权与角色校验之 HTTP 接口层。
// @Author: Kinsley
// @Created: 2021.09.09

package opsauth

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/opsfc"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/gin-gonic/gin"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   desk := NewDesk("", nil)
	  v1.POST("/ops/signed", func(c *gin.Context) {
		  if opsfc.Reject(c, perm.ActOpsAdmin) {
			   return
		  }
		  actor := perm.FromRequest(c.Request, "")
		  var body struct {
			  Action string `json:"action"`
			  Subject string `json:"subject"`
			  Body    string `json:"body"`
			  Nonce     string `json:"nonce"`
			  Signature string `json:"signature"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		   if _, err := validate.Actor(actor.ID); err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  rec, err := desk.Execute(ActionReq{
			   Actor: actor.ID, Role: actor.Role, Action: perm.Action(body.Action),
			  Subject: body.Subject, Body: body.Body, Nonce: body.Nonce, Signature: body.Signature,
		  })
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, gin.H{"receipt": rec, "host": config.HostIAMGateway})
	   })
	   v1.GET("/ops/signed/audit", func(c *gin.Context) {
		   if opsfc.Reject(c, perm.ActOpsAdmin) {
			   return
		  }
		  c.JSON(200, gin.H{"entries": desk.Audit().List("admin_action", c.Query("subject"), 50)})
	  })
	   v1.GET("/ops/actions", func(c *gin.Context) {
		  if opsfc.Reject(c, perm.ActOpsAdmin) {
			   return
		  }
		  c.JSON(200, gin.H{"actions": Catalog(), "host": config.HostIAMGateway})
	   })
   }
