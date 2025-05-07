package accrual

import (
	"net/http"
	"time"
	"account-assets.kucoin.net/funding/internal/clearing"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// RegisterHTTPExt 注册扩展 HTTP 路由到 gin 路由组。

   func RegisterHTTPExt(v1 *gin.RouterGroup, svc *Service, dcSvc *clearing.DayCloseService) {
	  if svc == nil {
		   return
	   }
	  // 应计回放
	  v1.GET("/accrual/replay", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  day := c.Query("business_day")
		  if day == "" {
			  day = time.Now().UTC().Format("2006-01-02")
		  }
		  report := svc.Engine().Replay(day)
		   c.JSON(http.StatusOK, gin.H{"replay": report})
	   })
	   // 手动触发完整日切
	  v1.POST("/accrual/daycut", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			   BusinessDay string `json:"business_day"`
			   BatchID string `json:"batch_id"` // 可选：指定批次 ID 获取在途 items
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		   if body.BusinessDay == "" {
			   body.BusinessDay = time.Now().UTC().Format("2006-01-02")
		   }
		  actor := perm.FromRequest(c.Request, "").ID
		   dc,
			  bat, err := svc.RunDayCutFull(c.Request.Context(), body.BusinessDay, actor, nil, body.BatchID)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, gin.H{
			  "day_close": dc,
			  "accrual_batch": bat,
		   })
	  })
	   // 营业日应计汇总报告
	   v1.GET("/accrual/report", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   day := c.Query("business_day")
		   if day == "" {
			   day = time.Now().UTC().Format("2006-01-02")
		  }
		   batches := svc.Engine().ListByDay(day)
		   totalDebit := "0"
		  totalCredit := "0"
		   for _, bat := range batches {
			   // 简单汇总最新批次的总额
			  if bat.Status == BatchPosted {
				   totalDebit = bat.TotalDebit
				  totalCredit = bat.TotalCredit
			   }
		   }
		  c.JSON(http.StatusOK, gin.H{
			   "business_day": day,
			  "batch_count":  len(batches),
			   "total_debit":  totalDebit,
			   "total_credit": totalCredit,
			   "batches": batches,
		  })
	  })
	   // 应计批次过账（带服务层指标）
	  v1.POST("/accrual/batches/:id/post-ext", func(c *gin.Context) {
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
		  bat, err := svc.PostBatch(c.Request.Context(), c.Param("id"), actor, body.Epoch, body.Token)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, gin.H{"batch": bat})
	  })
	  // 应计批次冲正（带服务层指标）
	  v1.POST("/accrual/batches/:id/reverse-ext", func(c *gin.Context) {
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
		  revBat, err := svc.ReverseBatch(c.Request.Context(), c.Param("id"), actor, body.Note)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, gin.H{"reversal_batch": revBat})
	   })
  }
