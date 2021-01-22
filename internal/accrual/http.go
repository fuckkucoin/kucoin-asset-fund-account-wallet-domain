// 跨日应计引擎：HTTP API 层。
// 接口：
// POST /accrual/batches — 创建应计批次（draft）
// POST /accrual/batches/:id/lines — 追加分录对
// POST /accrual/batches/:id/post — 过账
// POST /accrual/batches/:id/reverse — 冲正
// GET /accrual/batches/:id — 查询批次
// GET /accrual/batches/:id/lines — 查询分录列表
// GET /accrual/batches — 按营业日列表

// Author: EliotVoss
// Date: 2021.01.20

package accrual

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/mw"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/perm"
)

// RegisterHTTP 注册应计引擎 HTTP 路由到 gin 路由组。

  func RegisterHTTP(v1 *gin.RouterGroup, engine *Engine) {
	   if engine == nil {
		   engine = NewEngine()
	   }
	  // 创建应计批次
	   v1.POST("/accrual/batches", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   var body struct {
			   BusinessDay string `json:"business_day"`
			   DayCloseID string `json:"day_close_id"`
			  Note string `json:"note"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   actor := perm.FromRequest(c.Request, "").ID
		  bat, err := engine.CreateBatch(body.BusinessDay, body.DayCloseID, actor, body.Note)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, gin.H{"batch": bat})
	  })
	  // 追加分录对
	   v1.POST("/accrual/batches/:id/lines", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   var body struct {
			  Kind string `json:"kind"`
			   DebitUserID string `json:"debit_user_id"`
			   CreditUserID string `json:"credit_user_id"`
			   Asset      string `json:"asset"`
			   Amount     string `json:"amount"`
			  MovementID string `json:"movement_id"`
			   Note string `json:"note"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		   debit, credit, err := engine.AddLinePair(
			  c.Param("id"),
			   body.Kind,
			  body.DebitUserID,
			   body.CreditUserID,
			   body.Asset,
			  body.Amount,
			  body.MovementID,
			  body.Note,
		  )
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(http.StatusOK, gin.H{"debit": debit, "credit": credit})
	   })
	   // 过账
	  v1.POST("/accrual/batches/:id/post", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerFreeze); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   var body struct {
			   Epoch int64  `json:"epoch"`
			  Token string `json:"token"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   actor := perm.FromRequest(c.Request, "").ID
		   bat, err := engine.Post(c.Param("id"), actor, body.Epoch, body.Token)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, gin.H{"batch": bat})
	  })
	   // 冲正
	   v1.POST("/accrual/batches/:id/reverse", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			  Note string `json:"note"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		   actor := perm.FromRequest(c.Request, "").ID
		  revBat, err := engine.Reverse(c.Param("id"), actor, body.Note)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(http.StatusOK, gin.H{"reversal_batch": revBat})
	  })
	   // 查询单个批次
	   v1.GET("/accrual/batches/:id", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   bat, err := engine.GetBatch(c.Param("id"))
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, gin.H{"batch": bat})
	  })
	   // 查询分录列表
	   v1.GET("/accrual/batches/:id/lines", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   lines := engine.ListLines(c.Param("id"))
		   c.JSON(http.StatusOK, gin.H{"lines": lines, "count": len(lines)})
	  })
	   // 按营业日列出批次
	   v1.GET("/accrual/batches", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   day := c.Query("business_day")
		  var batches []AccrualBatch
		  if day != "" {
			   batches = engine.ListByDay(day)
		  } else {
			  batches = engine.ListBatches()
		  }
		   c.JSON(http.StatusOK, gin.H{"batches": batches, "count": len(batches)})
	  })
   }
