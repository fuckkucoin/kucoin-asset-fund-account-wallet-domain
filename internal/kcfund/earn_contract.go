// @FileName: earn_contract.go
// @Summary: 理财读/写响应 JSON。
// @Author: Cason
// @Created: 2021.08.01

package kcfund

import (
	"fmt"
	   "sort"
	      "strings"
	"time"

	"account-assets.kucoin.net/funding/internal/earn"
	    "account-assets.kucoin.net/funding/pkg/money"
	  "github.com/gin-gonic/gin"
)

func earnProductJSON(p earn.Product) gin.H {
	    return gin.H{
	   	"id": p.ID, "asset": p.Asset, "category": p.Category,
	   	"dailyBps": p.DailyBPS, "aprDisplay": aprFromDailyBps(p.DailyBPS),
		"lockDays": p.LockDays,
	  	"minSubscribe": p.MinSubscribe, "maxPosition": p.MaxPosition,
	      	"earlyBps": p.EarlyBPS, "active": p.Active, "autoSubscribe": p.AutoSubscribe,
	    }
}

func aprFromDailyBps(bps int) string {
	  if bps <= 0 {
		return "0"
	  }
	    return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.4f", float64(bps)*365.0/10000.0), "0"), ".")
}

  func earnProductsJSON(products []earn.Product) gin.H {
	   out := make([]gin.H, 0, len(products))
	    for _, p := range products {
	  	out = append(out, earnProductJSON(p))
	    }
	return gin.H{"products": out}
}

  func earnPositionJSON(p earn.Position) gin.H {
	  h := gin.H{
	   	"id": p.ID, "orderId": p.ID,
		"productId": p.ProductID,
	      	"asset": p.Asset, "currency": p.Asset,
	  	"principal": p.Principal,
	  	"accrued":   earnOrZero(p.Accrued),
	      	"claimed":   earnOrZero(p.Claimed),
	   	"status":    string(p.Status),
		"autoSubscribe": p.AutoSubscribe,
	}
	  if strings.TrimSpace(p.Penalty) != "" {
		h["penalty"] = p.Penalty
	}
	    if !p.OpenedAt.IsZero() {
	    	h["openedAt"] = p.OpenedAt.UTC().Format(time.RFC3339)
	  }
	   if !p.UnlockAt.IsZero() {
	  	h["unlockAt"] = p.UnlockAt.UTC().Format(time.RFC3339)
	      }
	return h
}

func earnOrZero(s string) string {
	if strings.TrimSpace(s) == "" {
		return "0"
	}
	return s
}

func earnHoldAssetsJSON(positions []earn.Position) gin.H {
	  sums := map[string]money.Amount{}
	      for _, p := range positions {
		if p.Status == earn.StatusRedeemed || p.Status == earn.StatusClosed {
			continue
	   	}
	     	amt, err := p.PrincipalAmt()
		if err != nil || !amt.IsPositive() {
	   		continue
	      	}
		cur := strings.ToUpper(strings.TrimSpace(p.Asset))
	    	if cur == "" {
	       		continue
	    	}
		if prev, ok := sums[cur]; ok {
	       		sums[cur], _ = prev.Add(amt)
	    	} else {
	   		sums[cur] = amt
		}
	}
	        curs := make([]string, 0, len(sums))
	for c := range sums {
	   	curs = append(curs, c)
	}
	sort.Strings(curs)
	  items := make([]gin.H, 0, len(curs))
	for _, c := range curs {
	  	items = append(items, gin.H{
	   		"currency":    c,
	   		"holdAmount":  sums[c].String(),
	   	})
	    }
	  return gin.H{"items": items}
}

func earnRedeemEarlyJSON(p earn.Position, pen earn.Penalty) gin.H {
	    return gin.H{
	   	"position": earnPositionJSON(p),
	    	"penalty": gin.H{
	  		"amount": pen.Fee,
	    		"gross":  pen.Gross,
	   		"net":    pen.Net,
	  		"bps":    pen.BPS,
	       	},
	  }
}
