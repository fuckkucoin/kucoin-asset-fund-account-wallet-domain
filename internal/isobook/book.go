package isobook

import (
	"sort"
	"strings"
	"sync"
	"time"
	"github.com/shopspring/decimal"
)

type Side struct {
	Currency string `json:"currency"`
	Total     string `json:"total"`
	Available string `json:"available"`
	Hold               string `json:"hold"`
	Liability          string `json:"liability"`
	LiabilityPrincipal string `json:"liabilityPrincipal"`
	LiabilityInterest  string `json:"liabilityInterest"`
	MaxBorrowSize      string `json:"maxBorrowSize"`
	BorrowEnabled      bool   `json:"borrowEnabled"`
	TransferInEnabled bool `json:"transferInEnabled"`
}

type Pair struct {
	UserID string `json:"user_id"`
	Symbol    string `json:"symbol"`
	Status    string `json:"status"`
	DebtRatio string `json:"debtRatio"`
	Base      Side   `json:"baseAsset"`
	Quote     Side   `json:"quoteAsset"`
}

type Book struct {
	mu sync.Mutex
	rows      map[string]*Pair
	sql       *persistSQL
	genBumper GenerationBumper
}

  func New() *Book { return &Book{rows: map[string]*Pair{}} }

  func SplitSymbol(symbol string) (base, quote string) {
	  symbol = strings.ToUpper(strings.TrimSpace(symbol))
	   if symbol == "" {
		  return "BTC", "USDT"
	  }
	  if i := strings.Index(symbol, "-"); i > 0 && i < len(symbol)-1 {
		  return symbol[:i], symbol[i+1:]
	  }
	  return symbol, "USDT"
   }

  func (b *Book) Ensure(userID, symbol string) Pair {
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  return b.ensureLocked(userID,
		   symbol)
   }

   func (b *Book) ensureLocked(userID, symbol string) Pair {
	  userID = strings.TrimSpace(userID)
	   symbol = strings.ToUpper(strings.TrimSpace(symbol))
	   if symbol == "" {
		  symbol = "BTC-USDT"
	  }
	  base, quote := SplitSymbol(symbol)
	  key := userID + "|" + symbol
	   if p, ok := b.rows[key]; ok {
		   return *p
	   }
	   p := &Pair{
		   UserID: userID, Symbol: symbol, Status: "EFFECTIVE", DebtRatio: "0",
		   Base: emptySide(base),
		   Quote: emptySide(quote),
	   }
	   b.rows[key] = p
	  b.persistPair(*p)
	  return *p
   }

   func emptySide(cur string) Side {
	   return Side{
		  Currency: cur, Total: "0", Available: "0", Hold: "0",
		  Liability: "0", LiabilityPrincipal: "0", LiabilityInterest: "0",
		  MaxBorrowSize: "0", BorrowEnabled: false, TransferInEnabled: true,
	   }
   }

   func (b *Book) Credit(userID, symbol, currency, amount string) (Pair, error) {
	   amt, err := decimal.NewFromString(strings.TrimSpace(amount))
	   if err != nil || !amt.IsPositive() {
		   return Pair{}, errAmount
	   }
	  b.mu.Lock()
	   defer b.mu.Unlock()
	   p := b.ensureLocked(userID, symbol)
	  row := b.rows[userID+"|"+p.Symbol]
	   cur := strings.ToUpper(strings.TrimSpace(currency))
	   switch cur {
	   case row.Base.Currency:
		  row.Base = addAvail(row.Base, amt)
	   case row.Quote.Currency:
		   row.Quote = addAvail(row.Quote,
			   amt)
	  default:
		  return Pair{}, errAsset
	  }
	  out := *row
	   b.persistPair(out)
	   return out, nil
  }

  func (b *Book) Debit(userID, symbol, currency, amount string) (Pair, error) {
	  amt, err := decimal.NewFromString(strings.TrimSpace(amount))
	   if err != nil || !amt.IsPositive() {
		   return Pair{}, errAmount
	   }
	  b.mu.Lock()
	  defer b.mu.Unlock()
	  p := b.ensureLocked(userID, symbol)
	   row := b.rows[userID+"|"+p.Symbol]
	   cur := strings.ToUpper(strings.TrimSpace(currency))
	  switch cur {
	   case row.Base.Currency:
		   next, err := subAvail(row.Base, amt)
		  if err != nil {
			   return Pair{}, err
		   }
		   row.Base = next
	   case row.Quote.Currency:
		  next, err := subAvail(row.Quote, amt)
		  if err != nil {
			  return Pair{}, err
		  }
		   row.Quote = next
	   default:
		  return Pair{}, errAsset
	  }
	   out := *row
	  b.persistPair(out)
	  return out, nil
  }

   func addAvail(s Side, amt decimal.Decimal) Side {
	   av,
		   _ := decimal.NewFromString(s.Available)
	  tot, _ := decimal.NewFromString(s.Total)
	  s.Available = av.Add(amt).String()
	  s.Total = tot.Add(amt).String()
	  return s
   }

  func subAvail(s Side, amt decimal.Decimal) (Side, error) {
	  av, _ := decimal.NewFromString(s.Available)
	  if av.LessThan(amt) {
		  return s, errAmount
	   }
	   tot,
		   _ := decimal.NewFromString(s.Total)
	   s.Available = av.Sub(amt).String()
	  s.Total = tot.Sub(amt).String()
	   return s, nil
  }

   func (b *Book) List(userID, symbol string) []Pair {
	  if b == nil {
		   return nil
	  }
	   userID = strings.TrimSpace(userID)
	  symbol = strings.ToUpper(strings.TrimSpace(symbol))
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  out := b.listLocked(userID, symbol)
	  if len(out) == 0 && userID != "" {
		   // 读面空户只返回内存预览，禁止 GET 落库默认交易对。
		  return []Pair{previewPair(userID, firstNonEmpty(symbol, "BTC-USDT"))}
	   }
	   return out
  }

// ListExisting 仅返回已开仓交易对，不补默认 BTC-USDT。
// 资金页 isolated 总览避免空户被物化成默认对。
  func (b *Book) ListExisting(userID, symbol string) []Pair {
	   userID = strings.TrimSpace(userID)
	   symbol = strings.ToUpper(strings.TrimSpace(symbol))
	   if b == nil || userID == "" {
		  return nil
	   }
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   return b.listLocked(userID,
		  symbol)
   }

// Get 读取单交易对；未开仓返回 false，调用方决定是否 Ensure。
  func (b *Book) Get(userID, symbol string) (Pair, bool) {
	  userID = strings.TrimSpace(userID)
	  symbol = strings.ToUpper(strings.TrimSpace(symbol))
	  if b == nil || userID == "" || symbol == "" {
		  return Pair{}, false
	  }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	  p, ok := b.rows[userID+"|"+symbol]
	   if !ok {
		   return Pair{}, false
	  }
	  return *p, true
   }

// Preview 详情读：已开仓返回账本行，否则内存零仓，不 persist。

  func (b *Book) Preview(userID, symbol string) Pair {
	   if p, ok := b.Get(userID, symbol); ok {
		  return p.WithRisk()
	   }
	   return previewPair(userID, symbol)
  }

   func previewPair(userID, symbol string) Pair {
	  userID = strings.TrimSpace(userID)
	  symbol = strings.ToUpper(strings.TrimSpace(symbol))
	  if symbol == "" {
		  symbol = "BTC-USDT"
	  }
	  base, quote := SplitSymbol(symbol)
	  return Pair{
		  UserID: userID, Symbol: symbol, Status: StatusEffective, DebtRatio: "0",
		  Base: emptySide(base), Quote: emptySide(quote),
	  }.WithRisk()
   }

   func (b *Book) listLocked(userID, symbol string) []Pair {
	   out := make([]Pair, 0)
	  for _, p := range b.rows {
		   if p.UserID != userID {
			   continue
		   }
		   if symbol != "" && p.Symbol != symbol {
			  continue
		  }
		  out = append(out, *p)
	  }
	  sort.Slice(out, func(i, j int) bool { return out[i].Symbol < out[j].Symbol })
	  return out
  }

  func (b *Book) Snapshot(userID, symbol, quote string) (totalAsset, totalLiab string, ts int64, assets []Pair) {
	   return b.snapshot(userID, symbol, quote, true)
  }

// SnapshotExisting 不补默认交易对，供资金页已开仓汇总。
  func (b *Book) SnapshotExisting(userID, symbol, quote string) (totalAsset, totalLiab string, ts int64, assets []Pair) {
	  return b.snapshot(userID, symbol, quote, false)
  }

   func (b *Book) snapshot(userID, symbol, quote string, ensure bool) (totalAsset, totalLiab string, ts int64, assets []Pair) {
	   if ensure {
		  assets = b.List(userID, symbol)
	   } else {
		   assets = b.ListExisting(userID, symbol)
	   }
	   quote = strings.ToUpper(strings.TrimSpace(quote))
	   if quote == "" {
		   quote = "USDT"
	  }
	   asset, liab := decimal.Zero, decimal.Zero
	  for i, p := range assets {
		  assets[i] = p.WithRisk()
		   p = assets[i]
		  if p.Quote.Currency != quote {
			   continue
		   }
		  if v, err := decimal.NewFromString(p.Quote.Total); err == nil {
			  asset = asset.Add(v)
		  }
		  if v, err := decimal.NewFromString(p.Quote.Liability); err == nil {
			   liab = liab.Add(v)
		  }
	  }
	   return asset.String(),
		   liab.String(), time.Now().UTC().UnixMilli(), assets
  }

// BookedSymbols 只返回已经落账的交易对。
  func (b *Book) BookedSymbols() []string {
	  if b == nil {
		  return nil
	  }
	  b.mu.Lock()
	  defer b.mu.Unlock()
	  seen := map[string]struct{}{}
	  for _, p := range b.rows {
		  if p != nil && p.Symbol != "" {
			  seen[p.Symbol] = struct{}{}
		  }
	  }
	  out := make([]string, 0, len(seen))
	  for s := range seen {
		  out = append(out, s)
	  }
	  sort.Strings(out)
	  return out
  }

// KnownSymbols 只返回已经落账的交易对。交易对由记账写入，不再并入写死的默认清单。
   func (b *Book) KnownSymbols() []string {
	   seen := map[string]struct{}{}
	   if b != nil {
		   b.mu.Lock()
		  for _, p := range b.rows {
			  if p != nil && p.Symbol != "" {
				  seen[p.Symbol] = struct{}{}
			  }
		  }
		  b.mu.Unlock()
	   }
	   out := make([]string, 0, len(seen))
	  for s := range seen {
		  out = append(out, s)
	   }
	   sort.Strings(out)
	   return out
   }

  func firstNonEmpty(a, b string) string {
	   if strings.TrimSpace(a) != "" {
		   return a
	   }
	  return b
  }

type errStr string

   func (e errStr) Error() string { return string(e) }

var (
	errAmount = errStr("isolated amount required")
	errAsset = errStr("isolated asset not in pair")
)
