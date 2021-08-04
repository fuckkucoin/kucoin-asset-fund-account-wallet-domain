// @FileName: travel_l4.go
// @Summary: 公开旅行规则健康分、案件-出金反查。只读；按 caller 过滤案件。他人出金单不得回案件。
// @Author: Orin
// @Created: 2021.08.03

package kcfund

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/travelvasp"
	"github.com/gin-gonic/gin"
)

  func (a *API) getTravelHealth(c *gin.Context) {
	   if _, okU := a.requireUser(c); !okU {
		   return
	  }
	  desk := a.travelDesk()
	  if desk == nil {
		   fail(c, 503, travelvasp.ErrUnavailable.Error())
		   return
	   }
	  ok(c, desk.InspectPeerHealth(c.Query("code")))
  }

   func (a *API) getTravelByWithdraw(c *gin.Context) {
	   uid, okU := a.requireUser(c)
	  if !okU {
		  return
	  }
	  desk := a.travelDesk()
	   if desk == nil {
		  fail(c,
			  503, travelvasp.ErrUnavailable.Error())
		   return
	  }
	  wd := strings.TrimSpace(c.Query("withdraw_id"))
	  if wd == "" {
		   wd = strings.TrimSpace(c.Query("withdrawalId"))
	  }
	   got, err := desk.CaseByWithdraw(c.Request.Context(), wd)
	   if err != nil {
		   fail(c, 404, err.Error())
		  return
	   }
	   if got.UserID != uid {
		   fail(c, 404, travelvasp.ErrCase.Error())
		  return
	  }
	  ok(c, got)
  }
