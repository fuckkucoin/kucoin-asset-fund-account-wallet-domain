// 功能：UTA 杠杆/借还读面：borrow info、负债、还本历史、风险、全仓/逐仓 margin 读。
// 写路径（借入/强平）不在本文件登记。

// 作者：Kian
// 日期：2021.08.03

package kcfund

import (
	  "net/http"
	   "strconv"
	  "strings"
	"account-assets.kucoin.net/funding/internal/utabook"
	    "github.com/gin-gonic/gin"
	    "github.com/shopspring/decimal"
)

 func (a *API) registerUTABorrowRead(g *gin.RouterGroup) {
	   g.GET("/ua/v1/margin/borrow/info", a.utaBorrowInfo)
	  g.GET("/ua/v1/margin/borrow/liability", a.utaBorrowLiability)
	  g.GET("/ua/v1/margin/repay/history", a.utaRepayHistory)
	  g.GET("/ua/v1/account/risk-rate", a.utaRiskRate)
	    g.GET("/ua/v1/unified/account/risk", a.utaRiskRate)
	      g.GET("/ua/v1/margin/cross/account", a.utaCrossMargin)
	       g.GET("/ua/v1/margin/isolated/account", a.utaIsolatedMargin)
	      g.GET("/ua/v1/margin/account", a.utaMarginRead)
	   g.GET("/ua/v1/account/interest-history-paged", a.utaInterestHistoryPaged)
	       g.GET("/ua/v1/account/collateral-catalog", a.utaCollateralCatalog)
  }

  func (a *API) utaBorrowInfo(c *gin.Context) {
	     if _, err := a.utaBook().RequireInterestLimits(c.Query("currency")); err != nil {
		  fail(c, 503, err.Error())
	    	  return
	      }
	      ok(c, a.utaBook().BorrowInfo(c.Query("currency")))
   }

   func (a *API) utaBorrowLiability(c *gin.Context) {
	   if a.forwardMeshUTA(c, http.MethodGet, "/api/ua/v1/margin/borrow/liability", nil) {
		   return
	   }
	  uid, okU := a.requireUser(c)
	  if !okU {
		   return
	     }
	       book := a.utaBook()
	  if c.Query("summary") == "1" {
		  ok(c, book.LiabilitySummary(uid))
		  return
	     }
	   ok(c, book.Liabilities(uid, c.Query("currency"), c.Query("marginMode"), c.Query("symbol")))
  }

   func (a *API) utaRepayHistory(c *gin.Context) {
	  if a.forwardMeshUTA(c, http.MethodGet, "/api/ua/v1/margin/repay/history", nil) {
		  return
	       }
	      uid, okU := a.requireUser(c)
	      if !okU {
	    	  return
	      }
	      page, _ := strconv.Atoi(strings.TrimSpace(c.Query("currentPage")))
	   if page <= 0 {
		  page, _ = strconv.Atoi(strings.TrimSpace(c.Query("page")))
	  }
	     pageSize, _ := strconv.Atoi(strings.TrimSpace(c.Query("pageSize")))
	     startMs, _ := strconv.ParseInt(strings.TrimSpace(c.Query("startAt")), 10, 64)
	   endMs, _ := strconv.ParseInt(strings.TrimSpace(c.Query("endAt")), 10, 64)
	       q := utabook.RepayQuery{
	   	   Currency: c.Query("currency"), MarginMode: c.Query("marginMode"), Symbol: c.Query("symbol"),
	  	   StartMs: startMs, EndMs: endMs, Page: page, PageSize: pageSize,
	      }
	  ok(c, a.utaBook().RepayHistory(uid, q))
   }

 func (a *API) utaRiskRate(c *gin.Context) {
	    if a.forwardMeshUTA(c, http.MethodGet, "/api/ua/v1/account/risk-rate", nil) {
	  	  return
	      }
	  uid, okU := a.requireUser(c)
	   if !okU {
		   return
	       }
	   book := a.utaBook()
	   if c.Query("recompute") == "1" {
	  	  eq := strings.TrimSpace(c.Query("equity"))
		  if eq == "" {
	  		  eq = a.quoteEquity(c, uid, c.Query("currency"))
		  }
	  	  view, err := book.RecomputeRiskFromLiabilities(uid, eq)
		  if err != nil {
	   		  fail(c, 503, err.Error())
			  return
		  }
		  ok(c, view)
	    	  return
	  }
	   if c.Query("units") == "1" {
	   	  ok(c, book.RiskUnits(uid))
		   return
	   }
	      stored := book.Risk(uid)
	   if stored.UpdatedAt != 0 {
		  ok(c, stored)
	  	  return
	     }
	       cur := c.Query("currency")
	   ok(c, assessedRisk(uid, a.quoteEquity(c, uid, cur), book.LiabilityByCurrency(uid, cur)))
  }

  func assessedRisk(uid, equity, liability string) utabook.RiskView {
	      view := utabook.RiskView{
		  UserID: uid, TotalEquity: equity, TotalLiability: liability,
		  MarginLevel: "0", CrossDebtRatio: "0",
	  }
	    e, errE := decimal.NewFromString(strings.TrimSpace(equity))
	  l, errL := decimal.NewFromString(strings.TrimSpace(liability))
	    if errE != nil || errL != nil || !e.IsPositive() || !l.IsPositive() {
		  return view
	     }
	  view.CrossDebtRatio = l.Div(e).String()
	     view.MarginLevel = e.Div(l).String()
	     return view
  }

   func (a *API) utaCrossMargin(c *gin.Context) {
	    if a.forwardMeshUTA(c, http.MethodGet, "/api/ua/v1/margin/cross/account", nil) {
	  	   return
	       }
	       uid, okU := a.requireUser(c)
	       if !okU {
		   return
	       }
	   ok(c, a.utaBook().CrossMarginView(uid, c.Query("currency")))
   }

  func (a *API) utaIsolatedMargin(c *gin.Context) {
	      if a.forwardMeshUTA(c, http.MethodGet, "/api/ua/v1/margin/isolated/account", nil) {
	   	  return
	  }
	     uid, okU := a.requireUser(c)
	  if !okU {
	  	   return
	  }
	   ok(c, a.utaBook().IsolatedMargin(uid, c.Query("symbol")))
   }

 func (a *API) utaMarginRead(c *gin.Context) {
	  if a.forwardMeshUTA(c, http.MethodGet, "/api/ua/v1/margin/account", nil) {
	    	   return
	   }
	   uid, okU := a.requireUser(c)
	   if !okU {
	  	  return
	    }
	     ok(c, a.utaBook().MarginRead(uid, c.Query("marginMode"), c.Query("symbol"), c.Query("currency")))
   }

  func (a *API) utaInterestHistoryPaged(c *gin.Context) {
	  if a.forwardMeshUTA(c, http.MethodGet, "/api/ua/v1/account/interest-history-paged", nil) {
	   	  return
	     }
	      uid, okU := a.requireUser(c)
	      if !okU {
	   	   return
	  }
	  page, _ := strconv.Atoi(strings.TrimSpace(c.DefaultQuery("page", "1")))
	     pageSize, _ := strconv.Atoi(strings.TrimSpace(c.DefaultQuery("pageSize", "50")))
	  ok(c, a.utaBook().InterestHistoryPaged(uid, c.Query("currency"), page, pageSize))
  }
