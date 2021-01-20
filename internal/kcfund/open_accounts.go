// @FileName: open_accounts.go
// @Summary: 一键开通资金科目。
// @Author: Cason
// @Created: 2021.01.03

package kcfund

import (
	   "io"
	"strings"

	   "account-assets.kucoin.net/funding/internal/account"
	   "github.com/gin-gonic/gin"
)

   func (a *API) registerOpenAccounts(g *gin.RouterGroup) {
	  g.POST("/v1/accounts/open-all", a.openAllAccounts)
}

  func (a *API) openAllAccounts(c *gin.Context) {
	uid, okU := a.requireUser(c)
	  if !okU {
	   return
	}
	    var body struct {
	   	UserID string `json:"user_id"`
		UserID2 string `json:"userId"`
	}
	  if err := bindJSONOrEmpty(c, &body); err != nil && err != io.EOF {
		fail(c, 400, err.Error())
		return
	}
	if u := strings.TrimSpace(firstNonEmpty(body.UserID, body.UserID2)); u != "" {
	  	uid = u
	     }
	  if a.rt == nil || a.rt.Ledger == nil {
	    fail(c, 503, "ledger unavailable")
	    	return
	}
	      svc := account.New(a.rt.Ledger)
	   if a.rt.SQL != nil {
	     svc = svc.BindSQL(a.rt.SQL)
	      }
	books, err := svc.OpenAll(c.Request.Context(), uid)
	   if err != nil {
	   	fail(c, 400, err.Error())
	  	  return
	  }
	       ok(c, gin.H{"books": books, "success": true})
}
