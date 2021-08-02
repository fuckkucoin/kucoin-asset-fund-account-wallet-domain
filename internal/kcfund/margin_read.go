// @FileName: margin_read.go
// @Summary: Classic 全仓/逐仓账户读面：列表、详情、流水与目录。
// 一致 docs.kucoin.com Classic margin/isolated GET。未接借款引擎时负债与可借为 0；queryType 仅接受公开枚举，禁止回落现货科目。
// @Author: Knox
// @Created: 2021.08.01

package kcfund

import (
	"errors"
	    "strconv"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/isobook"
	  "github.com/gin-gonic/gin"
	   "github.com/shopspring/decimal"
)

  func (a *API) classicMarginReads(g *gin.RouterGroup) {
	    g.GET("/v1/margin/account", a.getMarginAccount)
	     g.GET("/v1/margin/account/ledgers", a.getMarginLedgers)
	    g.GET("/v1/isolated/accounts", a.getIsolatedAccount)
	     g.GET("/v1/isolated/accounts/ledgers", a.getIsolatedLedgers)
	      g.GET("/v1/isolated/account/:symbol", a.getIsolatedAccountDetail)
	      g.GET("/v3/isolated/account/:symbol", a.getIsolatedAccountDetailV3)
	     g.GET("/v3/margin/accounts", a.getCrossMarginV3)
	      g.GET("/v3/isolated/accounts", a.getIsolatedV3)
	       g.GET("/v1/hf/margin/account", a.getHFMarginAccount)
	  g.GET("/v3/hf/margin/account", a.getHFMarginAccount)
	  g.GET("/v3/hf/margin/account/ledgers", a.getHFMarginLedgers)
	  g.GET("/v3/margin/currencies", a.getMarginCurrencies)
	  g.GET("/v1/margin/config", a.getMarginConfig)
	    g.GET("/v3/margin/symbols", a.getMarginSymbols)
	  g.GET("/v3/isolated/symbols", a.getIsolatedSymbols)
	      g.GET("/v1/isolated/symbols", a.getIsolatedSymbols)
   }

   func acceptCrossQuery(raw string) (string, bool) {
	  q := strings.ToUpper(strings.TrimSpace(raw))
	  if q == "" {
		  return "MARGIN", true
	   }
	      switch q {
	  case "MARGIN", "MARGIN_V2", "MARGIN_HF", "CROSS":
	   	   return q, true
	  default:
		  return "", false
	  }
  }

  func acceptIsolatedQuery(raw string) (string, bool) {
	      q := strings.ToUpper(strings.TrimSpace(raw))
	      if q == "" {
	  	  return "ISOLATED", true
	   }
	  switch q {
	  case "ISOLATED", "ISOLATED_V2":
		   return q, true
	      default:
		  return "", false
	  }
  }

  func (a *API) forceMarginQuery(c *gin.Context) {
	       c.Request.URL.RawQuery = "type=margin&" + c.Request.URL.RawQuery
   }

 func (a *API) getMarginAccount(c *gin.Context) {
	  a.forceMarginQuery(c)
	   out, okB := a.booksOrFail(c)
	      if !okB {
		   return
	     }
	     quote := isolatedQuote(c)
	   uid := a.user(c)
	     accs := make([]gin.H, 0, len(out))
	      for _, r := range out {
		  total, _, _ := a.liabilityParts(uid, r.Currency)
		  accs = append(accs, gin.H{
	    		  "currency": r.Currency, "totalBalance": r.Balance,
			   "availableBalance": r.Available, "holdBalance": r.Holds,
			  "liability": total, "maxBorrowSize": a.maxBorrow(c, uid, r.Currency),
	   	  })
	    }
	      asset := sumQuoteField(out, quote, func(r kcAccount) string { return r.Balance })
	      ok(c, gin.H{"debtRatio": quoteDebtRatio(asset, a.utaBook().LiabilityByCurrency(uid, quote)), "accounts": accs})
  }

  func (a *API) getCrossMarginV3(c *gin.Context) {
	     qt, okQ := acceptCrossQuery(c.Query("queryType"))
	       if !okQ {
		   fail(c, 400, "queryType invalid")
		  return
	   }
	   a.forceMarginQuery(c)
	   out, okB := a.booksOrFail(c)
	   if !okB {
	    	  return
	     }
	  body, okBody := a.crossMarginV3Body(c, a.user(c), out, isolatedQuote(c))
	      if !okBody {
	  	  return
	  }
	  body["queryType"] = qt
	  ok(c, body)
  }

   func (a *API) getHFMarginAccount(c *gin.Context) {
	   qt, okQ := acceptCrossQuery(firstNonEmpty(c.Query("queryType"), "MARGIN_HF"))
	      if !okQ {
		  fail(c, 400, "queryType invalid")
	    	   return
	  }
	     a.forceMarginQuery(c)
	     out, okB := a.booksOrFail(c)
	   if !okB {
		  return
	   }
	     body, okBody := a.crossMarginV3Body(c, a.user(c), out, isolatedQuote(c))
	  if !okBody {
	    	  return
	      }
	     body["hfOpened"] = a.hfOpened(c)
	     body["queryType"] = qt
	      ok(c, body)
   }

  func (a *API) crossMarginV3Body(c *gin.Context, uid string, out []kcAccount, quote string) (gin.H, bool) {
	   quote = strings.ToUpper(strings.TrimSpace(quote))
	    if quote == "" {
	    	  quote = "USDT"
	       }
	      accs := make([]gin.H, 0, len(out))
	      for _, r := range out {
		  total, principal, interest := a.liabilityParts(uid, r.Currency)
		  lim, err := a.utaBook().RequireInterestLimits(r.Currency)
		  if err != nil {
			  fail(c, 503, err.Error())
			  return gin.H{}, false
	    	  }
		  accs = append(accs, gin.H{
			  "currency": r.Currency, "total": r.Balance, "available": r.Available, "hold": r.Holds,
	   		  "liability": total, "liabilityPrincipal": principal, "liabilityInterest": interest,
	    		  "maxBorrowSize": a.maxBorrow(c, uid, r.Currency), "borrowEnabled": decPos(lim.BorrowLimitTotal),
	  		  "transferInEnabled": a.transferInOpen(c, r.Currency),
		  })
	       }
	   asset := sumQuoteField(out, quote, func(r kcAccount) string { return r.Balance })
	   liab := a.utaBook().LiabilityByCurrency(uid, quote)
	      status := isobook.StatusClear
	     if decPos(asset) {
		   status = isobook.StatusEffective
	   }
	      return gin.H{
		  "totalAssetOfQuoteCurrency":     asset,
		   "totalLiabilityOfQuoteCurrency": liab,
		  "debtRatio":                     quoteDebtRatio(asset, liab),
		   "status":                        status,
	  	   "accounts": accs,
	  }, true
   }

   func (a *API) getIsolatedAccount(c *gin.Context) {
	  if _, okU := a.requireUser(c); !okU {
	   	  return
	   }
	       total, liab, _, pairs, err := a.isolatedSnap(c, true)
	   if err != nil {
	   	   failQuery(c, err)
	  	  return
	     }
	   body := gin.H{
		  "totalConversionBalance": total, "liabilityConversionBalance": liab,
	  }
	      ok(c, withPairPage(c, body, pairs, func(ps []isobook.Pair) any { return isobook.V1Pairs(ps) }))
   }

  func (a *API) getIsolatedV3(c *gin.Context) {
	      qt, okQ := acceptIsolatedQuery(c.Query("queryType"))
	  if !okQ {
	   	  fail(c, 400, "queryType invalid")
		   return
	      }
	      if _, okU := a.requireUser(c); !okU {
	    	  return
	   }
	   total, liab, ts, pairs, err := a.isolatedSnap(c, true)
	     if err != nil {
	   	   failQuery(c, err)
		   return
	       }
	      body := gin.H{
	  	   "totalAssetOfQuoteCurrency": total, "totalLiabilityOfQuoteCurrency": liab,
		  "timestamp": ts, "queryType": qt,
	   }
	   ok(c, withPairPage(c, body, pairs, func(ps []isobook.Pair) any { return isobook.V3Pairs(ps) }))
  }

  func (a *API) getIsolatedAccountDetail(c *gin.Context) {
	    p, okP := a.isolatedDetail(c)
	   if !okP {
	  	   return
	      }
	  ok(c, isobook.V1Pair(p))
  }

   func (a *API) getIsolatedAccountDetailV3(c *gin.Context) {
	       p, okP := a.isolatedDetail(c)
	      if !okP {
	   	  return
	  }
	       ok(c, isobook.V3Pair(p))
   }

   func (a *API) isolatedDetail(c *gin.Context) (isobook.Pair, bool) {
	   if _, okU := a.requireUser(c); !okU {
		   return isobook.Pair{}, false
	   }
	  symbol := strings.ToUpper(strings.TrimSpace(c.Param("symbol")))
	  if symbol == "" {
		   fail(c, 400, "symbol required")
		   return isobook.Pair{}, false
	     }
	   c.Request.URL.RawQuery = "symbol=" + symbol + "&" + c.Request.URL.RawQuery
	      _, _, _, pairs, err := a.isolatedSnap(c, true)
	   if err != nil {
		  failQuery(c, err)
		  return isobook.Pair{}, false
	     }
	     if len(pairs) > 0 {
		  return pairs[0], true
	   }
	   fail(c, 404, "isolated account not found")
	   return isobook.Pair{}, false
   }

   func (a *API) isolatedSnap(c *gin.Context, preview bool) (total, liab string, ts int64, pairs []isobook.Pair, err error) {
	       symbol := strings.ToUpper(strings.TrimSpace(c.Query("symbol")))
	   quote := isolatedQuote(c)
	  uid := a.user(c)
	  if strings.TrimSpace(uid) == "" {
		  return "0", "0", 0, nil, errCaller
	  }
	   sides, err := a.isolatedSides(c, uid)
	   if err != nil {
	  	  return "0", "0", 0, nil, err
	       }
	   book := a.isoBook()
	  existing := book.ListExisting(uid,
	   	  symbol)
	   overlay := isobook.FilterBySymbol(isobook.OverlayPairs(uid, quote, sides), symbol)
	  pairs = isobook.MergePairs(existing, overlay)
	  _ = preview
	  pairs = isobook.FilterPairs(pairs, c.Query("currency"))
	       total, liab = isobook.SumQuote(pairs, quote)
	  return total, liab, time.Now().UTC().UnixMilli(), pairs, nil
   }

   func (a *API) isolatedSides(c *gin.Context, uid string) ([]isobook.SideInput, error) {
	  mesh, hit, err := a.tryMeshAccounts(c, uid)
	      if err != nil {
		  return nil, err
	    }
	  if hit {
	  	  return sidesFromAccounts(mesh, "isolated"), nil
	   }
	    if a == nil || a.rt == nil || a.rt.Ledger == nil {
	  	   return nil, errors.New("funding book unavailable")
	  }
	   rows, err := a.rt.Ledger.Accounts(c.Request.Context(), uid)
	     if err != nil {
	   	   return nil, err
	      }
	  out := make([]isobook.SideInput, 0)
	   for _, r := range rows {
		   if !matchType("isolated", bookToKC(r.Subject, r.Book)) {
			  continue
	   	  }
		  out = append(out, isobook.SideInput{
	    		  Currency: r.Asset, Total: r.Total, Available: r.Available, Hold: r.Frozen,
		  })
	     }
	      return out, nil
   }

  func sidesFromAccounts(rows []kcAccount, typ string) []isobook.SideInput {
	     out := make([]isobook.SideInput, 0)
	   for _, r := range rows {
		  if !matchType(typ, r.Type) {
			  continue
	   	  }
	  	  out = append(out, isobook.SideInput{
	   		   Currency: r.Currency, Total: r.Balance, Available: r.Available, Hold: r.Holds,
		   })
	   }
	  return out
  }

  func isolatedQuote(c *gin.Context) string {
	   q := strings.ToUpper(strings.TrimSpace(c.Query("quoteCurrency")))
	    if q == "" {
	   	  q = strings.ToUpper(strings.TrimSpace(c.Query("balanceCurrency")))
	      }
	   if q == "" {
		  q = "USDT"
	   }
	  return q
   }

   func withPairPage(c *gin.Context, body gin.H, pairs []isobook.Pair, view func([]isobook.Pair) any) gin.H {
	      page, _ := strconv.Atoi(strings.TrimSpace(c.Query("currentPage")))
	   size, _ := strconv.Atoi(strings.TrimSpace(c.Query("pageSize")))
	   if page <= 0 && size <= 0 {
		  body["assets"] = view(pairs)
	    	   return body
	  }
	     sliced, cur, ps, tot, tp := pageItems(pairs, page, size)
	  body["assets"] = view(sliced)
	   body["currentPage"] = cur
	      body["pageSize"] = ps
	      body["totalNum"] = tot
	   body["totalPage"] = tp
	  return body
   }

   func (a *API) getMarginLedgers(c *gin.Context) {
	  uid, okU := a.requireUser(c)
	  if !okU {
		  return
	   }
	  items, err := a.listLedgerItems(c, uid, "margin")
	  if err != nil {
		   failQuery(c, err)
	  	   return
	     }
	  ok(c, pageLedgers(c, items))
   }

   func (a *API) getIsolatedLedgers(c *gin.Context) {
	  uid, okU := a.requireUser(c)
	   if !okU {
	   	   return
	   }
	  items, err := a.listLedgerItems(c, uid, "isolated")
	   if err != nil {
		   failQuery(c, err)
		   return
	    }
	   symbol := strings.ToUpper(strings.TrimSpace(c.Query("symbol")))
	      if symbol != "" {
	  	   filtered := make([]gin.H,
			   0, len(items))
		   for _, row := range items {
			  cur, _ := row["currency"].(string)
			   if isobook.MatchPairAsset(symbol, cur) {
				   filtered = append(filtered, row)
			  }
		   }
		  items = filtered
	   }
	     ok(c, pageLedgers(c, items))
   }

   func (a *API) getHFMarginLedgers(c *gin.Context) {
	  uid, okU := a.requireUser(c)
	  if !okU {
		   return
	   }
	     items, err := a.listLedgerItems(c, uid, "margin")
	  if err != nil {
		   failQuery(c, err)
		   return
	      }
	     limit, _ := strconv.Atoi(strings.TrimSpace(c.Query("limit")))
	    if limit <= 0 {
	    	   limit = 100
	   }
	       if limit > 500 {
	    	   limit = 500
	     }
	  lastID := strings.TrimSpace(c.Query("lastId"))
	     if lastID != "" {
		  cut := -1
	    	  for i, row := range items {
			  if idOf(row) == lastID || seqOf(row) == lastID {
	    			   cut = i
	    			  break
			  }
		   }
		  if cut >= 0 {
			   items = items[cut+1:]
		  }
	   }
	    if len(items) > limit {
		  items = items[:limit]
	  }
	    next := ""
	   if n := len(items); n > 0 {
	  	   next = idOf(items[n-1])
	   }
	      ok(c, gin.H{"lastId": next, "items": items})
  }

  func (a *API) getMarginCurrencies(c *gin.Context) {
	  uid := a.user(c)
	  cur := strings.ToUpper(strings.TrimSpace(c.Query("currency")))
	       codes := a.utaBook().RateCatalog().BorrowableCurrencies()
	     if cur != "" {
	  	  known := false
		  for _, code := range codes {
			   if code == cur {
	  			  known = true
				  break
	  		   }
	    	   }
	  	   if !known {
	    		  ok(c, []gin.H{})
			  return
		  }
		   codes = []string{cur}
	   }
	  configured := map[string]struct{}{}
	     for _, code := range a.currencyCodes(c.Request.Context()) {
		  configured[code] = struct{}{}
	  }
	   items := make([]gin.H, 0, len(codes))
	   for _, code := range codes {
		  lim, err := a.utaBook().RequireInterestLimits(code)
		  if err != nil {
			  fail(c, 503, err.Error())
			  return
	  	  }
		  enabled := decPos(lim.BorrowLimitTotal)
	    	  _, _, interest := a.liabilityParts(uid, code)
		  _, listed := configured[code]
	  	  items = append(items, gin.H{
	  		   "currency": code, "borrowEnabled": enabled, "repayEnabled": enabled && decPos(interest),
	   		   "transferInEnabled": listed, "borrowCoefficient": lim.BorrowLimitHold, "marginCoefficient": lim.BorrowLimitHold,
			  "maxBorrowSize": a.maxBorrow(c, uid, code), "marketAvailable": a.bookAvailable(c, uid, "margin", code),
	   		   "currencyPrecision": a.precisionOf(code), "borrowPrecision": a.precisionOf(code),
		   })
	   }
	  ok(c, items)
   }

   func quoteDebtRatio(asset, liability string) string {
	      a, err1 := decimal.NewFromString(strings.TrimSpace(asset))
	  l, err2 := decimal.NewFromString(strings.TrimSpace(liability))
	  if err1 != nil || err2 != nil || !a.IsPositive() {
		  return "0"
	  }
	  return l.Div(a).String()
   }

  func (a *API) liabilityParts(uid, currency string) (total, principal, interest string) {
	  rows := a.utaBook().Liabilities(uid, currency, "", "")
	    t, p, i := decimal.Zero, decimal.Zero, decimal.Zero
	  for _, r := range rows {
		  if v, err := decimal.NewFromString(strings.TrimSpace(r.TotalLiability)); err == nil {
	  		  t = t.Add(v)
	   	  }
	    	  if v, err := decimal.NewFromString(strings.TrimSpace(r.Principal)); err == nil {
	  		  p = p.Add(v)
		  }
	   	  if v, err := decimal.NewFromString(strings.TrimSpace(r.Interest)); err == nil {
	    		  i = i.Add(v)
	    	  }
	  	  if v, err := decimal.NewFromString(strings.TrimSpace(r.AccruedInterest)); err == nil {
	    		  i = i.Add(v)
		  }
	  }
	  return t.String(), p.String(), i.String()
  }

  func (a *API) maxBorrow(c *gin.Context, uid, currency string) string {
	     eq := "0"
	  if c != nil {
		  eq = a.quoteEquity(c, uid, "USDT")
	    }
	    return a.utaBook().RateCatalog().PotentialBorrow(uid, currency, eq)
  }

   func sumQuoteField(rows []kcAccount, quote string, pick func(kcAccount) string) string {
	     total := decimal.Zero
	     quote = strings.ToUpper(strings.TrimSpace(quote))
	      for _, r := range rows {
		   if quote != "" && strings.ToUpper(r.Currency) != quote {
	  		   continue
		   }
	   	   amt, err := decimal.NewFromString(strings.TrimSpace(pick(r)))
	   	   if err != nil {
	  		  continue
		   }
		  total = total.Add(amt)
	  }
	  return total.String()
   }

   func decPos(s string) bool {
	   v, err := decimal.NewFromString(strings.TrimSpace(s))
	   return err == nil && v.IsPositive()
   }

  func idOf(row gin.H) string {
	   if v, ok := row["id"].(string); ok {
		  return v
	  }
	     return ""
  }

   func seqOf(row gin.H) string {
	   switch v := row["createdAt"].(type) {
	     case int:
	   	   return strconv.Itoa(v)
	  case int64:
	   	  return strconv.FormatInt(v, 10)
	    case float64:
	    	  return strconv.FormatInt(int64(v), 10)
	     case string:
		  return v
	     default:
		   return ""
	     }
  }
