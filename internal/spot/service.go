// 现货撮合入口。
// 作者: King
// 日期: 2020.12.01
package spot

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/metrics"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/swap"
)

// Service 现货订单：价格时间先撮合；空簿市价 IOC 按最新成交托底。
type Service struct {
	st     ledger.Store
	lock   ledger.Locker
	pricer swap.Pricer
	feeBPS int
	lockTTL time.Duration
	memMu   sync.Mutex
	mem     *orderBag
	book    *book
	OnBooks func(ctx context.Context, userID string)
	OnPrint func(symbol, // 成交价回灌行情，钱包页跟盘口同一份
		   price string)
	Guard func(userID string) error
}

// New 现货撮合面：冻资金账、价格时间先成交；空簿市价按最新成交托底。

   func New(st ledger.Store, lock ledger.Locker, pricer swap.Pricer, cfg config.SpotConfig) *Service {
	  if pricer == nil {
		   pricer = swap.DefaultPricer()
	   }
	  fee := cfg.FeeBPS
	  if fee < 0 {
		   fee = 0
	  }
	   return &Service{st: st, lock: lock, pricer: pricer, feeBPS: fee, lockTTL: 12 * time.Second, mem: &orderBag{byID: map[string]*Order{}, byIdem: map[string]string{}}, book: newBook()}
   }

// Symbols 钱包页可交易对，和行情 ticker 同一套。
   func (s *Service) Symbols() []map[string]string {
	  return []map[string]string{
		  {"symbol": "BTC-USDT", "base": "BTC", "quote": "USDT"},
		  {"symbol": "ETH-USDT", "base": "ETH", "quote": "USDT"},
		   {"symbol": "SOL-USDT", "base": "SOL", "quote": "USDT"},
		  {"symbol": "TRX-USDT", "base": "TRX", "quote": "USDT"},
		   {"symbol": "TON-USDT", "base": "TON", "quote": "USDT"},
	   }
   }

// Place 下单入口：幂等键命中则返回原单，否则冻资金账再进盘口。
  func (s *Service) Place(ctx context.Context, req PlaceReq) (*Order, error) {
	   if strings.TrimSpace(req.IdempotencyKey) == "" {
		  return nil, ledger.ErrMissingIdempotency
	  }
	  if existing, err := s.getByIdem(ctx, req.IdempotencyKey); err == nil {
		   if err := sameSpotIntent(existing, req); err != nil {
			  return existing, err
		  }
		  return existing, nil
	   }
	   base, quote, err := parseSymbol(req.Symbol)
	   if err != nil {
		   return nil, err
	  }
	  side := Side(strings.ToLower(strings.TrimSpace(req.Side)))
	  if side != SideBuy && side != SideSell {
		   return nil, ErrSide
	   }
	   typ := Type(strings.ToLower(strings.TrimSpace(req.Type)))
	   if typ == "" {
		   typ = TypeMarket
	   }
	   if typ != TypeMarket && typ != TypeLimit {
		   return nil, ErrType
	   }
	   size, err := ledger.ParseAmount(req.Size)
	   if err != nil {
		  return nil, err
	  }
	  if size.LessThanOrEqual(decimal.Zero) {
		  return nil, ledger.ErrInvalidAmount
	   }
	   if s.Guard != nil {
		  if err := s.Guard(req.UserID); err != nil {
			  return nil, err
		  }
	  }
	  last, ok := s.pricer.Rate(base, quote)
	  if !ok || last.IsZero() {
		   return nil, ledger.ErrStaleQuote
	   }
	   px := last
	  if typ == TypeLimit {
		   if strings.TrimSpace(req.Price) == "" {
			   return nil, ErrLimitPrice
		   }
		   px, err = ledger.ParseAmount(req.Price)
		   if err != nil {
			   return nil, err
		  }
		   if px.LessThanOrEqual(decimal.Zero) {
			   return nil, ledger.ErrInvalidAmount
		  }
	   }
	  quoteAmt := size.Mul(px)
	  fee := quoteAmt.Mul(decimal.NewFromInt(int64(s.feeBPS))).Div(decimal.NewFromInt(10000))
	   ord := &Order{
		  ID: uuid.NewString(), IdempotencyKey: req.IdempotencyKey, UserID: req.UserID,
		   Symbol: strings.ToUpper(strings.TrimSpace(req.Symbol)),
		  Base:   base, Quote: quote,
		   Side: side, Type: typ, Size: size, Price: px, QuoteAmount: quoteAmt,
		   FeeAmount: fee, FeeAsset: quote, Status: StatusOpen, CreatedAt: time.Now().UTC(),
	   }
	  lease, err := s.lock.Lock(ctx, ledger.UserLockKey(req.UserID), s.lockTTL)
	  if err != nil {
		   return nil, err
	   }
	   defer lease.Unlock()
	   err = s.st.InTx(ctx, func(ctx context.Context) error {
		   if existing, e2 := s.getByIdem(ctx, req.IdempotencyKey); e2 == nil {
			   *ord = *existing
			  return ledger.ErrIdempotency
		  }
		  hold := ord.holdAsset()
		  need := ord.holdAmount()
		  src, err := s.st.GetAccount(ctx, req.UserID, ledger.AccountSpot, hold)
		   if err != nil {
			   return err
		   }
		   if src.Available.LessThan(need) {
			  ord.Status = StatusRejected
			   ord.FailReason = ledger.ErrInsufficient.Error()
			  metrics.SpotOrders.WithLabelValues(string(ord.Status)).Inc()
			   _ = s.insertOrder(ctx, ord)
			   return ledger.ErrInsufficient
		   }
		   if err := s.insertOrder(ctx, ord); err != nil {
			  return err
		   }
		   lines := freezeSpot(ord.ID, req.UserID, hold, need)
		  if err := s.st.AppendJournal(ctx, lines); err != nil {
			  return err
		  }
		   for _, e := range lines {
			   if err := ledger.ApplyJournalToAccount(src, e); err != nil {
				  return err
			   }
		  }
		  return s.st.UpsertAccount(ctx,
			  src)
	   })
	   if errors.Is(err, ledger.ErrIdempotency) {
		  return ord, nil
	  }
	   if err != nil {
		   if ord.Status == StatusRejected {
			  return ord, err
		   }
		  return nil, err
	  }
	  if err := s.matchAndSettle(ctx, ord); err != nil {
		  return ord, err
	  }
	  got, err := s.getByID(ctx, ord.ID)
	  if err != nil {
		   return ord, err
	   }
	  if got.Open() && got.Type == TypeLimit && got.remainSize().IsPositive() {
		  s.book.add(got)
	   }
	   if got.Type == TypeMarket && got.remainSize().IsPositive() && got.Open() {
		  _ = s.stampOrder(ctx, got, lease)
		  _ = s.fillLocked(ctx, got)
		  got2, _ := s.getByID(ctx, ord.ID)
		   if got2 != nil && got2.remainSize().IsPositive() && got2.Open() {
			  _ = s.unfreezeLocked(ctx, got2, "ioc remainder")
		   }
		   got, _ = s.getByID(ctx, ord.ID)
	   }
	  if s.OnBooks != nil {
		  s.OnBooks(ctx, req.UserID)
	   }
	  return got, nil
   }

  func (o *Order) holdAsset() string {
	   if o.Side == SideBuy {
		   return o.Quote
	  }
	  return o.Base
   }

  func (o *Order) holdAmount() decimal.Decimal {
	   if o.Side == SideBuy {
		  return o.QuoteAmount.Add(o.FeeAmount)
	   }
	   return o.Size
   }

  func (o *Order) marketable(last decimal.Decimal) bool {
	   if o.Type == TypeMarket {
		  return true
	   }
	  if o.Side == SideBuy {
		   return !last.GreaterThan(o.Price)
	   }
	   return !last.LessThan(o.Price)
   }

   func freezeSpot(id, userID, asset string, amt decimal.Decimal) []ledger.JournalEntry {
	   return []ledger.JournalEntry{
		  {TransferID: id, Seq: 1, UserID: userID, AccountType: ledger.AccountSpot, Asset: asset, Debit: amt, Memo: "freeze_available"},
		   {TransferID: id, Seq: 2, UserID: userID, AccountType: ledger.AccountSpot, Asset: asset, Credit: amt, Memo: "freeze_frozen"},
	   }
   }

  func sameSpotIntent(o *Order, req PlaceReq) error {
	   if req.Symbol != "" && !strings.EqualFold(o.Symbol, req.Symbol) {
		   return ledger.ErrIdempotencyConflict
	   }
	   if req.Side != "" && string(o.Side) != strings.ToLower(strings.TrimSpace(req.Side)) {
		   return ledger.ErrIdempotencyConflict
	   }
	  if strings.TrimSpace(req.Size) != "" {
		  amt, err := ledger.ParseAmount(req.Size)
		   if err != nil {
			   return err
		  }
		   if !o.Size.Equal(amt) {
			   return ledger.ErrIdempotencyConflict
		  }
	   }
	   return nil
  }

   func (s *Service) Get(ctx context.Context, id string) (*Order, error) {
	  return s.getByID(ctx, id)
  }

   func (s *Service) List(ctx context.Context, userID string) ([]*Order, error) {
	  return s.listOrders(ctx, userID)
  }

// Cancel 撤未完成单并解冻剩余占用；已成交部分不回滚。

   func (s *Service) Cancel(ctx context.Context, id string) (*Order, error) {
	  cur, err := s.getByID(ctx, id)
	   if err != nil {
		  return nil, err
	  }
	  if cur.Status == StatusCancelled || cur.Status == StatusRejected {
		   return cur, nil
	  }
	   if cur.Status != StatusOpen && cur.Status != StatusPartial {
		  return cur, ledger.ErrCannotCancel
	  }
	  lease, err := s.lock.Lock(ctx, ledger.UserLockKey(cur.UserID), s.lockTTL)
	   if err != nil {
		   return nil, err
	  }
	   defer lease.Unlock()
	   if err := s.stampOrder(ctx, cur, lease); err != nil {
		  return nil, err
	   }
	  s.book.remove(cur.ID)
	   if err := s.unfreezeLocked(ctx, cur, "user_cancel"); err != nil {
		  return cur, err
	  }
	   got, _ := s.getByID(ctx, cur.ID)
	  if s.OnBooks != nil {
		   s.OnBooks(ctx, cur.UserID)
	  }
	  return got, nil
  }

// Depth 盘口快照，给钱包页深度。
   func (s *Service) Depth(symbol string) Depth {
	   if s.book == nil {
		  return Depth{Symbol: symbol, Note: "no book"}
	  }
	   return s.book.depth(strings.ToUpper(strings.TrimSpace(symbol)),
		  20)
  }

  func (s *Service) Trades(limit int) []Trade {
	   if s.book == nil {
		   return []Trade{}
	  }
	  return s.book.lastTrades(limit)
  }

// matchAndSettle 吃单后按命中切片入账，部分成交留下剩余占用。

  func (s *Service) matchAndSettle(ctx context.Context, taker *Order) error {
	  if s.book == nil || taker == nil {
		   return nil
	   }
	  hits := s.book.take(taker)
	  for _, h := range hits {
		  if err := s.settleMatch(ctx, taker.ID, h); err != nil {
			   return err
		  }
	  }
	   return nil
  }

// FillOpen worker 扫可成交挂单，空簿市价按最新成交托底。
   func (s *Service) FillOpen(ctx context.Context) {
	   list, err := s.listOpen(ctx)
	  if err != nil {
		   return
	  }
	   for _, o := range list {
		  _ = s.matchAndSettle(ctx, o)
		  got, err := s.getByID(ctx, o.ID)
		  if err != nil || got == nil || !got.Open() {
			   continue
		  }
		  if got.Type != TypeMarket {
			  continue
		   }
		   last,
			  ok := s.pricer.Rate(got.Base, got.Quote)
		  if !ok || last.IsZero() {
			   continue
		  }
		   lease, err := s.lock.Lock(ctx, ledger.UserLockKey(got.UserID), s.lockTTL)
		   if err != nil {
			  continue
		   }
		  _ = s.stampOrder(ctx, got, lease)
		  _ = s.fillLocked(ctx, got)
		  lease.Unlock()
		  if s.OnBooks != nil {
			   s.OnBooks(ctx, got.UserID)
		  }
	   }
   }

   func (s *Service) settleMatch(ctx context.Context, takerID string, h matchHit) error {
	  if h.maker == nil || h.qty.IsZero() {
		   return nil
	   }
	   return s.st.InTx(ctx, func(ctx context.Context) error {
		   taker,
			   err := s.getByID(ctx, takerID)
		  if err != nil {
			   return err
		   }
		   maker, err := s.getByID(ctx, h.maker.ID)
		  if err != nil {
			   return err
		   }
		   if !taker.Open() || !maker.Open() {
			   return nil
		   }
		  qty := h.qty
		  if qty.GreaterThan(taker.remainSize()) {
			   qty = taker.remainSize()
		  }
		  if qty.GreaterThan(maker.remainSize()) {
			   qty = maker.remainSize()
		  }
		   if qty.IsZero() {
			   return nil
		   }
		  px := h.px
		  quote := qty.Mul(px)
		  if err := s.fillSlice(ctx, taker, qty, quote, true); err != nil {
			  return err
		   }
		   if err := s.fillSlice(ctx, maker, qty, quote, false); err != nil {
			   return err
		  }
		   s.book.record(newTrade(taker.Symbol, taker, maker, qty, px))
		   s.emitPrint(taker.Symbol, px.String())
		  if s.OnBooks != nil {
			   s.OnBooks(ctx, taker.UserID)
			  if maker.UserID != taker.UserID {
				  s.OnBooks(ctx, maker.UserID)
			   }
		  }
		   return nil
	   })
  }

// fillSlice 成交一片：扣冻结、入目标资产、记手续费，资金账必须和订单状态对上。
   func (s *Service) fillSlice(ctx context.Context, o *Order, qty, quote decimal.Decimal, isTaker bool) error {
	   prev, err := s.st.ListJournal(ctx, o.ID)
	  if err != nil {
		  return err
	  }
	  seq := ledger.NextJournalSeq(prev)
	  hold := o.holdAsset()
	   var need, creditAmt decimal.Decimal
	   var dstAsset string
	   if o.Side == SideBuy {
		   need = quote
		   if isTaker && !o.Size.IsZero() && !o.FeeAmount.IsZero() {
			   need = need.Add(o.FeeAmount.Mul(qty).Div(o.Size))
		   }
		   creditAmt = qty
		   dstAsset = o.Base
	  } else {
		   need = qty
		  creditAmt = quote
		   if !isTaker && !o.Size.IsZero() && !o.FeeAmount.IsZero() {
			   creditAmt = quote.Sub(o.FeeAmount.Mul(qty).Div(o.Size))
		  }
		  dstAsset = o.Quote
	  }
	   src, err := s.st.GetAccount(ctx, o.UserID, ledger.AccountSpot, hold)
	  if err != nil {
		  return err
	   }
	  dst, err := s.st.GetAccount(ctx, o.UserID, ledger.AccountSpot, dstAsset)
	  if err != nil {
		   return err
	   }
	   deskFrom, err := s.st.GetAccount(ctx, "system", ledger.AccountBridge, hold)
	   if err != nil {
		   return err
	   }
	   deskTo, err := s.st.GetAccount(ctx, "system", ledger.AccountBridge, dstAsset)
	   if err != nil {
		   return err
	   }
	  lines := []ledger.JournalEntry{
		   {TransferID: o.ID, Seq: seq, UserID: o.UserID, AccountType: ledger.AccountSpot, Asset: hold, Debit: need, Memo: "consume_frozen"},
		   {TransferID: o.ID, Seq: seq + 1, UserID: "system", AccountType: ledger.AccountBridge, Asset: hold, Credit: need, Memo: "bridge_in"},
		  {TransferID: o.ID, Seq: seq + 2, UserID: "system", AccountType: ledger.AccountBridge, Asset: dstAsset, Debit: creditAmt, Memo: "bridge_out"},
		  {TransferID: o.ID, Seq: seq + 3, UserID: o.UserID, AccountType: ledger.AccountSpot, Asset: dstAsset, Credit: creditAmt, Memo: "credit_available"},
	   }
	  if err := s.st.AppendJournal(ctx, lines); err != nil {
		  return err
	   }
	   if err := ledger.ApplyJournalToAccount(src, lines[0]); err != nil {
		  return err
	  }
	  if err := ledger.ApplyJournalToAccount(deskFrom, lines[1]); err != nil {
		  return err
	   }
	  if err := ledger.ApplyJournalToAccount(deskTo, lines[2]); err != nil {
		  return err
	  }
	   if err := ledger.ApplyJournalToAccount(dst, lines[3]); err != nil {
		  return err
	  }
	   for _, a := range []*ledger.Account{src, dst, deskFrom, deskTo} {
		   if err := s.st.UpsertAccount(ctx, a); err != nil {
			   return err
		   }
	   }
	  o.Filled = o.Filled.Add(qty)
	  if o.remainSize().IsZero() {
		   o.Status = StatusFilled
	  } else {
		  o.Status = StatusPartial
	   }
	  metrics.SpotOrders.WithLabelValues(string(o.Status)).Inc()
	   return s.updateOrder(ctx, o)
  }

   func (s *Service) fillLocked(ctx context.Context, ord *Order) error {
	  return s.st.InTx(ctx, func(ctx context.Context) error {
		  cur, err := s.getByID(ctx, ord.ID)
		  if err != nil {
			  return err
		  }
		  if cur.Status == StatusFilled || cur.Status == StatusCancelled {
			   *ord = *cur
			  return nil
		   }
		  if !cur.Open() {
			   return fmt.Errorf("spot: cannot fill from %s", cur.Status)
		  }
		  if cur.remainSize().IsZero() {
			  cur.Status = StatusFilled
			   *ord = *cur
			  return s.updateOrder(ctx, cur)
		   }
		   qty := cur.remainSize()
		   quote := qty.Mul(cur.Price)
		  if err := s.fillSlice(ctx, cur, qty, quote, true); err != nil {
			  return err
		  }
		  s.emitPrint(cur.Symbol, cur.Price.String())
		   *ord = *cur
		   return nil
	  })
   }

   func (s *Service) emitPrint(symbol, price string) {
	  if s == nil || s.OnPrint == nil || symbol == "" || price == "" || price == "0" {
		  return
	  }
	   s.OnPrint(symbol, price)
   }

  func (s *Service) unfreezeLocked(ctx context.Context, ord *Order, reason string) error {
	   return s.st.InTx(ctx, func(ctx context.Context) error {
		  cur,
			  err := s.getByID(ctx, ord.ID)
		   if err != nil {
			  return err
		   }
		   if !cur.Open() {
			  *ord = *cur
			   return nil
		  }
		  prev, err := s.st.ListJournal(ctx, cur.ID)
		  if err != nil {
			   return err
		   }
		  need := cur.remainHold()
		  if need.IsZero() {
			  cur.Status = StatusCancelled
			  cur.FailReason = reason
			  *ord = *cur
			   return s.updateOrder(ctx, cur)
		   }
		  hold := cur.holdAsset()
		   seq := ledger.NextJournalSeq(prev)
		  lines := []ledger.JournalEntry{
			  {TransferID: cur.ID, Seq: seq, UserID: cur.UserID, AccountType: ledger.AccountSpot, Asset: hold, Debit: need, Memo: "unfreeze_frozen"},
			   {TransferID: cur.ID, Seq: seq + 1, UserID: cur.UserID, AccountType: ledger.AccountSpot, Asset: hold, Credit: need, Memo: "restore_available"},
		   }
		  acc, err := s.st.GetAccount(ctx, cur.UserID, ledger.AccountSpot, hold)
		  if err != nil {
			   return err
		  }
		   if err := s.st.AppendJournal(ctx, lines); err != nil {
			   return err
		   }
		   for _, e := range lines {
			   if err := ledger.ApplyJournalToAccount(acc, e); err != nil {
				   return err
			   }
		  }
		   if err := s.st.UpsertAccount(ctx, acc); err != nil {
			   return err
		   }
		  cur.Status = StatusCancelled
		   cur.FailReason = reason
		   metrics.SpotOrders.WithLabelValues(string(cur.Status)).Inc()
		  *ord = *cur
		   return s.updateOrder(ctx, cur)
	   })
  }

// FrozenInflight 在途现货占用，给对账用，保证挂单冻结和资金账对得上。

   func (s *Service) FrozenInflight(ctx context.Context, userID string) (map[string]decimal.Decimal, []ledger.Mismatch, error) {
	   orders,
		   err := s.listOrders(ctx, userID)
	   if err != nil {
		   return nil, nil, err
	  }
	   extra := map[string]decimal.Decimal{}
	   var mm []ledger.Mismatch
	   for _, o := range orders {
		   j, err := s.st.ListJournal(ctx, o.ID)
		   if err != nil {
			   return nil, nil, err
		  }
		   hasFreeze,
			   hasUnfreeze, hasCredit := false, false, false
		   for _, e := range j {
			  switch e.Memo {
			   case "freeze_available", "freeze_frozen":
				  hasFreeze = true
			   case "unfreeze_frozen", "restore_available":
				  hasUnfreeze = true
			  case "consume_frozen":
				   hasCredit = true
			   }
		  }
		  switch o.Status {
		   case StatusOpen, StatusPartial:
			  k := string(ledger.AccountSpot) + "|" + o.holdAsset()
			   extra[k] = extra[k].Add(o.remainHold())
			   if !hasFreeze {
				   mm = append(mm, ledger.Mismatch{UserID: userID, TransferID: o.ID, State: string(o.Status), Asset: o.holdAsset(), Note: "open spot order missing freeze journal"})
			  }
		   case StatusFilled:
			  if !hasCredit {
				  mm = append(mm, ledger.Mismatch{UserID: userID, TransferID: o.ID, State: string(o.Status), Asset: o.holdAsset(), Note: "filled spot missing settlement journal"})
			   }
		  case StatusCancelled, StatusRejected:
			   if hasFreeze && !hasUnfreeze && !hasCredit {
				  mm = append(mm, ledger.Mismatch{UserID: userID, TransferID: o.ID, State: string(o.Status), Asset: o.holdAsset(), Note: "cancelled spot still frozen"})
			  }
		  }
	   }
	  return extra, mm, nil
   }
