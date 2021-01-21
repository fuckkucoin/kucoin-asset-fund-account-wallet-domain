package kcfund

import (
	"errors"
	"strconv"
	  "strings"
	       "account-assets.kucoin.net/funding/internal/account"
	    "account-assets.kucoin.net/funding/internal/fundpref"
	"account-assets.kucoin.net/funding/internal/isobook"
	  "account-assets.kucoin.net/funding/internal/ledger"
	    "github.com/gin-gonic/gin"
	       "github.com/shopspring/decimal"
)

var (
	errCaller     = errors.New("caller user_id required")
	errDownstream = errors.New("downstream unavailable")
)

  func failQuery(c *gin.Context, err error) {
	    if errors.Is(err, errCaller) {
		fail(c, 401, err.Error())
	  	return
	}
	if errors.Is(err, errDownstream) {
		fail(c, 503, "downstream unavailable")
	   	return
	      }
	fail(c, 400, err.Error())
}

   func (a *API) accounts(g *gin.RouterGroup) {
	     a.registerOpenAccounts(g)
	       g.GET("/v1/accounts", a.getAccountList)
	   g.GET("/v1/accounts/ledgers", a.getLedgers)
	     g.GET("/v1/accounts/:accountId", a.getAccountDetail)
	  g.GET("/v1/hf/accounts/ledgers", a.getLedgers)
	   g.GET("/v1/hf/margin/account/ledgers", a.getLedgers)
	    g.GET("/v1/hf/accounts/opened", a.getSpotHFOpened)
	   g.GET("/v1/transaction-history", a.getFuturesLedgers)
	       g.GET("/v1/account-overview", a.getFuturesOverview)
	      a.classicMarginReads(g)
  }

type kcAccount struct {
	ID string `json:"id"`
	Currency string `json:"currency"`
	  Type      string `json:"type"`
	Balance   string `json:"balance"`
	  Available string `json:"available"`
	  Holds string `json:"holds"`
}

  func (a *API) booksOrFail(c *gin.Context) ([]kcAccount, bool) {
	       out, err := a.listBooks(c)
	     if err != nil {
	    	   failQuery(c, err)
	   	  return nil, false
	        }
	          return out, true
   }

  func (a *API) listBooks(c *gin.Context) ([]kcAccount, error) {
	       uid := a.user(c)
	      if strings.TrimSpace(uid) == "" {
		   return nil, errCaller
	  }
	  meshRows, hit, err := a.tryMeshAccounts(c, uid)
	      if err != nil {
	     	  return nil, err
	      }
	  if hit {
	    	  cur := strings.ToUpper(strings.TrimSpace(c.Query("currency")))
	   	  typ := strings.ToLower(strings.TrimSpace(c.Query("type")))
	     	   out := make([]kcAccount, 0, len(meshRows))
	  	  for _, r := range meshRows {
			   if !matchType(typ, r.Type) {
				  continue
	    		   }
			   if cur != "" && strings.ToUpper(r.Currency) != cur {
				  continue
	  		   }
	  		  out = append(out, r)
		  }
	    	  return out, nil
	          }
	     rows,
	  	  err := a.rt.Ledger.Accounts(c.Request.Context(), uid)
	  if err != nil {
	    	   return nil, err
	    }
	          cur := strings.ToUpper(strings.TrimSpace(c.Query("currency")))
	   typ := strings.ToLower(strings.TrimSpace(c.Query("type")))
	       out := make([]kcAccount,
		  0, len(rows))
	  for _, r := range rows {
	    	  kc := bookToKC(r.Subject, r.Book)
		  if !matchType(typ, kc) {
	      		   continue
	  	  }
	   	   if cur != "" && strings.ToUpper(r.Asset) != cur {
			   continue
	        	  }
	       	  out = append(out, kcAccount{
	   		   ID: uid + ":" + kc + ":" + r.Asset, Currency: r.Asset, Type: kc,
	    		   Balance: r.Total, Available: r.Available, Holds: r.Frozen,
	  	  })
	   }
	          return mergeKCAccounts(out), nil
  }

// mergeKCAccounts 同一 (type, currency) 合成一行。wallet+spot 都映射 MAIN，禁止拆成两本。

 func mergeKCAccounts(rows []kcAccount) []kcAccount {
	   type key struct{ typ, cur string }
	       idx := map[key]int{}
	        out := make([]kcAccount, 0, len(rows))
	   for _, r := range rows {
		   k := key{r.Type, r.Currency}
	       	  if i, ok := idx[k]; ok {
	  		  out[i].Available = addDecStr(out[i].Available, r.Available)
			   out[i].Holds = addDecStr(out[i].Holds,
				  r.Holds)
	    		   out[i].Balance = addDecStr(out[i].Balance, r.Balance)
	    		   continue
	    	   }
	    	  idx[k] = len(out)
	    	   out = append(out, r)
	       }
	  return out
   }

  func addDecStr(a, b string) string {
	   x, e1 := decimal.NewFromString(a)
	  y, e2 := decimal.NewFromString(b)
	   if e1 != nil || e2 != nil {
	       	   return a
	    }
	      return x.Add(y).String()
   }

   func (a *API) getAccountList(c *gin.Context) {
	     out, okB := a.booksOrFail(c)
	   if !okB {
	    	  return
	       }
	      ok(c, out)
  }

 func (a *API) getAccountDetail(c *gin.Context) {
	      id := c.Param("accountId")
	      out, okB := a.booksOrFail(c)
	    if !okB {
	      	   return
	        }
	  for _, row := range out {
	    	  if row.ID == id {
	  		  ok(c, row)
			  return
		  }
	       }
	   fail(c, 404, "account not found")
   }

  func (a *API) getSpotHFOpened(c *gin.Context) {
	   ok(c, a.hfOpened(c))
   }

  func (a *API) hfOpened(c *gin.Context) bool {
	     uid := a.user(c)
	    if a == nil || a.rt == nil || a.rt.Ledger == nil || strings.TrimSpace(uid) == "" {
	     	  return false
	      }
	    rows, err := a.rt.Ledger.Accounts(c.Request.Context(), uid)
	  if err != nil {
	  	  return false
	        }
	  for _, r := range rows {
	  	  if strings.Contains(strings.ToLower(r.Subject+" "+r.Book), "hf") {
	  		  return true
	  	  }
	       }
	  return false
  }

  func (a *API) hasKCBook(c *gin.Context, kc string) bool {
	    uid := a.user(c)
	    if a == nil || a.rt == nil || a.rt.Ledger == nil || strings.TrimSpace(uid) == "" {
	      	  return false
	  }
	    rows, err := a.rt.Ledger.Accounts(c.Request.Context(), uid)
	  if err != nil {
		  return false
	        }
	      for _, r := range rows {
	   	  if bookToKC(r.Subject, r.Book) == kc {
	   		  return true
	      	  }
	      }
	     return false
  }

  func (a *API) quoteEquity(c *gin.Context, uid, quote string) string {
	      quote = strings.ToUpper(strings.TrimSpace(quote))
	  if quote == "" {
	    	  quote = "USDT"
	  }
	      if a == nil || a.rt == nil || a.rt.Ledger == nil || strings.TrimSpace(uid) == "" {
		  return "0"
	       }
	    rows, err := a.rt.Ledger.Accounts(c.Request.Context(), uid)
	    if err != nil {
	    	  return "0"
	    }
	        total := decimal.Zero
	  for _, r := range rows {
	  	  if strings.ToUpper(r.Asset) != quote {
	    		  continue
		  }
	   	  v, err := decimal.NewFromString(strings.TrimSpace(r.Total))
	    	  if err == nil {
			  total = total.Add(v)
	    	  }
	    }
	    return total.String()
  }

  func (a *API) bookAvailable(c *gin.Context, uid, kc, currency string) string {
	  currency = strings.ToUpper(strings.TrimSpace(currency))
	    if a == nil || a.rt == nil || a.rt.Ledger == nil || strings.TrimSpace(uid) == "" {
		  return "0"
	      }
	     rows, err := a.rt.Ledger.Accounts(c.Request.Context(), uid)
	  if err != nil {
	   	  return "0"
	  }
	    total := decimal.Zero
	    for _, r := range rows {
	   	  if currency != "" && strings.ToUpper(r.Asset) != currency {
	   		  continue
	   	  }
	    	  if kc != "" && bookToKC(r.Subject, r.Book) != kc {
	  		  continue
		  }
	   	  v, err := decimal.NewFromString(strings.TrimSpace(r.Available))
		  if err == nil {
	    		  total = total.Add(v)
	    	  }
	      }
	      return total.String()
  }

   func (a *API) getFuturesLedgers(c *gin.Context) {
	   uid, okU := a.requireUser(c)
	    if !okU {
	   	  return
	   }
	      bills, err := a.rt.Ledger.ListBills(c.Request.Context(), uid, 200)
	     if err != nil {
	   	   fail(c, 400, err.Error())
		  return
	        }
	    cur := strings.ToUpper(strings.TrimSpace(c.Query("currency")))
	     list := make([]gin.H, 0)
	          for _, b := range bills {
	  	   kc := bookToKC(b.Subject, b.Book)
	  	   if kc != "contract" {
	  		  continue
	  	   }
	      	   if cur != "" && strings.ToUpper(b.Asset) != cur {
			  continue
		   }
	   	   list = append(list, gin.H{
	    		   "time": b.Seq, "type": b.AppType, "amount": firstNonEmpty(b.Credit, b.Debit),
			   "fee": a.chargedFee(c, b), "status": "Completed", "currency": b.Asset, "remark": b.Type,
		  })
	   }
	     ok(c, gin.H{"dataList": list, "hasMore": false})
   }

   func (a *API) classicBalance(c *gin.Context) {
	   typ := strings.TrimSpace(c.Query("accountType"))
	   if typ == "" {
		   uid := a.user(c)
	  	  if strings.TrimSpace(uid) != "" {
			   book, err := a.prefs().ResolveDefault(c.Request.Context(), uid)
	    		   if err != nil {
	    			   failPref(c, err)
	  			  return
	      		  }
	  		   typ = fundpref.KCAccountType(book)
	   	  } else {
			   typ = "FUNDING"
	    	  }
	   }
	      if book, okb := kcToBook(typ); okb {
	   	   c.Request.URL.RawQuery = "type=" + bookToKC(string(book), string(book)) + "&" + c.Request.URL.RawQuery
	         }
	      out, okB := a.booksOrFail(c)
	   if !okB {
	   	  return
	    }
	   rows := make([]gin.H, 0, len(out))
	   for _, r := range out {
		   rows = append(rows, gin.H{
	    		  "currency": r.Currency, "locked": r.Holds, "available": r.Available,
	    		  "balance": r.Balance, "equity": r.Balance,
	    	  })
	    }
	     ok(c, gin.H{"accountType": strings.ToUpper(strings.TrimSpace(typ)), "accounts": []gin.H{{"currencies": rows}}})
  }

  func (a *API) getLedgers(c *gin.Context) {
	      uid, okU := a.requireUser(c)
	       if !okU {
	     	  return
	      }
	      items, err := a.listLedgerItems(c, uid, "")
	  if err != nil {
	      	   failQuery(c, err)
		  return
	     }
	  ok(c, pageLedgers(c, items))
  }

 func (a *API) listLedgerItems(c *gin.Context, uid, forceAcct string) ([]gin.H, error) {
	      bills,
	       	   err := a.loadBills(c, uid, 500)
	       if err != nil {
		   return nil, err
	        }
	   cur := strings.ToUpper(strings.TrimSpace(bffParam(c, "currency")))
	  typ := strings.ToLower(strings.TrimSpace(bffParam(c, "bizType", "biz_type")))
	     acct := strings.ToLower(strings.TrimSpace(firstNonEmpty(forceAcct, firstNonEmpty(bffParam(c, "accountType", "type"), bffParam(c, "type")))))
	  dir := strings.ToLower(strings.TrimSpace(bffParam(c, "direction")))
	     start, hasStart, err := seqWindow(bffParam(c, "startAt", "start_at"))
	         if err != nil {
	       	  return nil, err
	   }
	       end, hasEnd, err := seqWindow(bffParam(c, "endAt", "end_at"))
	  if err != nil {
		   return nil, err
	     }
	  items := make([]gin.H, 0, len(bills))
	    bal := a.bookTotals(c, uid)
	     for _, b := range bills {
	    	   if cur != "" && strings.ToUpper(b.Asset) != cur {
			   continue
		  }
		  if typ != "" && !strings.EqualFold(b.AppType, typ) && !strings.EqualFold(b.Type, typ) {
	    		   continue
		   }
	   	   kc := bookToKC(b.Subject, b.Book)
	     	  if acct != "" && !matchType(acct, kc) {
			   continue
	        	   }
	  	  if hasStart && b.Seq < start {
			   continue
		  }
		  if hasEnd && b.Seq > end {
			   continue
		   }
	  	   way := directionOf(b.Credit, b.Debit)
		   if dir != "" && !strings.EqualFold(dir, way) {
	  		  continue
	   	  }
		  items = append(items, enhanceLedgerItem(a.ledgerItem(c, b, kc, way, bal)))
	       }
	       return items, nil
  }

  func (a *API) loadBills(c *gin.Context, uid string, limit int) ([]ledger.Bill, error) {
	     mesh, hit, err := a.tryMeshBills(c, uid, limit)
	     if err != nil {
		  return nil, err
	     }
	      if hit {
		   return mesh, nil
	       }
	      if a == nil || a.rt == nil || a.rt.Ledger == nil {
	   	   return nil, errors.New("funding book unavailable")
	     }
	        return a.rt.Ledger.ListBills(c.Request.Context(), uid, limit)
  }

  func (a *API) bookTotals(c *gin.Context, uid string) map[string]string {
	    out := map[string]string{}
	  if a == nil || a.rt == nil || a.rt.Ledger == nil || strings.TrimSpace(uid) == "" {
	  	  return out
	  }
	    rows, err := a.rt.Ledger.Accounts(c.Request.Context(), uid)
	         if err != nil {
	   	  return out
	      }
	      for _, r := range rows {
		  out[r.Book+"|"+strings.ToUpper(r.Asset)] = r.Total
	  }
	    return out
  }

   func (a *API) chargedFee(c *gin.Context, b ledger.Bill) string {
	       if a == nil || a.rt == nil || a.rt.Ledger == nil || strings.TrimSpace(b.MovementID) == "" {
		  return "0"
	  }
	    return a.rt.Ledger.ChargedFee(c.Request.Context(), b.MovementID, b.Asset)
  }

  func (a *API) ledgerItem(c *gin.Context, b ledger.Bill, kc, way string, bal map[string]string) gin.H {
	       balance := ""
	    if bal != nil {
	  	  balance = bal[b.Book+"|"+strings.ToUpper(b.Asset)]
	  }
	     return gin.H{
		   "id": b.MovementID, "currency": b.Asset, "amount": firstNonEmpty(b.Credit, b.Debit),
	       	  "fee": a.chargedFee(c, b), "balance": balance, "accountType": kc,
	    	  "bizType": b.AppType, "direction": way,
	      	   "context": b.Type, "createdAt": b.Seq,
	   }
   }

// seqWindow 仅接受流水序号。空串表示不设窗；毫秒或非法值拒绝，避免把时间窗当成序号滤空或滤丢。

  func seqWindow(raw string) (int, bool, error) {
	          raw = strings.TrimSpace(raw)
	  if raw == "" {
	   	   return 0, false, nil
	      }
	   n, err := strconv.ParseInt(raw, 10, 64)
	  if err != nil || n <= 0 || n >= 1e11 {
	    	   return 0, false, errors.New("startAt/endAt must be ledger seq")
	  }
	  return int(n), true, nil
   }

   func pageLedgers(c *gin.Context, items []gin.H) gin.H {
	       page, _ := parsePositiveInt(bffParam(c, "currentPage"), bffParam(c, "page"), 1)
	   size, _ := parsePositiveInt(bffParam(c, "pageSize"), "", 0)
	  sliced, cur, ps, tot, tp := pageItems(items, page, size)
	  return gin.H{"currentPage": cur, "pageSize": ps, "totalNum": tot, "totalPage": tp, "items": sliced}
  }

   func pageItems[T any](items []T, page, size int) (out []T, current, pageSize, total, totalPage int) {
	    total = len(items)
	       if page <= 0 && size <= 0 {
		  if total == 0 {
	   		  return items, 1, 0, 0, 1
	       	  }
	    	   return items, 1, total, total, 1
	  }
	     if page <= 0 {
		   page = 1
	       }
	     if size <= 0 {
	    	   size = 50
	         }
	     if size > 500 {
	      	  size = 500
	    }
	       totalPage = (total + size - 1) / size
	       if totalPage <= 0 {
	    	   totalPage = 1
	  }
	         start := (page - 1) * size
	  if start >= total {
	    	  return []T{}, page, size, total, totalPage
	   }
	  end := start + size
	      if end > total {
	  	  end = total
	         }
	         return items[start:end], page, size, total, totalPage
  }

 func (a *API) getFuturesOverview(c *gin.Context) {
	  c.Request.URL.RawQuery = "type=contract&" + c.Request.URL.RawQuery
	   out, okB := a.booksOrFail(c)
	      if !okB {
	    	   return
	  }
	     ok(c, gin.H{"accountEquity": sumAvail(out), "availableBalance": sumAvail(out), "currency": c.Query("currency"), "accounts": out})
   }

  func (a *API) isoBook() *isobook.Book {
	  if a != nil && a.rt != nil && a.rt.Isolated != nil {
	    	   return a.rt.Isolated
	  }
	   if a != nil && a.rt != nil {
	      	  a.rt.Isolated = isobook.New()
	  	  return a.rt.Isolated
	     }
	  return isobook.New()
  }

  func firstNonEmpty(a, b string) string {
	   if a != "" && a != "0" {
	   	   return a
	     }
	     return b
   }

 func directionOf(credit, debit string) string {
	   if credit != "" && credit != "0" {
		   return "in"
	  }
	  return "out"
  }

   func sumAvail(rows []kcAccount) string {
	   _ = account.BookFunding
	       total := decimal.Zero
	   for _, r := range rows {
		  amt, err := decimal.NewFromString(r.Available)
		   if err != nil {
	   		  continue
	    	   }
	      	   total = total.Add(amt)
	   }
	  return total.String()
  }
