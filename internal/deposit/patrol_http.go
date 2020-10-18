// @FileName: patrol_http.go
// @Summary: patrol_http.go
// 入金巡检、回退读面、刷充熔断和回收工单的 HTTP。
// 和提前入账要求入金观察权限。调整熔断笔数、派单和批准要求运营权限。
// CreateDate: 2022.02.24
// @Author: Jett
// @Created: 2020.10.17

package deposit

import (
	"strconv"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

  func registerPatrol(v1 *gin.RouterGroup, rt *app.Runtime, svc *Service) {
	   if v1 == nil || svc == nil {
		   return
	   }
	  if rt != nil {
		  svc.BindRecon(rt.Durable.Recon)
	  }
	   v1.GET("/deposit-patrol/contract", func(c *gin.Context) {
		  c.JSON(200, gin.H{"event": EventDepositDiff, "fields": ContractFields(), "phases": PhaseCatalog(), "kinds": KindCatalog()})
	  })
	  v1.POST("/deposit-patrol/apply", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActDepositObserve); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body PatrolIn
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  out, err := svc.ApplyPatrol(body)
		   if err != nil {
			  c.JSON(409, gin.H{"error": err.Error(), "result": out})
			  return
		   }
		  c.JSON(200, out)
	  })
	  v1.POST("/deposit-patrol/credit", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActDepositObserve); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			  Chain  string `json:"chain"`
			  TxHash string `json:"tx_hash"`
			  OutputIndex int `json:"output_index"`
			   TransferID string `json:"transfer_id"`
			   Amount     string `json:"amount"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   if err := svc.TrySecondCredit(body.Chain, body.TxHash, body.TransferID, body.Amount, body.OutputIndex); err != nil {
			  c.JSON(409, gin.H{"error": err.Error(), "second_credit": false})
			  return
		   }
		  c.JSON(200, gin.H{"second_credit": false, "allowed": true})
	   })
	   v1.POST("/deposit-patrol/early", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActDepositObserve); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body RollbackNote
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		  face, err := svc.NoteEarly(c.Request.Context(), body)
		   if err != nil {
			   c.JSON(409, gin.H{"error": err.Error(), "face": face})
			  return
		   }
		  c.JSON(200, face)
	   })
	   v1.POST("/deposit-patrol/rollback", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActDepositObserve); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body RollbackNote
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   face, err := svc.ApplyRollback(c.Request.Context(), body)
		   if err != nil {
			   c.JSON(409, gin.H{"error": err.Error(), "face": face})
			  return
		  }
		  c.JSON(200, face)
	  })
	   v1.GET("/deposit-patrol/summary", func(c *gin.Context) {
		  output, _ := strconv.Atoi(c.DefaultQuery("output_index", "0"))
		   c.JSON(200, svc.Summarize(c.Query("chain"), c.Query("tx_hash"), output))
	  })
	   v1.GET("/deposit-patrol/face", func(c *gin.Context) {
		   output, _ := strconv.Atoi(c.DefaultQuery("output_index", "0"))
		  face, err := svc.ReadFace(c.Query("chain"), c.Query("tx_hash"), output)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, face)
	   })
	   v1.GET("/deposit-patrol/audit", func(c *gin.Context) {
		  output, _ := strconv.Atoi(c.DefaultQuery("output_index", "0"))
		   rows := svc.Rolls().Audit(c.Query("chain"), c.Query("tx_hash"), output, 20)
		  c.JSON(200, gin.H{"audit": rows})
	   })
	  v1.POST("/deposit-patrol/abuse/cap", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			  Cap       int `json:"cap"`
			   WindowSec int `json:"window_sec"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  if body.Cap <= 0 || body.WindowSec <= 0 {
			  c.JSON(400, gin.H{"error": ErrAbuseKey.Error()})
			  return
		  }
		  if err := svc.Circuit().SetCap(body.Cap, seconds(body.WindowSec)); err != nil {
			  c.JSON(400, gin.H{"error": err.Error()})
			   return
		   }
		  c.JSON(200, svc.Circuit().Snapshot(c.Query("user_id"), c.Query("chain"), c.Query("asset")))
	  })
	  v1.GET("/deposit-patrol/abuse", func(c *gin.Context) {
		   snap := svc.Circuit().Snapshot(c.Query("user_id"), c.Query("chain"), c.Query("asset"))
		   c.JSON(200, snap)
	  })
	   v1.GET("/deposit-patrol/recovery", func(c *gin.Context) {
		   c.JSON(200, gin.H{"tickets": svc.Recoveries().List(c.Query("user_id"), 50)})
	   })
	   v1.POST("/deposit-patrol/recovery/:id/assign", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			   Actor string `json:"actor"`
			   Assignee string `json:"assignee"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		  got, err := svc.Recoveries().Assign(c.Param("id"), body.Actor, body.Assignee)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, got)
	  })
	  v1.POST("/deposit-patrol/recovery/:id/approve", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			   Actor string `json:"actor"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  got, err := svc.Recoveries().Approve(c.Param("id"), body.Actor)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, got)
	  })
   }
