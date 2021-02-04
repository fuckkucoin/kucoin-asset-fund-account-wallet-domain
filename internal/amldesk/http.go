// 功能：反洗钱人工审核台之 HTTP 接口层。
// 作者：Knox
// 日期：2021.02.02

package amldesk

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/opsauth"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/gin-gonic/gin"
)

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   svc := New()
	  _ = rt
	   v1.POST("/aml/cases", func(c *gin.Context) {
		  actor := perm.FromRequest(c.Request, "")
		   if err := opsauth.RequireAny(actor, perm.ActKYTDecide, perm.ActAMLOpen, perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			   UserID     string `json:"user_id"`
			  MovementID string `json:"movement_id"`
			  Kind       string `json:"kind"`
			  Reason     string `json:"reason"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		   uid, err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   body.UserID = uid
		  if _, err := validate.Actor(actor.ID); err != nil {
			   mw.WriteProblem(c, ErrActor)
			   return
		  }
		   rec, err := svc.Open(body.UserID, body.MovementID, body.Kind, body.Reason, actor.ID)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, gin.H{"case": rec, "host": "kyt.kucoin.net"})
	   })
	   v1.POST("/aml/cases/batch", func(c *gin.Context) {
		  actor := perm.FromRequest(c.Request, "")
		   if err := opsauth.RequireAny(actor, perm.ActAMLOpen, perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			   Items []OpenItem `json:"items"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   got, err := svc.OpenBatch(actor.ID, body.Items)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, got)
	  })
	   v1.POST("/aml/cases/:id/evidence", func(c *gin.Context) {
		   actor := perm.FromRequest(c.Request, "")
		   if err := actor.Check(perm.ActCaseRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			  Kind string `json:"kind"`
			   Body string `json:"body"`
			   Note string `json:"note"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  ev, err := svc.Attach(c.Param("id"), actor.ID, body.Kind, body.Body, body.Note)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, ev)
	  })
	  v1.POST("/aml/cases/:id/close", func(c *gin.Context) {
		  actor := perm.FromRequest(c.Request, "")
		   if err := opsauth.RequireAny(actor, perm.ActKYTDecide, perm.ActAMLClose, perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			  Decision string `json:"decision"`
			  Note  string `json:"note"`
			  Epoch int64  `json:"epoch"`
			   Token string `json:"token"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   rec, err := svc.Close(c.Param("id"), actor.ID, body.Decision, body.Epoch, body.Token, body.Note)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, rec)
	   })
	   v1.POST("/aml/cases/:id/assign", func(c *gin.Context) {
		   actor := perm.FromRequest(c.Request, "")
		   if err := opsauth.RequireAny(actor, perm.ActAMLOpen, perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body struct {
			   Reviewer string `json:"reviewer"`
			  Epoch    int64  `json:"epoch"`
			   Token string `json:"token"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		  rec, err := svc.Assign(c.Param("id"), actor.ID, body.Reviewer, body.Epoch, body.Token)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, rec)
	  })
	   v1.POST("/aml/cases/:id/travel", func(c *gin.Context) {
		   actor := perm.FromRequest(c.Request, "")
		   if err := actor.Check(perm.ActKYTScreen); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body struct {
			   Asset string `json:"asset"`
			  Amount      string `json:"amount"`
			   Chain       string `json:"chain"`
			   TxHash      string `json:"tx_hash"`
			   Originator  Party  `json:"originator"`
			   Beneficiary Party  `json:"beneficiary"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		   p, err := ShapeTravel(body.Asset, body.Amount, body.Chain, body.TxHash, body.Originator, body.Beneficiary)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   if err := svc.Book().BindTravel(c.Param("id"), p); err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, p)
	  })
	  v1.GET("/aml/cases", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   c.JSON(200, gin.H{"cases": svc.Queue(), "host": "kyt.kucoin.net"})
	  })
	  v1.GET("/aml/cases/:id", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  rec, err := svc.Get(c.Param("id"))
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, gin.H{"case": rec, "timeline": svc.Book().Timeline(rec.ID), "evidence": svc.Book().Evidence(rec.ID)})
	   })
	  v1.POST("/aml/travel", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActKYTScreen); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			  Asset  string `json:"asset"`
			  Amount string `json:"amount"`
			   Chain  string `json:"chain"`
			   TxHash     string `json:"tx_hash"`
			  Originator Party  `json:"originator"`
			   Beneficiary Party `json:"beneficiary"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  p, err := ShapeTravel(body.Asset, body.Amount, body.Chain, body.TxHash, body.Originator, body.Beneficiary)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   hits := svc.Watch().MatchTravel(p)
		  c.JSON(200, gin.H{"payload": p, "watch_hits": hits})
	  })
	  v1.POST("/aml/watch", func(c *gin.Context) {
		  actor := perm.FromRequest(c.Request, "")
		  if err := opsauth.RequireAny(actor, perm.ActAMLOpen, perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body struct {
			   Kind string `json:"kind"`
			   Subject string `json:"subject"`
			   Reason string `json:"reason"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  k, err := ParseWatchKind(body.Kind)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   ent, err := svc.Watch().Put(k, body.Subject, body.Reason, actor.ID)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, ent)
	  })
	  v1.GET("/aml/watch", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  c.JSON(200, gin.H{"entries": svc.Watch().List(100)})
	   })
	  v1.GET("/aml/sla", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  c.JSON(200, gin.H{"sla": svc.Book().ScanSLA(svc.Book().clock(), 0)})
	   })
	   v1.POST("/aml/sla/escalate", func(c *gin.Context) {
		  actor := perm.FromRequest(c.Request, "")
		   if err := opsauth.RequireAny(actor, perm.ActAMLClose, perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   rows, err := svc.Book().EscalateOverdue(actor.ID, svc.Book().clock(), 0)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, gin.H{"escalated": rows})
	  })
	  v1.GET("/aml/export", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  raw, err := svc.Book().Export(c.Query("user_id"))
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.Data(200, "application/json", raw)
	  })
	   v1.GET("/aml/overdue", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  rows := svc.Book().QueueOverdue(svc.Book().Now(), 0)
		  c.JSON(200, gin.H{"overdue": rows, "count": len(rows), "host": "kyt.kucoin.net"})
	  })
  }
