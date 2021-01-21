// 托管与多方签发域之法定人数签名 HTTP 接口。

// Author: Knox
// Date: 2021.01.20

package custody

import (
	"errors"
	"net/http"
	"account-assets.kucoin.net/funding/internal/app"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/dualctl"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
)

   func quorumStatus(err error) int {
	  switch {
	   case errors.Is(err,
		   dualctl.ErrActor), errors.Is(err, dualctl.ErrNotReady):
		  return http.StatusForbidden
	   case errors.Is(err, dualctl.ErrNotFound):
		   return http.StatusNotFound
	  case errors.Is(err, dualctl.ErrClosed), errors.Is(err, dualctl.ErrDuplicate), errors.Is(err, dualctl.ErrStale):
		  return http.StatusConflict
	   case errors.Is(err,
		  dualctl.ErrNeed):
		  return http.StatusBadRequest
	   default:
		  return status(err)
	  }
  }

  func registerQuorum(v1 *gin.RouterGroup, rt *app.Runtime, svc *Service) {
	   _ = rt
	  q := svc.Quorum()
	  v1.POST("/custody/quorum", func(c *gin.Context) {
		  actor := perm.FromRequest(c.Request, "")
		  if err := actor.Check(perm.ActCustodyApprove); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body struct {
			  Kind string `json:"kind"`
			  Subject string `json:"subject"`
			   Need int `json:"need"`
			   Note string `json:"note"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  if body.Need == 0 {
			  body.Need = 2
		   }
		  got, err := q.Open(body.Kind, actor.ID, body.Subject, body.Need, body.Note)
		   if err != nil {
			   c.JSON(quorumStatus(err), gin.H{"error": err.Error()})
			  return
		   }
		  c.JSON(200, got)
	  })
	  v1.GET("/custody/quorum", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   c.JSON(200, gin.H{"requests": q.List(50), "ready": q.ReadyCount(), "label": "n-of-m dual control"})
	  })
	   v1.POST("/custody/quorum/:id/vote", func(c *gin.Context) {
		  actor := perm.FromRequest(c.Request, "")
		  if err := actor.Check(perm.ActCustodyApprove); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			  Allow bool   `json:"allow"`
			  Epoch int64  `json:"epoch"`
			  Token string `json:"token"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  got, err := q.Vote(c.Param("id"), actor.ID, body.Allow, body.Epoch, body.Token)
		   if err != nil {
			   c.JSON(quorumStatus(err), gin.H{"error": err.Error(), "request": got})
			  return
		  }
		   c.JSON(200, got)
	   })
	   v1.POST("/custody/quorum/:id/execute", func(c *gin.Context) {
		   actor := perm.FromRequest(c.Request, "")
		   if err := actor.Check(perm.ActCustodySweep); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   var body struct {
			  Epoch int64 `json:"epoch"`
			   Token string `json:"token"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  got, err := q.Execute(c.Param("id"), actor.ID, body.Epoch, body.Token)
		  if err != nil {
			   c.JSON(quorumStatus(err), gin.H{"error": err.Error()})
			  return
		   }
		  c.JSON(200, got)
	  })
	  v1.POST("/custody/quorum/:id/revoke", func(c *gin.Context) {
		   actor := perm.FromRequest(c.Request, "")
		   if err := actor.Check(perm.ActCustodyApprove); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   got, err := q.Revoke(c.Param("id"), actor.ID)
		  if err != nil {
			   c.JSON(quorumStatus(err), gin.H{"error": err.Error()})
			  return
		  }
		   c.JSON(200, got)
	   })
	   v1.POST("/custody/quorum/expire", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodyApprove); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   c.JSON(200, gin.H{"expired": q.ExpireDue()})
	  })
   }
