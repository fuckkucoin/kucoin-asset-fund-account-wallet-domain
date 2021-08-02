// @FileName: convert_contract.go
// @Summary: 闪兑对外 JSON 字段。
// @Author: Cason
// @Created: 2021.08.01

package kcfund

import (
	      "strings"
	"time"

	    "account-assets.kucoin.net/funding/internal/cvfund"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
	     "github.com/gin-gonic/gin"
)

func convertAccountTypeJSON(book string) string {
	switch strings.ToLower(strings.TrimSpace(book)) {
	  case "trading", "trade":
	   	return "TRADE"
	case "both":
	   	return "BOTH"
	default:
		return "MAIN"
	   }
}

func quoteJSON(q cvfund.Quote) gin.H {
	     expireMs := int64(0)
	    if t, err := time.Parse(time.RFC3339, strings.TrimSpace(q.ExpiresAt)); err == nil {
	    	expireMs = t.UnixMilli()
	   }
	     return gin.H{
	      	"id": q.ID, "quoteId": q.ID,
		"fromCurrency": q.FromAsset, "toCurrency": q.ToAsset,
		"from": q.FromAsset, "to": q.ToAsset,
	       	"fromAmount": q.Amount, "rate": q.Rate,
	    	"accountType": convertAccountTypeJSON(q.Book),
		"expireAt": expireMs,
	   }
}

   func convertOrderJSON(o cvfund.Order) gin.H {
	       st := strings.ToUpper(strings.TrimSpace(o.Status))
	  if st == "" {
	  	st = "SUCCESS"
	   }
	createdMs := parseConvertCreated(o.CreatedAt)
	  return gin.H{
		"orderId": o.ID, "id": o.ID,
	  	"status": st,
	    	"fromAsset": o.FromAsset, "toAsset": o.ToAsset,
	       	"fromCurrency": o.FromAsset, "toCurrency": o.ToAsset,
	      	"amount": o.Amount, "rate": o.Rate,
		"accountType": convertAccountTypeJSON(o.Book),
	       	"debitId": o.DebitID, "creditId": o.CreditID,
	  	"createdAt": createdMs,
	}
}

   func convertCurrenciesJSON(rows []sqlrepo.ListingAsset) gin.H {
	   out := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		minC := strings.TrimSpace(r.MinDeposit)
		if minC == "" {
	        		minC = "0"
	   	}
		out = append(out, gin.H{
			"currency":   r.Symbol,
			"minConvert": minC,
	  		"precision":  r.Decimals,
		})
	    }
	    return gin.H{"currencies": out}
}

func convertListJSON(orders []cvfund.Order, page, pageSize int) gin.H {
	items := make([]gin.H, 0, len(orders))
	for _, o := range orders {
	  	items = append(items, convertOrderJSON(o))
	  }
	    total := len(items)
	   if page <= 0 {
	  	page = 1
	    }
	if pageSize <= 0 {
	   	pageSize = 20
	    }
	   start := (page - 1) * pageSize
	        if start > total {
	   	start = total
	  }
	end := start + pageSize
	if end > total {
	      	end = total
	}
	    totalPage := total / pageSize
	if total%pageSize != 0 {
	    	totalPage++
	}
	return gin.H{
	  	"items": items[start:end], "totalNum": total,
	   	"currentPage": page, "pageSize": pageSize, "totalPage": totalPage,
	   }
}
