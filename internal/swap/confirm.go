package swap

import (
	"context"
	"strings"
	"sync"
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/quote"
	"account-assets.kucoin.net/funding/internal/quotemultigate"
)

// SwapLedger 兑换入账面。失败不得调用。

type SwapLedger interface {
	WriteSwap(ctx context.Context,
		userID, from, to, idem string, fromAmt, toAmt decimal.Decimal) error
}

// RecSwapLedger 记录写入次数。
type RecSwapLedger struct {
	mu     sync.Mutex
	Writes int
}

   func (r *RecSwapLedger) WriteSwap(_ context.Context, _, _, _, _ string, _, _ decimal.Decimal) error {
	   if r == nil {
		   return ErrSwapLedger
	   }
	  r.mu.Lock()
	  defer r.mu.Unlock()
	  r.Writes++
	   return nil
  }

  func (r *RecSwapLedger) Count() int {
	  if r == nil {
		  return 0
	   }
	  r.mu.Lock()
	   defer r.mu.Unlock()
	  return r.Writes
   }

// ErrSwapLedger 确认成功路径缺少入账面。
var ErrSwapLedger = errSwapLedgerMissing

var errSwapLedgerMissing = quote.ErrQuoteIllegal

// ConfirmReq 兑换确认。

type ConfirmReq struct {
	UserID string
	From   string
	To     string
	Amount decimal.Decimal
	Idem   string
	Now    time.Time
	Quote *quote.Quote
	Multi quotemultigate.Bundle
}

// ConfirmResult 兑换确认结果。

type ConfirmResult struct {
	UserID string `json:"user_id"`
	From   string `json:"from"`
	To     string `json:"to"`
	FromAmt decimal.Decimal `json:"from_amt"`
	ToAmt   decimal.Decimal `json:"to_amt"`
	Rate decimal.Decimal `json:"rate"`
	Source  string `json:"source"`
	Applied bool   `json:"applied"`
}

// Gate 确认闸：只接受簿内新鲜报价。
type Gate struct {
	book  *quote.Book
	clock quote.Clock
	ld    SwapLedger
	multi *quotemultigate.Panel
}

  func NewGate(clock quote.Clock, ttl time.Duration, ld SwapLedger) *Gate {
	   if quote.NormalizeTTL(ttl) == 0 {
		   ttl = quote.DefaultSwapTTL
	  }
	  return &Gate{book: quote.NewBook(ttl, clock), clock: clock, ld: ld}
   }

// BindMulti 挂载多源一致性面板。档未附带采集束则拒绝确认。
   func (g *Gate) BindMulti(p *quotemultigate.Panel) {
	  if g != nil {
		   g.multi = p
	  }
  }

   func (g *Gate) Put(q quote.Quote) error {
	  if g == nil || g.book == nil {
		  return quote.ErrQuoteEmpty
	  }
	   key := q.Pair
	   if key == "" {
		  return quote.ErrPairRequired
	   }
	   return g.book.Put(key, q)
   }

   func (g *Gate) Delete(pair string) {
	  if g != nil && g.book != nil {
		  g.book.Delete(pair)
	  }
   }

// ConfirmSwap 确认前 GuardConfirm。空/过期不写 ledger。
   func (g *Gate) ConfirmSwap(ctx context.Context, req ConfirmReq) (ConfirmResult, error) {
	   var zero ConfirmResult
	  if g == nil {
		   return zero, quote.ErrQuoteEmpty
	   }
	  if strings.TrimSpace(req.UserID) == "" || !req.Amount.IsPositive() {
		   return zero, quote.ErrQuoteIllegal
	   }
	  now, err := quote.ResolveNow(g.clock, req.Now)
	   if err != nil {
		   return zero, err
	  }
	  var q quote.Quote
	   var dec quotemultigate.Decision
	  usedMulti := req.Multi.Active()
	   if usedMulti {
		  req.Multi.Idem = quotemultigate.Coalesce(req.Multi.Idem, req.Idem)
		   if req.Multi.Key == "" {
			  req.Multi.Key = quote.PairKey(req.From, req.To)
		  }
		   dec,
			   err = quotemultigate.OrPanel(g.multi).Decide(req.Multi, now)
		  if err != nil {
			   return zero, err
		  }
		  q = dec.Quote
	  } else if g.multi != nil && g.multi.NeedBundle() {
		  return zero, quote.ErrQuoteTimeout
	   } else if req.Quote != nil {
		  q = *req.Quote
		   if err := quote.GuardConfirm(q, now); err != nil {
			   return zero, err
		  }
	  } else {
		  key := quote.PairKey(req.From, req.To)
		   q,
			   err = g.book.GetGuarded(key, now)
		  if err != nil {
			  return zero, err
		   }
	  }
	   if g.ld == nil {
		   return zero, ErrSwapLedger
	  }
	   toAmt := req.Amount.Mul(q.Rate)
	  idem := strings.TrimSpace(req.Idem)
	   if idem == "" {
		   idem = req.UserID + "|" + q.Pair
	   }
	   if usedMulti && dec.Reentry {
		  return ConfirmResult{
			  UserID: req.UserID,
			   From:   quote.AssetKey(req.From),
			   To:      quote.AssetKey(req.To),
			  FromAmt: req.Amount,
			   ToAmt:  toAmt,
			  Rate:   q.Rate,
			  Source: q.Source,
			  Applied: true,
		   }, nil
	  }
	   if err := g.ld.WriteSwap(ctx, req.UserID, quote.AssetKey(req.From), quote.AssetKey(req.To), idem, req.Amount, toAmt); err != nil {
		   return zero, err
	   }
	   if usedMulti {
		   quotemultigate.OrPanel(g.multi).Commit(idem, q.Pair, q.Rate.String(), dec.Fingerprint)
	  }
	   return ConfirmResult{
		  UserID:  req.UserID,
		   From:    quote.AssetKey(req.From),
		  To:      quote.AssetKey(req.To),
		   FromAmt: req.Amount,
		   ToAmt:   toAmt,
		   Rate:   q.Rate,
		   Source: q.Source,
		  Applied: true,
	   }, nil
  }
