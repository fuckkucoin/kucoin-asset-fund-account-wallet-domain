// 现货订单簿。
// 作者: Span
// 日期: 2021.01.19
package spot

import (
	"sort"
	"sync"
	"time"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// book 价格时间先订单簿。
type book struct {
	mu sync.Mutex
	asks map[string][]*Order // price -> FIFO
	bids map[string][]*Order
	byID map[string]string // order id -> symbol side marker
	trades []Trade
}

  func newBook() *book {
	   return &book{asks: map[string][]*Order{}, bids: map[string][]*Order{}, byID: map[string]string{}}
  }

  func (b *book) add(o *Order) {
	   if o == nil || o.remainSize().IsZero() {
		  return
	   }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	  b.addLocked(o)
   }

   func (b *book) addLocked(o *Order) {
	  px := o.Price.String()
	   cp := *o
	   if o.Side == SideSell {
		   b.asks[px] = append(b.asks[px], &cp)
	   } else {
		   b.bids[px] = append(b.bids[px], &cp)
	  }
	   b.byID[o.ID] = string(o.Side)
  }

   func (b *book) remove(id string) {
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   b.removeLocked(id)
   }

   func (b *book) removeLocked(id string) {
	   delete(b.byID, id)
	  for px, q := range b.asks {
		  b.asks[px] = dropID(q, id)
		  if len(b.asks[px]) == 0 {
			  delete(b.asks, px)
		  }
	  }
	  for px, q := range b.bids {
		   b.bids[px] = dropID(q, id)
		  if len(b.bids[px]) == 0 {
			   delete(b.bids, px)
		  }
	   }
  }

   func dropID(q []*Order, id string) []*Order {
	  out := q[:0]
	  for _, o := range q {
		  if o != nil && o.ID != id {
			   out = append(out, o)
		   }
	   }
	   return out
  }

type matchHit struct {
	maker *Order
	qty   decimal.Decimal
	px    decimal.Decimal
}

   func (b *book) take(taker *Order) []matchHit {
	  if taker == nil {
		  return nil
	  }
	  b.mu.Lock()
	  defer b.mu.Unlock()
	  need := taker.remainSize()
	   var hits []matchHit
	  if taker.Side == SideBuy {
		   prices := sortPrices(b.asks,
			  true)
		  for _, px := range prices {
			   if taker.Type == TypeLimit && px.GreaterThan(taker.Price) {
				  break
			   }
			   q := b.asks[px.String()]
			  for i := 0; i < len(q) && need.IsPositive(); i++ {
				   m := q[i]
				  if m == nil || m.UserID == taker.UserID {
					  continue
				   }
				  qty := decimal.Min(need, m.remainSize())
				  if qty.IsZero() {
					   continue
				   }
				  hits = append(hits, matchHit{maker: m, qty: qty, px: px})
				   m.Filled = m.Filled.Add(qty)
				   need = need.Sub(qty)
			  }
			   b.asks[px.String()] = compactRemain(q)
			   if len(b.asks[px.String()]) == 0 {
				   delete(b.asks, px.String())
			   }
		   }
	  } else {
		   prices := sortPrices(b.bids, false)
		   for _, px := range prices {
			  if taker.Type == TypeLimit && px.LessThan(taker.Price) {
				  break
			  }
			   q := b.bids[px.String()]
			  for i := 0; i < len(q) && need.IsPositive(); i++ {
				   m := q[i]
				  if m == nil || m.UserID == taker.UserID {
					   continue
				  }
				  qty := decimal.Min(need, m.remainSize())
				   if qty.IsZero() {
					   continue
				   }
				  hits = append(hits, matchHit{maker: m, qty: qty, px: px})
				  m.Filled = m.Filled.Add(qty)
				   need = need.Sub(qty)
			   }
			   b.bids[px.String()] = compactRemain(q)
			  if len(b.bids[px.String()]) == 0 {
				   delete(b.bids, px.String())
			  }
		   }
	   }
	   return hits
   }

  func compactRemain(q []*Order) []*Order {
	  out := q[:0]
	  for _, o := range q {
		  if o != nil && o.remainSize().IsPositive() {
			   out = append(out, o)
		   } else if o != nil {
			   // fully filled; drop
		  }
	  }
	  return out
  }

  func sortPrices(m map[string][]*Order, asc bool) []decimal.Decimal {
	   out := make([]decimal.Decimal, 0, len(m))
	   for k := range m {
		   d, err := decimal.NewFromString(k)
		   if err != nil {
			  continue
		  }
		   out = append(out, d)
	  }
	   sort.Slice(out, func(i, j int) bool {
		   if asc {
			   return out[i].LessThan(out[j])
		  }
		  return out[i].GreaterThan(out[j])
	   })
	  return out
   }

  func (b *book) record(t Trade) {
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  b.trades = append(b.trades, t)
	   if len(b.trades) > 200 {
		  b.trades = b.trades[len(b.trades)-200:]
	  }
  }

   func (b *book) lastTrades(n int) []Trade {
	   b.mu.Lock()
	  defer b.mu.Unlock()
	  if n <= 0 || n > len(b.trades) {
		   n = len(b.trades)
	   }
	  if n == 0 {
		   return []Trade{}
	  }
	  out := make([]Trade, n)
	  copy(out, b.trades[len(b.trades)-n:])
	  return out
   }

  func (b *book) depth(symbol string, n int) Depth {
	   if n <= 0 {
		  n = 10
	   }
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   sumSide := func(m map[string][]*Order, side Side) []DepthLevel {
		   var prices []decimal.Decimal
		  for k := range m {
			   d, err := decimal.NewFromString(k)
			  if err != nil {
				  continue
			  }
			  prices = append(prices, d)
		   }
		   asc := side == SideSell
		  sort.Slice(prices, func(i, j int) bool {
			   if asc {
				  return prices[i].LessThan(prices[j])
			   }
			  return prices[i].GreaterThan(prices[j])
		  })
		   out := []DepthLevel{}
		   for _, px := range prices {
			   q := m[px.String()]
			   sz := decimal.Zero
			  c := 0
			  for _, o := range q {
				   if o == nil || (symbol != "" && o.Symbol != symbol) {
					   continue
				   }
				   r := o.remainSize()
				   if r.IsZero() {
					  continue
				   }
				  sz = sz.Add(r)
				  c++
			   }
			  if c == 0 {
				   continue
			  }
			   out = append(out, DepthLevel{Price: px.String(), Size: sz.String(), Orders: c})
			  if len(out) >= n {
				   break
			   }
		   }
		   return out
	   }
	   return Depth{
		   Symbol: symbol,
		  Bids: sumSide(b.bids, SideBuy),
		  Asks: sumSide(b.asks, SideSell),
		  Note: "in-process CLOB; not a matching-engine cluster",
	   }
  }

   func newTrade(symbol string, taker, maker *Order, qty, px decimal.Decimal) Trade {
	  return Trade{
		  ID: uuid.NewString(),
		   Symbol: symbol, Price: px.String(), Size: qty.String(),
		   Side: string(taker.Side), TakerOrder: taker.ID, MakerOrder: maker.ID,
		   TakerUser: taker.UserID, MakerUser: maker.UserID, Ts: time.Now().UTC().UnixMilli(),
	  }
  }
