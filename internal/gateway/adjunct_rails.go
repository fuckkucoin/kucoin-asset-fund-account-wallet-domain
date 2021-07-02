// 写路径仍走 fiatadd 签名确认与 p2pgate 冻结；本文件仅聚合 query-gateway 路由。
// 契约参考 docs.assets.kucoin.local/funding/add-funds 与 funding/p2p-holds。

// Author: Axel
// Date: 2021.06.29

package gateway

import (
	"strconv"
	"strings"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/fiatadd"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/notify"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/gin-gonic/gin"
)

   func registerAdjunctRails(v1 *gin.RouterGroup, rt *app.Runtime) {
	  orch := v1.Group("")
	   orch.Use(gatewayProductOrchestrationMiddleware(rt))
	  registerFiatRails(orch, rt)
	   registerP2PRails(orch, rt)
	   registerEarnRails(orch, rt)
	   registerMarginRails(orch, rt)
	   registerTravelRails(orch, rt)
	  registerUTARails(orch, rt)
	   registerFundsurfRails(v1, rt)
	   registerSubKeyRails(v1, rt)
	   registerMarksRails(v1, rt)
	   registerFeedRails(v1, rt)
  }

   func registerFiatRails(v1 *gin.RouterGroup, rt *app.Runtime) {
	   book := fiatadd.FromRuntime(rt)
	   opt := callerid.FromRuntime(rt.Mem, "", "", "")
	  if rt != nil && rt.Cfg != nil {
		  opt = callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile)
	   }
	  v1.GET("/assets/fiat/vendors", func(c *gin.Context) {
		  c.JSON(200, gin.H{
			   "vendors": []string{"moonpay", "banxa", "circle"},
			   "bases":   []string{config.VendorMoonPay, config.VendorBanxa, config.VendorCircle},
			   "host": config.HostFiatOnramp,
		  })
	  })
	   v1.GET("/assets/fiat/sessions", func(c *gin.Context) {
		  uid, err := callerid.Require(c, opt)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   list, err := book.ListForUser(c.Request.Context(), uid, 20)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, gin.H{"user_id": uid, "sessions": list, "host": config.HostFiatOnramp})
	   })
	   v1.POST("/assets/fiat/sessions", func(c *gin.Context) {
		   var body struct {
			   Asset  string `json:"asset"`
			  Amount string `json:"amount"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   uid, err := callerid.Require(c, opt)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   sess, err := book.Open(c.Request.Context(), uid, body.Asset, body.Amount)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, gin.H{"session": sess, "host": config.HostFiatOnramp})
	   })
	  v1.GET("/assets/fiat/sessions/:id", func(c *gin.Context) {
		  uid, err := callerid.Require(c, opt)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   sess, err := book.Get(c.Request.Context(), uid, c.Param("id"))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, gin.H{"session": sess})
	  })
	  v1.POST("/assets/fiat/sessions/:id/confirm", func(c *gin.Context) {
		  var body struct {
			  Signature string `json:"signature"`
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
		   sess, err := book.Confirm(c.Request.Context(), uid, c.Param("id"), body.Signature)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  if rt != nil {
			   _ = notify.FromConfig(rt.KV, rt.Cfg).Publish(c.Request.Context(), "fiat:"+sess.ID, notify.KindFiatCredited, sess.ID)
		  }
		   c.JSON(200, gin.H{"session": sess})
	  })
   }

   func registerMarksRails(v1 *gin.RouterGroup, rt *app.Runtime) {
	   v1.GET("/assets/marks", func(c *gin.Context) {
		  user,
			   err := validate.UserID(callerid.Resolve(c, idOpt(rt)))
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   quote := strings.ToUpper(strings.TrimSpace(c.DefaultQuery("quote", "USDT")))
		  raw := strings.TrimSpace(c.Query("assets"))
		   assets := []string{}
		   if raw != "" {
			  for _, p := range strings.Split(raw, ",") {
				  p = strings.ToUpper(strings.TrimSpace(p))
				   if p != "" {
					   assets = append(assets, p)
				  }
			   }
		   }
		   if len(assets) == 0 {
			  assets = []string{"BTC", "ETH", "USDT", "KCS"}
		   }
		  marks := newFxBook(rt.KV, convertQuoteURL(rt))
		   items := make([]gin.H, 0, len(assets))
		  incomplete := false
		   for _, asset := range assets {
			  rate,
				  err := quoteRate(c.Request.Context(), marks, asset, quote)
			  if err != nil || !rate.IsPositive() {
				  incomplete = true
				  continue
			  }
			  items = append(items, gin.H{"asset": asset, "quote": quote, "rate": rate.String()})
		  }
		   c.JSON(200, gin.H{
			  "user_id": user, "quote": quote, "marks": items, "incomplete": incomplete,
			   "host": "query-gateway.kucoin.net",
		   })
	   })
  }

   func registerFeedRails(v1 *gin.RouterGroup, rt *app.Runtime) {
	   v1.GET("/assets/feed", func(c *gin.Context) {
		   user, err := validate.UserID(callerid.Resolve(c, idOpt(rt)))
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  limit := 30
		  if n, err := strconv.Atoi(strings.TrimSpace(c.Query("limit"))); err == nil && n > 0 && n <= 50 {
			  limit = n
		   }
		  if rt == nil || rt.Ledger == nil {
			   c.JSON(200, gin.H{"user_id": user, "items": []any{}})
			   return
		  }
		   list,
			   err := rt.Ledger.List(c.Request.Context(), user, limit)
		   if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		  }
		  items := make([]any, 0, len(list))
		  for _, tr := range list {
			   if tr == nil || !fundingFeedDir(tr.Direction) {
				  continue
			  }
			   items = append(items, ledger.ViewTransfer(tr, ""))
		   }
		   c.JSON(200, gin.H{"user_id": user, "items": items, "host": "query-gateway.kucoin.net"})
	  })
  }

   func fundingFeedDir(dir ledger.Direction) bool {
	   switch dir {
	  case ledger.WalletDeposit, ledger.WalletWithdraw, ledger.SpotSend,
		   ledger.FiatOnramp, ledger.FiatOfframp, ledger.P2PCredit, ledger.P2PDebit:
		  return true
	  default:
		  return false
	   }
  }
