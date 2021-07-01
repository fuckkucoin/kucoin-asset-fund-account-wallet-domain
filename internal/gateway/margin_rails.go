package gateway

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/fundmesh"
	"account-assets.kucoin.net/funding/internal/isobook"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/utabook"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

   func registerMarginRails(v1 *gin.RouterGroup, rt *app.Runtime) {
	  v1.GET("/assets/margin/cross", func(c *gin.Context) { writeMarginCross(c, rt) })
	   v1.GET("/assets/margin/cross/ledgers", func(c *gin.Context) { writeMarginLedgers(c, rt, "margin") })
	  v1.GET("/assets/margin/isolated", func(c *gin.Context) { writeMarginIsolated(c, rt) })
	  v1.GET("/assets/margin/isolated/ledgers", func(c *gin.Context) { writeMarginLedgers(c, rt, "isolated") })
	   v1.GET("/assets/margin/isolated/:symbol", func(c *gin.Context) { writeMarginIsolatedDetail(c, rt) })
	   v1.GET("/assets/margin/ledgers", func(c *gin.Context) { writeMarginLedgers(c, rt, firstNonEmpty(c.Query("book"), "margin")) })
	  v1.GET("/assets/margin/summary", func(c *gin.Context) { writeMarginSummary(c, rt) })
	   v1.GET("/assets/margin/currencies", func(c *gin.Context) { writeMarginCurrencies(c, rt) })
  }

  func writeMarginCross(c *gin.Context, rt *app.Runtime) {
	  user, err := validate.UserID(callerid.Resolve(c, idOpt(rt)))
	   if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	  if _, okQ := acceptCrossQuery(c.Query("queryType")); !okQ {
		   c.JSON(400, gin.H{"error": "queryType invalid"})
		  return
	  }
	  books, err := ledgerBooks(c.Request.Context(), rt, user, c.GetHeader("Authorization"))
	  if err != nil {
		   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
		  return
	   }
	  opts, err := resolveDisplayOpts(c.Request.Context(), rt, user, firstNonEmpty(c.Query("quote"), c.Query("quoteCurrency")), c.Query("hide_small"))
	   if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	  rows := filterBooks(books, "margin")
	  rows = filterBookCurrency(rows, c.Query("currency"))
	  rows = hideSmallBooks(rows, opts)
	  asset := sumBookQuote(rows, opts.Quote)
	  liab := utaLiability(rt, user, opts.Quote)
	  c.JSON(200, gin.H{
		   "user_id": user, "quote": opts.Quote, "book": "margin",
		   "total_asset": asset, "total_liability": liab,
		   "debt_ratio": debtRatio(asset, liab), "status": marginStatus(rows),
		  "accounts": rows, "host": config.HostQueryGateway,
	   })
   }

  func writeMarginIsolated(c *gin.Context, rt *app.Runtime) {
	  user, err := validate.UserID(callerid.Resolve(c, idOpt(rt)))
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	   if _, okQ := acceptIsolatedQuery(c.Query("queryType")); !okQ {
		  c.JSON(400, gin.H{"error": "queryType invalid"})
		   return
	  }
	   opts, err := resolveDisplayOpts(c.Request.Context(), rt, user, firstNonEmpty(c.Query("quote"), firstNonEmpty(c.Query("quoteCurrency"), c.Query("balanceCurrency"))), c.Query("hide_small"))
	  if err != nil {
		   mw.WriteProblem(c, err)
		   return
	  }
	   quote := opts.Quote
	  if quote == "" {
		  quote = "USDT"
	  }
	  symbol := strings.ToUpper(strings.TrimSpace(c.Query("symbol")))
	   pairs,
		   err := loadIsolatedPairs(c, rt, user, symbol, quote)
	   if err != nil {
		  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
		   return
	  }
	  pairs = isobook.FilterPairs(pairs, c.Query("currency"))
	  if opts.Hide {
		   pairs = isobook.HideSmallPairs(pairs, opts.HideSmall)
	  }
	  total, liab := isobook.SumQuote(pairs, quote)
	   page, _ := strconv.Atoi(strings.TrimSpace(c.Query("currentPage")))
	  size, _ := strconv.Atoi(strings.TrimSpace(c.Query("pageSize")))
	  body := gin.H{
		  "user_id": user, "quote": quote,
		  "total_asset": total, "total_liability": liab,
		   "host": config.HostQueryGateway,
	   }
	   if page > 0 || size > 0 {
		  var cur, ps, tot, tp int
		  pairs, cur, ps, tot, tp = pageSlice(pairs, page, size)
		   body["currentPage"] = cur
		  body["pageSize"] = ps
		   body["totalNum"] = tot
		  body["totalPage"] = tp
	   }
	  body["assets"] = isobook.V3Pairs(pairs)
	  c.JSON(200, body)
   }

   func writeMarginIsolatedDetail(c *gin.Context, rt *app.Runtime) {
	   user, err := validate.UserID(callerid.Resolve(c, idOpt(rt)))
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	   }
	  symbol := strings.ToUpper(strings.TrimSpace(c.Param("symbol")))
	  if symbol == "" {
		  c.JSON(400, gin.H{"error": "symbol required"})
		   return
	   }
	  quote := strings.ToUpper(strings.TrimSpace(firstNonEmpty(c.Query("quote"), "USDT")))
	  pairs, err := loadIsolatedPairs(c, rt, user, symbol, quote)
	   if err != nil {
		   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
		  return
	   }
	  for _, p := range pairs {
		  if p.Symbol == symbol {
			  c.JSON(200, gin.H{"user_id": user, "pair": isobook.V3Pair(p), "host": config.HostQueryGateway})
			   return
		  }
	   }
	   c.JSON(404, gin.H{"error": "isolated account not found", "host": config.HostQueryGateway})
   }

   func writeMarginLedgers(c *gin.Context, rt *app.Runtime, book string) {
	   user, err := validate.UserID(callerid.Resolve(c, idOpt(rt)))
	  if err != nil {
		   mw.WriteProblem(c, err)
		   return
	  }
	   bills, err := loadMarginBills(c, rt, user, 500)
	  if err != nil {
		   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
		   return
	  }
	   want := strings.ToLower(strings.TrimSpace(book))
	   cur := strings.ToUpper(strings.TrimSpace(c.Query("currency")))
	  biz := strings.ToLower(strings.TrimSpace(c.Query("bizType")))
	  dir := strings.ToLower(strings.TrimSpace(c.Query("direction")))
	  start, hasStart, errW := seqWindow(c.Query("startAt"))
	   if errW != nil {
		   c.JSON(400, gin.H{"error": errW.Error()})
		   return
	  }
	   end, hasEnd, errW := seqWindow(c.Query("endAt"))
	  if errW != nil {
		   c.JSON(400, gin.H{"error": errW.Error()})
		  return
	  }
	   items := make([]gin.H, 0, len(bills))
	   bal := map[string]string{}
	   if rt != nil && rt.Ledger != nil {
		   if rows, errB := rt.Ledger.Accounts(c.Request.Context(), user); errB == nil {
			   for _, row := range rows {
				   bal[row.Book+"|"+strings.ToUpper(row.Asset)] = row.Total
			   }
		   }
	   }
	   for _, b := range bills {
		  if !matchMarginBook(want, b.Book, b.Subject) {
			  continue
		  }
		  if cur != "" && strings.ToUpper(b.Asset) != cur {
			  continue
		  }
		  if biz != "" && !strings.EqualFold(b.AppType, biz) && !strings.EqualFold(b.Type, biz) {
			  continue
		  }
		  if hasStart && b.Seq < start {
			  continue
		  }
		   if hasEnd && b.Seq > end {
			   continue
		   }
		  way := "out"
		   if b.Credit != "" && b.Credit != "0" {
			   way = "in"
		  }
		  if dir != "" && !strings.EqualFold(dir, way) {
			   continue
		   }
		   if symbol := strings.ToUpper(strings.TrimSpace(c.Query("symbol"))); symbol != "" && !isobook.MatchPairAsset(symbol, b.Asset) {
			  continue
		   }
		   fee := "0"
		   if rt != nil && rt.Ledger != nil {
			   fee = rt.Ledger.ChargedFee(c.Request.Context(), b.MovementID, b.Asset)
		   }
		   items = append(items, gin.H{
			   "id": b.MovementID, "currency": b.Asset, "amount": firstNonEmpty(b.Credit, b.Debit),
			   "fee": fee, "balance": bal[b.Book+"|"+strings.ToUpper(b.Asset)], "accountType": kcBook(b.Subject, b.Book),
			   "bizType": b.AppType, "direction": way, "context": b.Type, "createdAt": b.Seq,
		   })
	   }
	  page, _ := strconv.Atoi(strings.TrimSpace(c.Query("currentPage")))
	  size, _ := strconv.Atoi(strings.TrimSpace(firstNonEmpty(c.Query("pageSize"), c.Query("limit"))))
	   sliced, curPage, ps, tot, tp := pageSlice(items, page, size)
	   c.JSON(200, gin.H{
		   "user_id": user, "book": want, "items": sliced,
		  "currentPage": curPage, "pageSize": ps, "totalNum": tot, "totalPage": tp,
		   "host": config.HostQueryGateway,
	  })
   }

  func writeMarginSummary(c *gin.Context, rt *app.Runtime) {
	   user, err := validate.UserID(callerid.Resolve(c, idOpt(rt)))
	  if err != nil {
		  mw.WriteProblem(c, err)
		  return
	  }
	  opts, err := resolveDisplayOpts(c.Request.Context(), rt, user, firstNonEmpty(c.Query("quote"), c.Query("quoteCurrency")), c.Query("hide_small"))
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	   }
	   books, err := ledgerBooks(c.Request.Context(), rt, user, c.GetHeader("Authorization"))
	  if err != nil {
		  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
		  return
	  }
	   cross := hideSmallBooks(filterBookCurrency(filterBooks(books, "margin"), c.Query("currency")), opts)
	   isoPairs, err := loadIsolatedPairs(c, rt, user, "", opts.Quote)
	  if err != nil {
		  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
		   return
	  }
	   isoPairs = isobook.FilterPairs(isoPairs, c.Query("currency"))
	   if opts.Hide {
		  isoPairs = isobook.HideSmallPairs(isoPairs, opts.HideSmall)
	   }
	  isoTotal, isoLiab := isobook.SumQuote(isoPairs, opts.Quote)
	  crossAsset := sumBookQuote(cross, opts.Quote)
	  crossLiab := utaLiability(rt, user, opts.Quote)
	  c.JSON(200, gin.H{
		  "user_id": user, "quote": opts.Quote,
		   "cross": gin.H{
			   "accounts": cross, "total_asset": crossAsset,
			   "total_liability": crossLiab, "debt_ratio": debtRatio(crossAsset, crossLiab), "status": marginStatus(cross),
		  },
		   "isolated": gin.H{
			  "pairs": len(isoPairs), "total_asset": isoTotal, "total_liability": isoLiab,
			  "status": isolatedListStatus(isoPairs),
		   },
		  "host": config.HostQueryGateway,
	  })
  }

  func writeMarginCurrencies(c *gin.Context, rt *app.Runtime) {
	  user, err := validate.UserID(callerid.Resolve(c, idOpt(rt)))
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	  want := strings.ToUpper(strings.TrimSpace(c.Query("currency")))
	  curs := gatewayMarginCurrencies(rt)
	   items := make([]gin.H, 0, len(curs))
	   for _, ccy := range curs {
		   if ccy == "" || (want != "" && ccy != want) {
			   continue
		   }
		  lim := utabook.InterestLimit{}
		  pot := "0"
		  if rt != nil && rt.UTA != nil {
			  lim, err = rt.UTA.RequireInterestLimits(ccy)
			  if err != nil {
				  mw.WriteProblem(c, err)
				  return
			  }
			  pot = rt.UTA.RateCatalog().PotentialBorrow(user, ccy, "0")
		  }
		  enabled := false
		  if v, errD := decimal.NewFromString(strings.TrimSpace(lim.BorrowLimitTotal)); errD == nil && v.IsPositive() {
			  enabled = true
		  }
		  items = append(items, gin.H{
			  "currency": ccy, "borrowEnabled": enabled, "transferInEnabled": enabled,
			   "maxBorrowSize": pot,
		   })
	  }
	   sort.Slice(items, func(i, j int) bool {
		  a, _ := items[i]["currency"].(string)
		   b, _ := items[j]["currency"].(string)
		  return a < b
	   })
	  c.JSON(200, gin.H{"items": items, "host": config.HostQueryGateway})
  }

   func loadIsolatedPairs(c *gin.Context, rt *app.Runtime, user, symbol, quote string) ([]isobook.Pair, error) {
	   sides, err := isolatedSidesFromBooks(c, rt, user)
	  if err != nil {
		  return nil, err
	   }
	   existing := isoRuntime(rt).ListExisting(user, symbol)
	   overlay := isobook.FilterBySymbol(isobook.OverlayPairs(user, quote, sides), symbol)
	  return isobook.MergePairs(existing, overlay), nil
   }

  func isolatedSidesFromBooks(c *gin.Context, rt *app.Runtime, user string) ([]isobook.SideInput, error) {
	  books, err := ledgerBooks(c.Request.Context(), rt, user, c.GetHeader("Authorization"))
	   if err != nil {
		  return nil, err
	   }
	   out := make([]isobook.SideInput, 0)
	  for _, b := range books {
		   if !matchMarginBook("isolated", b.Book, b.Subject) {
			   continue
		   }
		   out = append(out, isobook.SideInput{
			  Currency: b.Asset, Total: b.Total, Available: b.Available, Hold: b.Frozen,
		   })
	  }
	  return out, nil
   }

  func loadMarginBills(c *gin.Context, rt *app.Runtime, user string, limit int) ([]ledger.Bill, error) {
	   if rt != nil && rt.FundMesh != nil && rt.FundMesh.Active() {
		   bills, err := rt.FundMesh.LedgerBills(c.Request.Context(), meshCallOpts(c, user), user, limit)
		   if err != nil {
			   rt.Log.Warn().Err(err).Str("user", user).Msg("gateway margin mesh bills")
			   return nil, errMeshDown
		   }
		   return bills, nil
	   }
	  if rt == nil || rt.Ledger == nil {
		  return []ledger.Bill{}, nil
	   }
	   return rt.Ledger.ListBills(c.Request.Context(), user, limit)
  }

   func meshCallOpts(c *gin.Context, user string) fundmesh.CallOpts {
	   return fundmesh.CallOpts{UserID: user, Authorization: strings.TrimSpace(c.GetHeader("Authorization"))}
  }

   func isoRuntime(rt *app.Runtime) *isobook.Book {
	  if rt != nil && rt.Isolated != nil {
		  return rt.Isolated
	  }
	   return isobook.New()
  }

  func filterBooks(books []ledger.BookView, want string) []ledger.BookView {
	   want = strings.ToLower(strings.TrimSpace(want))
	   out := make([]ledger.BookView, 0, len(books))
	   for _, b := range books {
		  if matchMarginBook(want, b.Book, b.Subject) {
			   out = append(out, b)
		  }
	   }
	  return out
   }

   func filterBookCurrency(rows []ledger.BookView, currency string) []ledger.BookView {
	  currency = strings.ToUpper(strings.TrimSpace(currency))
	  if currency == "" {
		   return rows
	  }
	  out := make([]ledger.BookView,
		   0, len(rows))
	   for _, r := range rows {
		  if strings.ToUpper(r.Asset) == currency {
			  out = append(out, r)
		   }
	   }
	   return out
   }

   func hideSmallBooks(rows []ledger.BookView, opts displayOpts) []ledger.BookView {
	   if !opts.Hide {
		   return rows
	  }
	  thresh, err := decimal.NewFromString(strings.TrimSpace(opts.HideSmall))
	   if err != nil || !thresh.IsPositive() {
		   thresh = decimal.NewFromInt(1)
	   }
	  out := make([]ledger.BookView, 0, len(rows))
	  for _, r := range rows {
		  amt, err := decimal.NewFromString(strings.TrimSpace(r.Total))
		   if err != nil || amt.Abs().LessThan(thresh) {
			  continue
		  }
		  out = append(out, r)
	  }
	  return out
  }

   func matchMarginBook(want, book, subject string) bool {
	  got := strings.ToLower(strings.TrimSpace(book))
	  sub := strings.ToLower(strings.TrimSpace(subject))
	   if want == "" {
		  return got == "margin" || got == "isolated" || sub == "margin" || sub == "isolated"
	   }
	   if sub == want || got == want {
		   return true
	  }
	   if want == "margin" && (got == "margin" || sub == "margin") {
		  return true
	  }
	   if want == "isolated" && (got == "isolated" || sub == "isolated") {
		  return true
	   }
	   return false
  }

   func sumBookQuote(rows []ledger.BookView, quote string) string {
	   total := decimal.Zero
	  quote = strings.ToUpper(strings.TrimSpace(quote))
	  for _, r := range rows {
		  if quote != "" && strings.ToUpper(r.Asset) != quote {
			  continue
		  }
		  amt,
			   err := decimal.NewFromString(strings.TrimSpace(r.Total))
		   if err != nil {
			   continue
		  }
		  total = total.Add(amt)
	   }
	  return total.String()
   }

  func marginStatus(rows []ledger.BookView) string {
	  for _, r := range rows {
		   amt, err := decimal.NewFromString(strings.TrimSpace(r.Total))
		   if err == nil && amt.IsPositive() {
			   return isobook.StatusEffective
		   }
	   }
	   return isobook.StatusClear
  }

   func isolatedListStatus(pairs []isobook.Pair) string {
	  for _, p := range pairs {
		   p = p.WithRisk()
		  if p.Status == isobook.StatusEffective {
			   return isobook.StatusEffective
		   }
	  }
	   return isobook.StatusClear
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

  func kcBook(subject, book string) string {
	   s := strings.ToLower(strings.TrimSpace(subject))
	  b := strings.ToLower(strings.TrimSpace(book))
	   switch {
	  case s == "isolated" || b == "isolated":
		  return "isolated"
	   case s == "margin" || b == "margin":
		  return "margin"
	  default:
		   return b
	  }
   }

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

  func utaLiability(rt *app.Runtime, user, currency string) string {
	  if rt == nil || rt.UTA == nil {
		  return "0"
	  }
	  return rt.UTA.LiabilityByCurrency(user, currency)
  }

  func debtRatio(asset, liability string) string {
	  a, err1 := decimal.NewFromString(strings.TrimSpace(asset))
	  l, err2 := decimal.NewFromString(strings.TrimSpace(liability))
	  if err1 != nil || err2 != nil || !a.IsPositive() {
		  return "0"
	  }
	  return l.Div(a).String()
  }

  func gatewayMarginCurrencies(rt *app.Runtime) []string {
	  if rt == nil || rt.UTA == nil {
		  return []string{}
	  }
	  return rt.UTA.RateCatalog().BorrowableCurrencies()
  }

   func pageSlice[T any](items []T, page, size int) (out []T, current, pageSize, total, totalPage int) {
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

  func firstNonEmpty(a, b string) string {
	   if strings.TrimSpace(a) != "" {
		   return a
	   }
	   return b
  }
