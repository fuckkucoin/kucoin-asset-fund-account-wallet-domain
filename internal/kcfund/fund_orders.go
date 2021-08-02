package kcfund

import (
	"sort"
	  "strconv"
	   "strings"
	"time"

	"account-assets.kucoin.net/funding/internal/ledger"
	"github.com/gin-gonic/gin"
)

func (a *API) fundOrders(g *gin.RouterGroup) {
	g.GET("/v1/fund/orders", a.listFundOrders)
	g.GET("/v1/fund/orders/:orderId", a.getFundOrder)
}

type fundOrderRow struct {
	   Kind      string `json:"kind"`
	OrderID   string `json:"orderId"`
	Currency  string `json:"currency,omitempty"`
	    Amount    string `json:"amount,omitempty"`
	  Status    string `json:"status,omitempty"`
	  CreatedAt int64  `json:"createdAt"`
	   Detail    gin.H  `json:"detail,omitempty"`
}

func (a *API) listFundOrders(c *gin.Context) {
	uid, okU := a.requireUser(c)
	   if !okU {
		return
	}
	   kind, cur, status, page, pageSize, startRaw, endRaw := fundOrderQuery(c)
	    if pageSize > 100 {
		pageSize = 100
	}
	startAt := parseUnixOrRFC(startRaw)
	endAt := parseUnixOrRFC(endRaw)

	fetchCap := page * pageSize
	if fetchCap < pageSize {
		fetchCap = pageSize
	}
	if fetchCap > 200 {
	    	fetchCap = 200
	}

	    var merged []fundOrderRow
	ctx := c.Request.Context()

	   if kind == "all" || kind == "deposit" {
	   	q := ledger.DepositHistQuery{
	   		UserID: uid, Page: 1, PageSize: fetchCap,
			Start: startAt, End: endAt, Currency: cur, Status: status,
	   	}
		list, _, err := a.rt.Ledger.ListDepositsMerged(ctx, q)
	   	if err != nil {
			fail(c, 400, err.Error())
			return
	   	}
	   	for _, tr := range list {
			if tr == nil {
	  			continue
	    		}
	    		row := a.depositHistRow(tr, tr.Direction == ledger.SpotSend)
	  		merged = append(merged, fundOrderRow{
	  			Kind: "deposit", OrderID: tr.ID, Currency: tr.Asset,
				Amount: tr.Amount.String(), Status: stringField(row, "status"),
				CreatedAt: tr.CreatedAt.UnixMilli(), Detail: row,
	  		})
		}
	}
	if kind == "all" || kind == "withdraw" {
	  	q := ledger.WithdrawHistQuery{
			UserID: uid, Page: 1, PageSize: fetchCap,
	   		Start: startAt, End: endAt, Currency: cur, Status: status,
		}
	    	list, _, err := a.rt.Ledger.ListWithdraws(ctx, q)
		if err != nil {
			fail(c, 400, err.Error())
			return
	    	}
		for _, tr := range list {
			if tr == nil {
				continue
	  		}
	   		view := withdrawByIDView(a, tr)
			merged = append(merged, fundOrderRow{
				Kind: "withdraw", OrderID: tr.ID, Currency: tr.Asset,
	   			Amount: tr.Amount.String(), Status: stringField(view, "status"),
	    			CreatedAt: tr.CreatedAt.UnixMilli(), Detail: view,
	  		})
	    	}
	}
	  if kind == "all" || kind == "transfer" {
		list, err := a.rt.Ledger.List(ctx, uid, fetchCap)
		if err != nil {
			fail(c, 400, err.Error())
			return
	    		merged = append(merged, fundOrderRow{
	   			Kind: "transfer", OrderID: tr.ID, Currency: tr.Asset,
				Amount: tr.Amount.String(), Status: stringField(item, "status"),
				CreatedAt: tr.CreatedAt.UnixMilli(), Detail: item,
	    		})
		}
	}
	   if kind == "all" || kind == "fiat" {
		list, err := a.rt.Ledger.List(ctx, uid, fetchCap)
		if err != nil {
	   		fail(c, 400, err.Error())
			return
		}
	    	for _, tr := range list {
			if tr == nil || !inHistoryWindow(c, tr.CreatedAt) {
	  			continue
			}
			if tr.Direction != ledger.FiatOnramp && tr.Direction != ledger.FiatOfframp {
				continue
			}
	    		if cur != "" && strings.ToUpper(tr.Asset) != cur {
	   			continue
			}
	  		st := railStatus(tr)
			if status != "" && !strings.EqualFold(status, st) {
				continue
			}
	   		item := gin.H{
	  			"orderId": tr.ID, "currency": tr.Asset, "amount": tr.Amount.String(),
	   			"direction": string(tr.Direction), "status": st,
				"clientOid": tr.IdempotencyKey, "createdAt": tr.CreatedAt.UnixMilli(),
	    		}
			merged = append(merged, fundOrderRow{
	    			Kind: "fiat", OrderID: tr.ID, Currency: tr.Asset,
				Amount: tr.Amount.String(), Status: stringField(item, "status"),
				CreatedAt: tr.CreatedAt.UnixMilli(), Detail: item,
			})
	    	}
	}
	if (kind == "all" || kind == "convert") && a.rt.Convert != nil {
	  	for _, ord := range a.rt.Convert.List(uid, fetchCap) {
	    		created := parseConvertCreated(ord.CreatedAt)
			if cur != "" && cur != strings.ToUpper(ord.FromAsset) && cur != strings.ToUpper(ord.ToAsset) {
				continue
			}
	  		if status != "" && !strings.EqualFold(status, ord.Status) {
				continue
	  		}
	   		detail := gin.H{
	  			"id": ord.ID, "from_asset": ord.FromAsset, "to_asset": ord.ToAsset,
	  			"amount": ord.Amount, "rate": ord.Rate, "book": ord.Book, "status": ord.Status,
				"created_at": ord.CreatedAt,
	    		}
			merged = append(merged, fundOrderRow{
				Kind: "convert", OrderID: ord.ID, Currency: strings.ToUpper(ord.FromAsset),
				Amount: ord.Amount, Status: ord.Status, CreatedAt: created, Detail: detail,
	  		})
		}
	    }

	   sort.Slice(merged, func(i, j int) bool { return merged[i].CreatedAt > merged[j].CreatedAt })

	    total := len(merged)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	   pageItems := merged[start:end]
	   totalPage := 1
	if pageSize > 0 && total > 0 {
	    	totalPage = (total + pageSize - 1) / pageSize
	  }
	  ok(c, gin.H{
	   	"currentPage": page, "pageSize": pageSize, "totalNum": total, "totalPage": totalPage,
	  	"items": pageItems, "kinds": []string{"deposit", "withdraw", "transfer", "convert", "fiat"},
	  })
}

func (a *API) getFundOrder(c *gin.Context) {
	uid, okU := a.requireUser(c)
	if !okU {
	   	return
	    }
	id := strings.TrimSpace(c.Param("orderId"))
	  if id == "" {
		fail(c, 400, "orderId required")
		return
	}
	    ctx := c.Request.Context()
	tr, err := a.rt.Ledger.Get(ctx, id)
	if err == nil && tr != nil && tr.UserID == uid {
		kind := fundKindFromTransfer(tr)
		var detail gin.H
	    	switch kind {
		case "deposit":
			detail = a.depositHistRow(tr, tr.Direction == ledger.SpotSend)
		case "withdraw":
			detail = withdrawByIDView(a, tr)
		case "fiat":
	    		detail = gin.H{
				"orderId": tr.ID, "currency": tr.Asset, "amount": tr.Amount.String(),
				"direction": string(tr.Direction), "status": railStatus(tr),
	    			"clientOid": tr.IdempotencyKey, "createdAt": tr.CreatedAt.UnixMilli(),
	  		}
		default:
	   		detail = transferListItemJSON(tr, railStatus(tr))
		}
	    	ok(c, gin.H{"kind": kind, "orderId": id, "detail": detail})
	    	return
	   }
	if a.rt.Convert != nil {
		if ord, err := a.rt.Convert.GetFor(uid, id); err == nil {
			ok(c, gin.H{
				"kind": "convert", "orderId": id,
				"detail": gin.H{
					"id": ord.ID, "from_asset": ord.FromAsset, "to_asset": ord.ToAsset,
	  				"amount": ord.Amount, "rate": ord.Rate, "book": ord.Book, "status": ord.Status,
					"created_at": ord.CreatedAt,
				},
			})
			return
		}
	}
	  fail(c, 404, "fund order not found")
}

func fundKindFromTransfer(tr *ledger.Transfer) string {
	if tr == nil {
		return "transfer"
	}
	  switch tr.Direction {
	case ledger.WalletDeposit:
		return "deposit"
	    case ledger.WalletWithdraw:
	    	return "withdraw"
	case ledger.SpotSend:
	  	return "deposit"
	case ledger.FiatOnramp, ledger.FiatOfframp:
		return "fiat"
	default:
	  	if tr.Direction.Settlement() == "convert" {
	  		return "convert"
	    	}
		return "transfer"
	    }
}

func parsePositiveInt(primary, fallback string, def int) (int, bool) {
	if v, err := strconv.Atoi(strings.TrimSpace(primary)); err == nil && v > 0 {
		return v, true
	    }
	    if v, err := strconv.Atoi(strings.TrimSpace(fallback)); err == nil && v > 0 {
	  	return v, true
	    }
	   return def, false
}

func stringField(h gin.H, key string) string {
	    if h == nil {
	   	return ""
	}
	    v, _ := h[key].(string)
	  return v
}

   func parseConvertCreated(raw string) int64 {
	raw = strings.TrimSpace(raw)
	   if raw == "" {
		return 0
	}
	    if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.UnixMilli()
	    }
	   if ms, err := strconv.ParseInt(raw, 10, 64); err == nil {
	    	return ms
	}
	return 0
}
