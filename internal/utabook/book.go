package utabook

import (
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

type Leverage struct {
	Symbol   string `json:"symbol"`
	Currency string `json:"currency,omitempty"`
	Leverage string `json:"leverage"`
	MarginMode string `json:"marginMode"`
	TradeType  string `json:"tradeType,omitempty"`
}

type InterestLimit struct {
	CurrentRateHourly string `json:"currentRateHourly"`
	CurrentRateDaily  string `json:"currentRateDaily"`
	BorrowLimitTotal     string `json:"borrowLimitTotal"`
	BorrowLimitTotalHold string `json:"borrowLimitTotalHold"`
	BorrowLimitHold      string `json:"borrowLimitHold"`
	InterestFreeBorrowLimit string `json:"interestFreeBorrowLimit"`
}

type InterestRow struct {
	Currency string `json:"currency"`
	Amount   string `json:"interest"`
	CreatedAt int64 `json:"createdAt"`
}

type Book struct {
	mu sync.Mutex
	mode  map[string]string
	lev   map[string]Leverage
	hist  map[string][]InterestRow
	liab  map[string][]LiabilityRow
	repay map[string][]RepayRow
	risk       map[string]RiskView
	cross      map[string][]CrossAsset
	collateral map[string]CollateralRule
	rateCatalog *RateCatalog
	kv          persist.KV
	sql         *persistSQL
	hydrateErr string
}

   func New() *Book {
	   b := &Book{
		   mode: map[string]string{}, lev: map[string]Leverage{}, hist: map[string][]InterestRow{},
		  liab: map[string][]LiabilityRow{}, repay: map[string][]RepayRow{}, risk: map[string]RiskView{},
		   cross: map[string][]CrossAsset{}, collateral: map[string]CollateralRule{},
	   }
	  b.rateCatalog = NewRateCatalog()
	  return b
  }

   func (b *Book) Mode(userID string) string {
	  mode, err := b.RequireMode(userID)
	  if err != nil {
		  return ""
	  }
	  return mode
   }

   func (b *Book) SetMode(userID, mode string) (string, error) {
	  userID = strings.TrimSpace(userID)
	   mode = strings.ToUpper(strings.TrimSpace(mode))
	  if mode != "CLASSIC" {
		   mode = "UNIFIED"
	  }
	  if err := b.persistMode(userID, mode); err != nil {
		  return "", err
	  }
	   b.mu.Lock()
	   b.mode[userID] = mode
	   b.mu.Unlock()
	   return mode, nil
   }

   func (b *Book) SetLeverage(userID string, in Leverage) (Leverage, error) {
	  userID = strings.TrimSpace(userID)
	  in.Symbol = strings.ToUpper(strings.TrimSpace(in.Symbol))
	   in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	   in.MarginMode = strings.ToUpper(strings.TrimSpace(in.MarginMode))
	  in.TradeType = strings.ToUpper(strings.TrimSpace(in.TradeType))
	   if in.Leverage == "" {
		   in.Leverage = "1"
	   }
	  if in.MarginMode == "" {
		  in.MarginMode = "CROSS"
	   }
	   key := userID + "|" + in.Symbol + "|" + in.Currency + "|" + in.MarginMode + "|" + in.TradeType
	  if err := b.persistLev(userID, in); err != nil {
		  return Leverage{}, err
	  }
	   b.mu.Lock()
	  b.lev[key] = in
	  b.mu.Unlock()
	  return in, nil
  }

  func (b *Book) Leverage(userID, symbol, currency, tradeType, marginMode string) []Leverage {
	  userID = strings.TrimSpace(userID)
	   symbol = strings.ToUpper(strings.TrimSpace(symbol))
	  currency = strings.ToUpper(strings.TrimSpace(currency))
	   tradeType = strings.ToUpper(strings.TrimSpace(tradeType))
	  marginMode = strings.ToUpper(strings.TrimSpace(marginMode))
	  b.mu.Lock()
	  defer b.mu.Unlock()
	  out := make([]Leverage, 0)
	  for k, v := range b.lev {
		   if !strings.HasPrefix(k, userID+"|") {
			   continue
		  }
		  if symbol != "" && v.Symbol != symbol {
			   continue
		   }
		   if currency != "" && v.Currency != currency {
			   continue
		   }
		   if tradeType != "" && v.TradeType != tradeType {
			   continue
		  }
		  if marginMode != "" && v.MarginMode != marginMode {
			  continue
		   }
		   out = append(out, v)
	  }
	  return out
  }

   func (b *Book) Borrowable() []string {
	   if b == nil {
		   return []string{}
	   }
	   return b.RateCatalog().BorrowableCurrencies()
  }

   func (b *Book) InterestLimits(currency string) InterestLimit {
	  lim, err := b.RequireInterestLimits(currency)
	  if err != nil {
		  return zeroInterestLimit()
	  }
	  return lim
   }

  func (b *Book) InterestHistory(userID, currency string) []InterestRow {
	  userID = strings.TrimSpace(userID)
	  currency = strings.ToUpper(strings.TrimSpace(currency))
	  if rows, ok := b.interestFromSQL(userID, currency); ok {
		  return rows
	  }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	   rows := b.hist[userID]
	  if currency == "" {
		   if rows == nil {
			   return []InterestRow{}
		   }
		   return rows
	  }
	   out := make([]InterestRow, 0)
	  for _, r := range rows {
		  if r.Currency == currency {
			  out = append(out, r)
		   }
	   }
	   return out
   }

  func (b *Book) NoteInterest(userID, currency, amount string) error {
	   userID = strings.TrimSpace(userID)
	   currency = strings.ToUpper(strings.TrimSpace(currency))
	  if userID == "" || currency == "" {
		  return nil
	  }
	   row := InterestRow{Currency: currency,
		  Amount: amount, CreatedAt: time.Now().UTC().UnixMilli()}
	  if err := b.persistInterest(userID, row); err != nil {
		  return err
	  }
	  b.mu.Lock()
	  b.hist[userID] = append(b.hist[userID], row)
	  b.mu.Unlock()
	  return nil
  }

   func zeroInterestLimit() InterestLimit {
	  return InterestLimit{
		   CurrentRateHourly: "0", CurrentRateDaily: "0",
		  BorrowLimitTotal: "0", BorrowLimitTotalHold: "0",
		  BorrowLimitHold: "0", InterestFreeBorrowLimit: "0",
	   }
   }

// Health 返回借还读面装载错误（若有）；供就绪探针聚合。

  func (b *Book) Health() string {
	   if b == nil {
		   return ""
	  }
	  b.mu.Lock()
	   defer b.mu.Unlock()
	   return b.hydrateErr
  }
