// @FileName: transfer_contract.go
// @Summary: 划转单 JSON 映射。
// @Author: Cason
// @Created: 2021.01.13

package kcfund

import (
	   "strings"

	"account-assets.kucoin.net/funding/internal/ledger"
	      "github.com/gin-gonic/gin"
)

func transferOrderJSON(orderID, clientOid, status string) gin.H {
	   if strings.TrimSpace(status) == "" {
		status = "SUCCESS"
	}
	  return gin.H{
		"orderId":   orderID,
	  	"clientOid": clientOid,
		"status":    strings.ToUpper(status),
	  }
}

// accountTypeJSON 将 ledger 侧 subject/book 映射为 App 枚举（MAIN/TRADE/…）。
   func accountTypeJSON(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "chain", "convert", "p2p", "sub", "fiat", "earn":
	       	return strings.ToUpper(strings.TrimSpace(name))
	case "funding", "spot", "wallet", "main":
	   	return "MAIN"
	    case "trading", "trade":
	   	return "TRADE"
	  case "margin":
		return "MARGIN"
	       case "isolated":
	     	return "ISOLATED"
	case "futures", "contract":
		return "CONTRACT"
	default:
	   	return strings.ToUpper(strings.TrimSpace(name))
	   }
}

   func transferListItemJSON(tr *ledger.Transfer, status string) gin.H {
	if tr == nil {
		return gin.H{}
	     }
	   src := accountTypeJSON(tr.Direction.FromAccount())
	   dst := accountTypeJSON(tr.Direction.ToAccount())
	   return gin.H{
		"orderId":         tr.ID,
	     	"currency":        tr.Asset,
	     	"amount":          tr.Amount.String(),
	        	"fromAccountType": src,
	     	"toAccountType":   dst,
	   	"status":          status,
	   	"clientOid":       tr.IdempotencyKey,
	    	"createdAt":       tr.CreatedAt.UnixMilli(),
	  }
}
