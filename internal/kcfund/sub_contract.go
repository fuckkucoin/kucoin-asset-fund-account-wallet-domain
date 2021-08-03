// @FileName: sub_contract.go
// @Summary: 子账户 REST JSON。
// @Author: Cason
// @Created: 2021.08.02

package kcfund

import (
	        "strings"
	    "time"

	  "account-assets.kucoin.net/funding/internal/subacct"
	  "github.com/gin-gonic/gin"
)

func privilegeStrings(privs []subacct.Privilege) []string {
	       out := make([]string, 0, len(privs))
	    for _, p := range privs {
	        	out = append(out, string(p))
	}
	            return out
}

func (a *API) subTransferable(c *gin.Context, parentID, childID string) bool {
	   if a == nil || a.rt == nil || a.rt.FundSurf == nil || c == nil {
	      	return false
	    }
	  return a.surf().SubXferPerm(c.Request.Context(), parentID, childID)
}

func (a *API) subAccountJSON(c *gin.Context, ch subacct.Child, remarks string) gin.H {
	transferable := a.subTransferable(c, ch.ParentID, ch.ID)
	        h := gin.H{
	           	"subUserId":    ch.ID,
	        	"subName":      ch.Label,
	    	"remarks":      firstNonEmpty(remarks, ch.Label),
	  	"status":       string(ch.Status),
		"privileges":   privilegeStrings(ch.Privileges),
	        	"transferable": transferable,
	    	"userId":       ch.UserID,
	    	"uid":          ch.ID,
	}
	        if !ch.OpenedAt.IsZero() {
	      	h["createdAt"] = ch.OpenedAt.UTC().Format(time.RFC3339)
	}
	        if ch.Status == subacct.StatusFrozen && strings.TrimSpace(ch.Reason) != "" {
	            	h["freezeReason"] = ch.Reason
	}
	return h
}

func (a *API) subCreateJSON(c *gin.Context, ch subacct.Child, access string) gin.H {
	         h := a.subAccountJSON(c, ch, "")
	   h["access"] = access
	return h
}

   func subBalanceItemsJSON(rows []subacct.BookBal) []gin.H {
	    items := make([]gin.H, 0, len(rows))
	        for _, r := range rows {
		items = append(items, gin.H{
			"currency":  r.Asset,
			"available": r.Available,
	  		"holds":     r.Frozen,
	          	})
	}
	        return items
}
