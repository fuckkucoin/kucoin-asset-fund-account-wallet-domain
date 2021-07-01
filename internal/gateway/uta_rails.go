// 功能：资金页 UTA 借还读面：负债摘要、风险单元、全仓/逐仓 margin 与利率阶梯。
// 作者：TianpengLan
// 日期：2021.06.30

package gateway

import (
	"strconv"
	"strings"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/utabook"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/gin-gonic/gin"
)

  func registerUTARails(v1 *gin.RouterGroup, rt *app.Runtime) {
	   book := utabook.New()
	   if rt != nil && rt.UTA != nil {
		   book = rt.UTA
	  }
	   opt := callerid.FromRuntime(false, "", "", "")
	  if rt != nil && rt.Cfg != nil {
		  opt = callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile)
	  }
	   v1.GET("/assets/uta/liability", func(c *gin.Context) {
		   uid, err := callerid.Require(c, opt)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  if _, err := validate.UserID(uid); err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, gin.H{"user_id": uid, "summary": book.LiabilitySummary(uid), "host": config.HostAccountBook})
	  })
	  v1.GET("/assets/uta/repay-history", func(c *gin.Context) {
		  uid,
			  err := callerid.Require(c, opt)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  page, _ := strconv.Atoi(strings.TrimSpace(c.DefaultQuery("page", "1")))
		  pageSize, _ := strconv.Atoi(strings.TrimSpace(c.DefaultQuery("pageSize", "50")))
		  q := utabook.RepayQuery{
			  Currency: c.Query("currency"), MarginMode: c.Query("marginMode"), Symbol: c.Query("symbol"),
			  Page: page, PageSize: pageSize,
		   }
		  c.JSON(200, gin.H{"user_id": uid, "page": book.RepayHistory(uid, q), "host": config.HostAccountBook})
	   })
	   v1.GET("/assets/uta/risk", func(c *gin.Context) {
		  uid, err := callerid.Require(c, opt)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, gin.H{"user_id": uid, "risk": book.RiskUnits(uid), "host": config.HostAccountBook})
	   })
	   v1.GET("/assets/uta/cross-margin", func(c *gin.Context) {
		   uid, err := callerid.Require(c, opt)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, gin.H{"user_id": uid, "account": book.CrossMarginView(uid, c.Query("currency")), "host": config.HostAccountBook})
	  })
	  v1.GET("/assets/uta/isolated-margin", func(c *gin.Context) {
		   uid,
			   err := callerid.Require(c, opt)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, gin.H{"user_id": uid, "account": book.IsolatedMargin(uid, c.Query("symbol")), "host": config.HostAccountBook})
	  })
	  v1.GET("/assets/uta/health", func(c *gin.Context) {
		   c.JSON(200, gin.H{"health": book.BorrowReadHealth(), "host": config.HostAccountBook})
	   })
   }
