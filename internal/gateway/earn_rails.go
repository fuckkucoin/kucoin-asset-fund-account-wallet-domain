package gateway

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/earn"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

  func registerEarnRails(v1 *gin.RouterGroup, rt *app.Runtime) {
	   svc := earn.New(nil)
	   if rt != nil {
		   svc = earn.New(rt.Ledger).BindKV(rt.KV).BindHolds(holdcat.New(rt.KV))
	  }
	   opt := callerid.FromRuntime(false, "", "", "")
	  if rt != nil && rt.Cfg != nil {
		  opt = callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile)
	  }
	  v1.GET("/assets/earn/holds", func(c *gin.Context) {
		   uid, err := callerid.Require(c, opt)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   if _, err := validate.UserID(uid); err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, gin.H{
			   "user_id": uid, "positions": svc.List(uid),
			   "host": config.HostAccountBook,
		  })
	  })
	   v1.GET("/assets/earn/summary", func(c *gin.Context) {
		  uid, err := callerid.Require(c, opt)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   if _, err := validate.UserID(uid); err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   byAsset := map[string]string{}
		  openN := 0
		  for _, p := range svc.List(uid) {
			   if p.Status != earn.StatusOpen && p.Status != earn.StatusLocked {
				   continue
			  }
			  openN++
			   asset := strings.ToUpper(strings.TrimSpace(p.Asset))
			  principal, err := decimal.NewFromString(strings.TrimSpace(p.Principal))
			  if err != nil || !principal.IsPositive() {
				  continue
			  }
			  prev, _ := decimal.NewFromString(byAsset[asset])
			  byAsset[asset] = prev.Add(principal).String()
		  }
		  c.JSON(200, gin.H{
			  "user_id": uid, "open_positions": openN,
			  "principal_by_asset": byAsset, "host": config.HostAccountBook,
		   })
	   })
  }
