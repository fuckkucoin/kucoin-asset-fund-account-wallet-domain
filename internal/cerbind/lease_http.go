package cerbind

import (
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// MountLease 挂 Decide 租约路由。

  func MountLease(v1 *gin.RouterGroup, eng *Engine) {
	  if v1 == nil || eng == nil {
		  return
	   }
	  v1.POST("/cerbind/decide-with-lease", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCeremony); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body struct {
			  PlanReq
			   IdempotencyKey string `json:"idempotency_key"`
			   TTLSec int64 `json:"ttl_sec"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		  got, err := eng.DecideWithLease(c.Request.Context(), body.PlanReq, body.IdempotencyKey, body.TTLSec)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, got)
	   })
	  v1.POST("/cerbind/assert-exec-lease", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCeremony); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			  LeaseID string  `json:"decide_lease_id"`
			   Actor   string  `json:"actor"`
			  ExecRef string  `json:"exec_ref"`
			   Subject Subject `json:"subject"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  got, err := eng.AssertExecLease(c.Request.Context(), body.LeaseID, body.Actor, body.ExecRef, body.Subject)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, got)
	  })
   }
