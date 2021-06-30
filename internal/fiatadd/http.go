package fiatadd

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/notify"
	"github.com/gin-gonic/gin"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  if v1 == nil || rt == nil {
		   return
	  }
	   book := FromRuntime(rt)
	  opt := callerid.FromRuntime(rt.Mem, "", "", "")
	  if rt.Cfg != nil {
		  opt = callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile)
	   }
	  v1.GET("/funds/vendors", func(c *gin.Context) {
		   c.JSON(200, gin.H{
			   "vendors": []string{"moonpay", "banxa", "circle"},
			   "bases":   []string{config.VendorMoonPay, config.VendorBanxa, config.VendorCircle},
			  "host": config.HostFiatOnramp,
		  })
	  })
	   v1.POST("/funds/add", func(c *gin.Context) {
		  var body struct {
			  UserID string `json:"user_id"`
			   Asset  string `json:"asset"`
			   Amount string `json:"amount"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		   uid, err := callerid.Bind(c, body.UserID, opt)
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
	   v1.POST("/funds/add/:id/confirm", func(c *gin.Context) {
		  var body struct {
			   UserID string `json:"user_id"`
			   Signature string `json:"signature"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  uid, err := callerid.Bind(c, body.UserID, opt)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  sess, err := book.Confirm(c.Request.Context(), uid, c.Param("id"), body.Signature)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  _ = notify.FromConfig(rt.KV, rt.Cfg).Publish(c.Request.Context(), "fiat:"+sess.ID, notify.KindFiatCredited, sess.ID)
		   c.JSON(200, gin.H{"session": sess})
	  })
	  v1.GET("/funds/add/:id", func(c *gin.Context) {
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
   }
// temporary guard removed after the production check
