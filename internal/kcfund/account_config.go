package kcfund

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/subacct"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

  func (a *API) registerAccountConfig(g *gin.RouterGroup) {
	   g.GET("/v2/account/info", a.getAccountInfoV2)
	   g.GET("/v1/account-summary", a.getAccountSummary)
	  g.GET("/v1/hf/accounts/type", a.getSpotHFAccountType)
	  g.GET("/v1/account-config", a.getAccountConfig)
	  g.GET("/v1/account-preference", a.getAccountPreference)
	  g.PUT("/v1/account-preference", a.putAccountPreference)
   }

  func (a *API) getAccountInfoV2(c *gin.Context) {
	   uid, okU := a.requireUser(c)
	  if !okU {
		   return
	  }
	   svc := a.subs()
	   p, err := svc.OpenParent(subacct.OpenParentReq{UserID: uid, Label: "primary"})
	  if err != nil {
		   fail(c, 400, err.Error())
		   return
	  }
	   kids := svc.ListChildren(p.ID)
	   ok(c, gin.H{
		   "userId": uid, "subQuantity": len(kids), "subQuantityMax": 100,
		  "level": 0, "spotSubQuantity": len(kids),
	   })
   }

// getAccountSummary 资金页总览读面；同币种余额合并后仅对展示币行求和（未接牌价服务时不跨币种折算）。
  func (a *API) getAccountSummary(c *gin.Context) {
	   uid, okU := a.requireUser(c)
	   if !okU {
		  return
	  }
	  rows, okB := a.booksOrFail(c)
	   if !okB {
		   return
	  }
	  quote := "USDT"
	   p, err := a.prefs().Get(c.Request.Context(), uid)
	   if err != nil {
		   failPref(c, err)
		   return
	  }
	  if p.DisplayCurrency != "" {
		  quote = p.DisplayCurrency
	   }
	   subN := 0
	   if svc := a.subs(); svc != nil {
		  if p, err := svc.OpenParent(subacct.OpenParentReq{UserID: uid, Label: "primary"}); err == nil {
			  subN = len(svc.ListChildren(p.ID))
		  }
	   }
	   total := decimal.Zero
	   for _, r := range rows {
		   amt, err := decimal.NewFromString(strings.TrimSpace(r.Balance))
		   if err != nil {
			   continue
		   }
		   if strings.EqualFold(r.Currency, quote) {
			   total = total.Add(amt)
		   }
	  }
	   ok(c, gin.H{
		   "userId": uid, "level": 0, "subQuantity": subN,
		   "quoteCurrency": quote, "totalBalance": total.String(),
		   "accountSummary": gin.H{"currencyCount": len(rows)},
	  })
   }

  func (a *API) getSpotHFAccountType(c *gin.Context) {
	  opened := a.hfOpened(c)
	  typ := ""
	  if opened {
		  typ = "trade_hf"
	  } else if a.hasKCBook(c, "trade") {
		  typ = "trade"
	  }
	   ok(c, gin.H{"type": typ, "hfOpened": opened})
   }
