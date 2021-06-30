// 资金页冻结与网络目录读面；与 /api/v1/currencies/networks 同源 fundsurf。

// Author: Axel
// Date: 2021.06.29

package gateway

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/fundsurf"
	"account-assets.kucoin.net/funding/internal/mw"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

  func registerFundsurfAssetsRead(v1 *gin.RouterGroup, rt *app.Runtime) {
	   if v1 == nil || rt == nil {
		   return
	   }
	   h := fundsurf.NewHTTP(rt.KV, rt.Ledger, rt.Cfg, rt.Mem)
	   if rt.FundSurf != nil {
		   h.Surf = rt.FundSurf
	  }
	  fundsurf.RegisterAssetsRead(v1,
		   h)
  }

   func registerFundsurfRails(v1 *gin.RouterGroup, rt *app.Runtime) {
	  h := fundsurf.NewHTTP(rt.KV, rt.Ledger, rt.Cfg, rt != nil && rt.Mem)
	  if rt != nil && rt.FundSurf != nil {
		  h.Surf = rt.FundSurf
	  }
	  opt := callerid.FromRuntime(false, "", "", "")
	   if rt != nil && rt.Cfg != nil {
		   opt = callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile)
	  }
	  surf := h.Surf
	  v1.GET("/assets/frozen/summary", func(c *gin.Context) {
		   uid, err := callerid.Require(c, opt)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  rows,
			   err := surf.Frozen(c.Request.Context(), uid)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   byAsset := map[string]string{}
		  n := 0
		  for _, r := range rows {
			  amt, err := decimal.NewFromString(strings.TrimSpace(r.Frozen))
			   if err != nil || !amt.IsPositive() {
				  continue
			   }
			   n++
			   asset := strings.ToUpper(strings.TrimSpace(r.Asset))
			   prev, _ := decimal.NewFromString(byAsset[asset])
			  byAsset[asset] = prev.Add(amt).String()
		  }
		  c.JSON(200, gin.H{
			  "user_id": uid, "line_count": n,
			   "frozen_by_asset": byAsset, "host": config.HostFundSurface,
		   })
	   })
  }
