package isobook

import (
	"sort"
	"strings"
	"github.com/shopspring/decimal"
)

const (
	StatusClear = "CLEAR"
	StatusEffective = "EFFECTIVE"
)

// DefaultSymbols Classic isolated 公开默认交易对。
   func DefaultSymbols() []string {
	   return []string{"BTC-USDT", "ETH-USDT", "KCS-USDT"}
   }

type V1Asset struct {
	Currency string `json:"currency"`
	TotalBalance string `json:"totalBalance"`
	HoldBalance      string `json:"holdBalance"`
	AvailableBalance string `json:"availableBalance"`
	Liability        string `json:"liability"`
	Interest         string `json:"interest"`
	BorrowableAmount string `json:"borrowableAmount"`
}

type V1PairView struct {
	Symbol     string  `json:"symbol"`
	Status     string  `json:"status"`
	DebtRatio  string  `json:"debtRatio"`
	BaseAsset  V1Asset `json:"baseAsset"`
	QuoteAsset V1Asset `json:"quoteAsset"`
}

type V3Asset struct {
	Currency string `json:"currency"`
	BorrowEnabled     bool   `json:"borrowEnabled"`
	TransferInEnabled bool   `json:"transferInEnabled"`
	Liability         string `json:"liability"`
	LiabilityPrincipal string `json:"liabilityPrincipal"`
	LiabilityInterest  string `json:"liabilityInterest"`
	Total              string `json:"total"`
	Available     string `json:"available"`
	Hold          string `json:"hold"`
	MaxBorrowSize string `json:"maxBorrowSize"`
}

type V3PairView struct {
	Symbol    string  `json:"symbol"`
	Status    string  `json:"status"`
	DebtRatio string  `json:"debtRatio"`
	BaseAsset V3Asset `json:"baseAsset"`
	QuoteAsset V3Asset `json:"quoteAsset"`
}

type SymbolRow struct {
	Symbol string `json:"symbol"`
	SymbolName   string `json:"symbolName"`
	BaseCurrency string `json:"baseCurrency"`
	QuoteCurrency string `json:"quoteCurrency"`
	MaxLeverage int `json:"maxLeverage"`
	FlDebtRatio           string `json:"flDebtRatio"`
	TradeEnable           bool   `json:"tradeEnable"`
	AutoRenewMaxDebtRatio string `json:"autoRenewMaxDebtRatio"`
	EnableTrading bool   `json:"enableTrading"`
	Market        string `json:"market"`
	Name          string `json:"name"`
}

   func nz(s string) string {
	  s = strings.TrimSpace(s)
	  if s == "" {
		  return "0"
	  }
	  return s
  }

  func V1Side(s Side) V1Asset {
	  return V1Asset{
		   Currency:     s.Currency,
		  TotalBalance: nz(s.Total),
		  HoldBalance:  nz(s.Hold),
		  AvailableBalance: nz(s.Available),
		  Liability:        nz(s.Liability),
		  Interest:         nz(s.LiabilityInterest),
		   BorrowableAmount: nz(s.MaxBorrowSize),
	  }
   }

  func V3Side(s Side) V3Asset {
	  return V3Asset{
		   Currency: s.Currency,
		   BorrowEnabled: s.BorrowEnabled,
		  TransferInEnabled:  s.TransferInEnabled,
		   Liability:          nz(s.Liability),
		  LiabilityPrincipal: nz(s.LiabilityPrincipal),
		   LiabilityInterest:  nz(s.LiabilityInterest),
		   Total:              nz(s.Total),
		  Available:          nz(s.Available),
		  Hold:               nz(s.Hold),
		  MaxBorrowSize:      nz(s.MaxBorrowSize),
	   }
  }

  func V1Pair(p Pair) V1PairView {
	   p = p.WithRisk()
	  return V1PairView{
		  Symbol: p.Symbol, Status: p.Status, DebtRatio: p.DebtRatio,
		   BaseAsset: V1Side(p.Base), QuoteAsset: V1Side(p.Quote),
	   }
   }

   func V3Pair(p Pair) V3PairView {
	  p = p.WithRisk()
	   return V3PairView{
		  Symbol: p.Symbol, Status: p.Status, DebtRatio: p.DebtRatio,
		  BaseAsset: V3Side(p.Base), QuoteAsset: V3Side(p.Quote),
	  }
   }

   func V1Pairs(pairs []Pair) []V1PairView {
	   out := make([]V1PairView, 0, len(pairs))
	   for _, p := range pairs {
		   out = append(out, V1Pair(p))
	   }
	  return out
  }

   func V3Pairs(pairs []Pair) []V3PairView {
	   out := make([]V3PairView,
		   0, len(pairs))
	  for _, p := range pairs {
		  out = append(out, V3Pair(p))
	   }
	   return out
   }

// WithRisk 按计价币一侧资产/负债派生 debtRatio 与 status。
// 读面统一风险字段，避免 handler 各写一套。无牌价时不计 base 市值；无负债一律 CLEAR，不编造强平态。
   func (p Pair) WithRisk() Pair {
	  p.DebtRatio, p.Status = riskOf(p)
	  return p
   }

   func riskOf(p Pair) (ratio, status string) {
	  qLiab, _ := decimal.NewFromString(nz(p.Quote.Liability))
	   bLiab, _ := decimal.NewFromString(nz(p.Base.Liability))
	   qTot, _ := decimal.NewFromString(nz(p.Quote.Total))
	   bTot, _ := decimal.NewFromString(nz(p.Base.Total))
	   liab := qLiab.Add(bLiab)
	  if !liab.IsPositive() {
		   return "0", StatusClear
	   }
	  if qTot.IsPositive() {
		   return qLiab.Div(qTot).String(), StatusEffective
	   }
	   if bTot.IsPositive() {
		  return "0", StatusEffective
	  }
	  return "0", StatusEffective
  }

// SideInput 科目行投影，供 mesh/ledger isolated 总账叠到交易对读面。
type SideInput struct {
	Currency string
	Total     string
	Available string
	Hold      string
}

   func HasCurrency(p Pair, cur string) bool {
	   cur = strings.ToUpper(strings.TrimSpace(cur))
	   if cur == "" {
		   return true
	  }
	  return strings.EqualFold(p.Base.Currency, cur) || strings.EqualFold(p.Quote.Currency, cur)
   }

  func FilterBySymbol(pairs []Pair, symbol string) []Pair {
	   symbol = strings.ToUpper(strings.TrimSpace(symbol))
	   if symbol == "" {
		  return pairs
	   }
	   out := make([]Pair, 0, 1)
	   for _, p := range pairs {
		   if p.Symbol == symbol {
			  out = append(out, p)
		   }
	   }
	  return out
   }

  func FilterPairs(pairs []Pair, currency string) []Pair {
	  currency = strings.ToUpper(strings.TrimSpace(currency))
	  if currency == "" {
		   return pairs
	   }
	  out := make([]Pair, 0, len(pairs))
	  for _, p := range pairs {
		   if HasCurrency(p, currency) {
			   out = append(out, p)
		   }
	  }
	  return out
  }

  func FilterSymbols(rows []SymbolRow, symbol, quote string) []SymbolRow {
	  symbol = strings.ToUpper(strings.TrimSpace(symbol))
	  quote = strings.ToUpper(strings.TrimSpace(quote))
	  if symbol == "" && quote == "" {
		   return rows
	   }
	   out := make([]SymbolRow, 0, len(rows))
	  for _, r := range rows {
		  if symbol != "" && r.Symbol != symbol {
			  continue
		   }
		  if quote != "" && r.QuoteCurrency != quote {
			   continue
		   }
		  out = append(out, r)
	  }
	  return out
  }

  func sideFromInput(in SideInput) Side {
	   cur := strings.ToUpper(strings.TrimSpace(in.Currency))
	   s := emptySide(cur)
	  s.Total = nz(in.Total)
	  s.Available = nz(in.Available)
	  s.Hold = nz(in.Hold)
	   return s
   }

// OverlayPairs 在无逐仓分账时，用 isolated 总账拼交易对读视图。
// mesh/本进程 isolated 科目与 isobook 空户一致，避免资金页与 REST 各看一套。同一计价币挂到多对时只计一次折算；不得把现货科目叠进逐仓。

   func OverlayPairs(userID, quote string, rows []SideInput) []Pair {
	   userID = strings.TrimSpace(userID)
	  quote = strings.ToUpper(strings.TrimSpace(quote))
	   if quote == "" {
		  quote = "USDT"
	  }
	   byCur := map[string]Side{}
	  for _, r := range rows {
		   cur := strings.ToUpper(strings.TrimSpace(r.Currency))
		   if cur == "" {
			   continue
		  }
		   byCur[cur] = sideFromInput(r)
	   }
	  if len(byCur) == 0 {
		  return nil
	  }
	  used := map[string]struct{}{}
	  out := make([]Pair, 0)
	  quoteAttached := false
	  attachQuote := func(p Pair) Pair {
		  if quoteAttached {
			  return p
		  }
		  if s, ok := byCur[quote]; ok {
			  p.Quote = s
			   used[quote] = struct{}{}
			   quoteAttached = true
		   }
		   return p
	   }
	  for _, sym := range DefaultSymbols() {
		   base, q := SplitSymbol(sym)
		  if q != quote {
			  continue
		  }
		   _, hb := byCur[base]
		  _, hq := byCur[q]
		  if !hb && !hq {
			   continue
		   }
		  p := previewPair(userID, sym)
		  if s, ok := byCur[base]; ok {
			  p.Base = s
			  used[base] = struct{}{}
		  }
		  p = attachQuote(p)
		  out = append(out, p.WithRisk())
	  }
	  for cur, s := range byCur {
		  if _, ok := used[cur]; ok || cur == quote {
			  continue
		  }
		  p := previewPair(userID, cur+"-"+quote)
		   p.Base = s
		   p = attachQuote(p)
		   out = append(out, p.WithRisk())
		   used[cur] = struct{}{}
	   }
	   sort.Slice(out, func(i, j int) bool { return out[i].Symbol < out[j].Symbol })
	   return out
   }

// MergePairs 已开仓先，overlay 只补缺失交易对。
// mesh isolated 总账与 isobook 分账并存时禁止覆盖已记账对。

  func MergePairs(keep, extra []Pair) []Pair {
	   seen := map[string]struct{}{}
	  out := make([]Pair, 0, len(keep)+len(extra))
	  for _, p := range keep {
		  if p.Symbol == "" {
			   continue
		   }
		  out = append(out, p.WithRisk())
		   seen[p.Symbol] = struct{}{}
	  }
	   for _, p := range extra {
		   if p.Symbol == "" {
			  continue
		   }
		   if _, ok := seen[p.Symbol]; ok {
			  continue
		  }
		   out = append(out, p.WithRisk())
		   seen[p.Symbol] = struct{}{}
	  }
	   sort.Slice(out,
		  func(i, j int) bool { return out[i].Symbol < out[j].Symbol })
	   return out
  }

  func HideSmallPairs(pairs []Pair, thresh string) []Pair {
	   t, err := decimal.NewFromString(strings.TrimSpace(thresh))
	   if err != nil || !t.IsPositive() {
		   t = decimal.NewFromInt(1)
	  }
	  out := make([]Pair, 0, len(pairs))
	  for _, p := range pairs {
		   bt,
			  _ := decimal.NewFromString(nz(p.Base.Total))
		   qt, _ := decimal.NewFromString(nz(p.Quote.Total))
		  if bt.Abs().LessThan(t) && qt.Abs().LessThan(t) {
			  continue
		   }
		  out = append(out, p)
	  }
	  return out
  }

   func MatchPairAsset(symbol, asset string) bool {
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	   if asset == "" || strings.TrimSpace(symbol) == "" {
		  return true
	  }
	  base, quote := SplitSymbol(symbol)
	  return asset == base || asset == quote
   }

   func SumQuote(pairs []Pair, quote string) (asset, liab string) {
	   quote = strings.ToUpper(strings.TrimSpace(quote))
	   if quote == "" {
		  quote = "USDT"
	  }
	   a, l := decimal.Zero, decimal.Zero
	  for _, p := range pairs {
		   if p.Quote.Currency != quote {
			  continue
		   }
		   if v, err := decimal.NewFromString(nz(p.Quote.Total)); err == nil {
			  a = a.Add(v)
		  }
		  if v, err := decimal.NewFromString(nz(p.Quote.Liability)); err == nil {
			  l = l.Add(v)
		   }
	  }
	  return a.String(), l.String()
  }

   func SymbolCatalog(symbols []string) []SymbolRow {
	  out := make([]SymbolRow, 0, len(symbols))
	  for _, raw := range symbols {
		   sym := strings.ToUpper(strings.TrimSpace(raw))
		   if sym == "" {
			   continue
		   }
		   base, quote := SplitSymbol(sym)
		  out = append(out, SymbolRow{
			   Symbol: sym,
			   SymbolName: sym,
			   Name:         sym,
			   BaseCurrency: base,
			   QuoteCurrency: quote,
		   })
	   }
	  return out
   }
