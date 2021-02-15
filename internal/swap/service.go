// 链上兑换：报价、冻结、成交和手续费入账。
// 作者: Likeo
// 日期: 2021.01.13
package swap

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/events"
	"account-assets.kucoin.net/funding/internal/ledger"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/trace"
	"account-assets.kucoin.net/funding/internal/wallet"
)

// Service 链上兑换：冻钱包 → 链下 Submit → 入目标资产和手续费。
// 报价必须读现货同一套 ticker，禁止钱包页另开一本价。

type Service struct {
	st ledger.Store
	lock   ledger.Locker
	pricer Pricer
	feeBPS int
	ttl         time.Duration
	slippageBPS int
	quoteMaxAge time.Duration
	lockTTL time.Duration
	memMu   sync.Mutex
	mem     *orderBag
	OnBooks func(ctx context.Context, userID string)
	Submit    func(ctx context.Context, chain, to string, amount decimal.Decimal) (string, error) // 与账本共用 Registry
	Guard     func(userID string) error
	leaseHook *LeaseHook
}

// New 兑换服务：冻钱包 → 事务外 Submit → 入目标资产和手续费。报价必须读现货 ticker。
  func New(st ledger.Store, lock ledger.Locker, pricer Pricer, cfg config.SwapConfig) *Service {
	  if pricer == nil {
		   pricer = DefaultPricer()
	  }
	   ttl := cfg.QuoteTTL
	   if ttl == 0 {
		   ttl = 15 * time.Second
	   }
	   slip := cfg.SlippageBPS
	   if slip <= 0 {
		   slip = 50
	   }
	   age := cfg.QuoteMaxAge
	   if age <= 0 {
		   age = 2 * time.Second
	   }
	   return &Service{
		   st: st, lock: lock, pricer: pricer, feeBPS: cfg.FeeBPS, ttl: ttl, slippageBPS: slip, quoteMaxAge: age, lockTTL: 12 * time.Second,
		  mem: &orderBag{byID: map[string]*Order{},
			   byIdem: map[string]string{}},
	  }
  }

// Quote 询价：汇率必须来自现货同一套 ticker，过期行情直接拒，不另开一本价。
  func (s *Service) Quote(_ context.Context, from, to, amountStr string) (*Quote, error) {
	  amt, err := ledger.ParseAmount(amountStr)
	   if err != nil {
		   return nil, err
	  }
	  rate, src, ok := s.execRate(from, to)
	  if !ok {
		  metrics.QuoteRejected.WithLabelValues("stale").Inc()
		  return nil, ledger.ErrStaleQuote
	  }
	  gross := amt.Mul(rate)
	  fee := gross.Mul(decimal.NewFromInt(int64(s.feeBPS))).Div(decimal.NewFromInt(10000))
	   return &Quote{
		  FromAsset: from, ToAsset: to, FromAmount: amt.String(),
		  Rate: rate.String(), ToAmount: gross.Sub(fee).String(),
		  FeeAsset: to, FeeAmount: fee.String(),
		  ExpiresAt: time.Now().Add(s.ttl).Unix(), Source: src,
	   }, nil
   }

// Execute 冻 filledFrom、事务外 Submit、再 settle。hashed frozen 不得因行情过期自动解冻。
   func (s *Service) Execute(ctx context.Context, req ExecReq) (got *Order, err error) {
	   ctx, sp := trace.WithSpan(ctx, "swap.Execute")
	  defer func() { sp.End(zerolog.Nop(), err) }()
	   if req.IdempotencyKey == "" {
		   req.IdempotencyKey = uuid.NewString()
	   }
	  if existing, err := s.getByIdem(ctx, req.IdempotencyKey); err == nil {
		   if err := sameSwapIntent(existing, req); err != nil {
			  metrics.IdempotencyConflicts.Inc()
			  return existing, err
		  }
		   if existing.Status == StatusFrozen {
			   return s.finishFrozen(ctx, existing)
		   }
		   return existing, nil
	   }
	  fromAmt,
		  err := ledger.ParseAmount(req.FromAmount)
	  if err != nil {
		  return nil, err
	   }
	   if s.Guard != nil {
		   if err := s.Guard(req.UserID); err != nil {
			   return nil, err
		  }
	  }
	  // 前置检查：用户是否存在未关闭 ThawCase（双释阻断）。
	   if s.leaseHook != nil {
		   if err := s.leaseHook.GuardFreeze(req.UserID); err != nil {
			  return nil, err
		   }
	  }
	  rate, src, ok := s.execRate(req.FromAsset, req.ToAsset)
	   if !ok {
		   metrics.QuoteRejected.WithLabelValues("stale").Inc()
		   return nil, ledger.ErrStaleQuote
	  }
	   _ = src
	   if req.QuoteRate != "" {
		   quoted, err := ledger.ParseAmount(req.QuoteRate)
		   if err != nil {
			  return nil, err
		   }
		  if err := s.checkSlippage(quoted, rate); err != nil {
			   metrics.QuoteRejected.WithLabelValues("slippage").Inc()
			  return nil, err
		   }
	  }
	  ratio := decimal.NewFromInt(1)
	  if req.FillRatio != "" {
		   ratio, err = ledger.ParseRatio(req.FillRatio)
		  if err != nil {
			  return nil, err
		   }
	   }
	   filledFrom := fromAmt.Mul(ratio)
	   gross := filledFrom.Mul(rate)
	  fee := gross.Mul(decimal.NewFromInt(int64(s.feeBPS))).Div(decimal.NewFromInt(10000))
	   toAmt := gross.Sub(fee)
	   if req.Chain == "" {
		  req.Chain = "ETH"
	  }
	   if !wallet.AssetOnChain(req.FromAsset, req.Chain) {
		   return nil, fmt.Errorf("%w: %s on %s", ledger.ErrAssetChain, req.FromAsset, req.Chain)
	  }
	   ord := &Order{
		  ID: uuid.NewString(), IdempotencyKey: req.IdempotencyKey, UserID: req.UserID,
		   FromAsset: req.FromAsset, ToAsset: req.ToAsset, FromAmount: fromAmt,
		   QuoteRate:    rate,
		  FilledAmount: filledFrom, ToAmount: toAmt,
		   FeeAsset: req.ToAsset, FeeAmount: fee, Status: StatusQuoted,
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
		   if ratio.IsZero() {
			   ord.Status = StatusFailed
			  ord.FailReason = "zero fill"
			  ord.FilledAmount = decimal.Zero
			   ord.ToAmount = decimal.Zero
			  ord.FeeAmount = decimal.Zero
			  metrics.SwapOrders.WithLabelValues(string(ord.Status)).Inc()
			   return s.insertOrder(ctx, ord)
		   }
		   src, err := s.st.GetAccount(ctx, req.UserID, ledger.AccountWallet, req.FromAsset)
		   if err != nil {
			  return err
		   }
		  if src.Available.LessThan(filledFrom) {
			   ord.Status = StatusFailed
			   ord.FailReason = ledger.ErrInsufficient.Error()
			   metrics.SwapOrders.WithLabelValues(string(ord.Status)).Inc()
			   _ = s.insertOrder(ctx, ord)
			   return ledger.ErrInsufficient
		  }
		   if req.Chain == "" {
			   req.Chain = "ETH"
		  }
		  ord.Chain = req.Chain
		  // freeze first; only settle / failUnfreeze releases it
		   lines := freezeSwap(ord.ID, req.UserID, req.FromAsset, filledFrom)
		   if err := s.st.AppendJournal(ctx, lines); err != nil {
			  return err
		   }
		   for _, e := range lines {
			  if err := ledger.ApplyJournalToAccount(src, e); err != nil {
				   return err
			  }
		  }
		  if err := s.st.UpsertAccount(ctx, src); err != nil {
			  return err
		  }
		  ord.Status = StatusFrozen
		  metrics.SwapOrders.WithLabelValues(string(ord.Status)).Inc()
		   _ = s.emit(ctx, ord, "frozen", map[string]any{"from": req.FromAsset, "filled": filledFrom.String()})
		   if err := s.insertOrder(ctx, ord); err != nil {
			  return err
		   }
		  // 冻额效果令牌：与 freeze journal 同事务写入，绑定世代。
		  if s.leaseHook != nil {
			   leaseID, leaseToken, err := s.leaseHook.OnFreezeCommit(ctx, ord.ID, req.UserID, req.FromAsset, filledFrom.String())
			   if err != nil {
				   return fmt.Errorf("swap: LeaseHook.OnFreezeCommit: %w", err)
			   }
			   ord.FreezeLeaseID = leaseID
			  ord.FreezeLeaseToken = leaseToken
		   }
		  return s.stampOrder(ctx,
			   ord, lease)
	  })
	   if err != nil {
		   if errors.Is(err, ledger.ErrIdempotency) {
			   existing,
				   e2 := s.getByIdem(ctx, req.IdempotencyKey)
			   if e2 != nil {
				   return ord, err
			   }
			  if e := sameSwapIntent(existing, req); e != nil {
				   metrics.IdempotencyConflicts.Inc()
				  return existing, e
			   }
			   return s.finishFrozenLocked(ctx, existing, lease)
		   }
		   if errors.Is(err, ledger.ErrInsufficient) {
			  return ord, err
		  }
		   return ord, err
	   }
	   if ord.Status == StatusFailed {
		  return ord, nil
	  }
	   // 广播放在冻结事务外，RPC 不能拖着库事务。失败且还没 hash 才解冻。
	   if s.Submit != nil {
		   if err := lease.Touch(ctx); err != nil {
			   // 租约丢了不要解冻，可能已经换人持锁。
			   got, _ := s.getByID(ctx, ord.ID)
			   if got != nil {
				  return got, err
			  }
			  return ord, err
		   }
		   // 兑换链上出口走系统账户派生地址。
		   hash, err := s.Submit(wallet.WithSubmitNonce(wallet.WithUserID(ctx, ord.UserID), ord.ID), ord.Chain, swapRouter(ord.Chain), filledFrom)
		  if err != nil {
			   if rb := s.failUnfreezeLocked(ctx, ord, err.Error()); rb != nil {
				   return ord, rb
			  }
			  got, _ := s.getByID(ctx, ord.ID)
			   return got, err
		   }
		   ord.TxHash = hash
		   _ = s.updateOrder(ctx, ord)
		  _ = s.emit(ctx, ord, "chain_submitted", map[string]any{"tx_hash": hash})
	  }
	  if req.Async {
		  got, err = s.getByID(ctx, ord.ID)
		  if err != nil {
			   return ord, err
		  }
		  if s.OnBooks != nil {
			   s.OnBooks(ctx, req.UserID)
		  }
		  return got, nil
	  }
	   if err := s.settleLocked(ctx, ord); err != nil {
		  got, _ := s.getByID(ctx, ord.ID)
		  if got != nil {
			   return got, err
		   }
		  return ord, err
	   }
	  got, err = s.getByID(ctx, ord.ID)
	  if err != nil {
		   return ord, err
	   }
	  if s.OnBooks != nil {
		  s.OnBooks(ctx, req.UserID)
	   }
	   return got, nil
   }

// Get 按单号取兑换单。
  func (s *Service) Get(ctx context.Context, id string) (*Order, error) {
	   return s.getByID(ctx, id)
   }

  func (s *Service) ListOrders(ctx context.Context, userID string) ([]*Order, error) {
	   return s.listOrders(ctx, userID)
   }

// freezeSwap 复用账本冻结 memo，对账把兑换占用和钱包锁同等对待。

   func freezeSwap(id, userID, asset string, amt decimal.Decimal) []ledger.JournalEntry {
	   return []ledger.JournalEntry{
		  {TransferID: id, Seq: 1, UserID: userID, AccountType: ledger.AccountWallet, Asset: asset, Debit: amt, Memo: "freeze_available"},
		  {TransferID: id, Seq: 2, UserID: userID, AccountType: ledger.AccountWallet, Asset: asset, Credit: amt, Memo: "freeze_frozen"},
	   }
  }

// sameSwapIntent 幂等键命中时核对币对/数量/链，对不上记冲突。
   func sameSwapIntent(o *Order, req ExecReq) error {
	  if req.FromAsset != "" && o.FromAsset != req.FromAsset {
		   return ledger.ErrIdempotencyConflict
	  }
	   if req.ToAsset != "" && o.ToAsset != req.ToAsset {
		  return ledger.ErrIdempotencyConflict
	   }
	  if strings.TrimSpace(req.FromAmount) != "" {
		  amt, err := ledger.ParseAmount(req.FromAmount)
		   if err != nil {
			  return err
		   }
		   if !o.FromAmount.Equal(amt) {
			  return ledger.ErrIdempotencyConflict
		   }
	   }
	   reqChain := strings.ToUpper(strings.TrimSpace(req.Chain))
	   gotChain := strings.ToUpper(strings.TrimSpace(o.Chain))
	   if reqChain != "" && gotChain != "" && reqChain != gotChain {
		  return ledger.ErrIdempotencyConflict
	   }
	  return nil
  }

// execRate 读现货缓存报价；没有新鲜 tick 返回 false，禁止用过期价成交。
   func (s *Service) execRate(from, to string) (decimal.Decimal, string, bool) {
	   r, ok := s.pricer.Rate(from, to)
	  src := "static"
	  if _, isLive := s.pricer.(CachePricer); isLive {
		  src = "spot_ticker"
	   }
	   return r, src, ok
   }

// checkSlippage 成交价相对询价超出滑点则拒单，避免钱包页和交易页价格打架。

   func (s *Service) checkSlippage(quoted, live decimal.Decimal) error {
	   if quoted.IsZero() {
		  return nil
	  }
	   diff := live.Sub(quoted).Abs()
	   bps := diff.Div(quoted).Mul(decimal.NewFromInt(10000))
	   if bps.GreaterThan(decimal.NewFromInt(int64(s.slippageBPS))) {
		   return ledger.ErrSlippage
	   }
	   return nil
   }

// finishFrozen 已冻未结的单接着结算：部分成交入账，失败解冻，状态必须和余额对上。
  func (s *Service) finishFrozen(ctx context.Context,
	   o *Order) (*Order, error) {
	   lease, err := s.lock.Lock(ctx, ledger.UserLockKey(o.UserID), s.lockTTL)
	  if err != nil {
		   return nil, err
	   }
	  defer lease.Unlock()
	  if err := s.stampOrder(ctx, o, lease); err != nil {
		   return nil, err
	   }
	  return s.finishFrozenLocked(ctx, o, lease)
   }

// finishFrozenLocked 已持用户锁：有 hash 的冻结必须结算，不得因行情过期自动解冻。
  func (s *Service) finishFrozenLocked(ctx context.Context, o *Order, lease ledger.Guard) (*Order, error) {
	  cur, err := s.getByID(ctx, o.ID)
	   if err != nil {
		   return nil, err
	  }
	   switch cur.Status {
	  case StatusFilled, StatusPartial, StatusFailed:
		   return cur, nil
	   case StatusFrozen:
	  default:
		  return cur, nil
	  }
	   live, ok := s.pricer.Rate(cur.FromAsset, cur.ToAsset)
	  if !ok {
		   metrics.QuoteRejected.WithLabelValues("stale").Inc()
		   if cur.TxHash != "" {
			  return cur, ledger.ErrStaleQuote
		  }
		  if err := s.failUnfreezeLocked(ctx, cur, ledger.ErrStaleQuote.Error()); err != nil {
			  return cur, err
		   }
		  got, _ := s.getByID(ctx, cur.ID)
		   return got, ledger.ErrStaleQuote
	  }
	   if err := s.checkSlippage(cur.QuoteRate, live); err != nil {
		  metrics.QuoteRejected.WithLabelValues("slippage").Inc()
		   if cur.TxHash != "" {
			   return cur, err
		   }
		   if rb := s.failUnfreezeLocked(ctx, cur, err.Error()); rb != nil {
			  return cur, rb
		  }
		  got, _ := s.getByID(ctx, cur.ID)
		  return got, err
	   }
	   if cur.TxHash == "" && s.Submit != nil {
		   if lease != nil {
			  if err := lease.Touch(ctx); err != nil {
				   got, _ := s.getByID(ctx, cur.ID)
				  if got != nil {
					  return got, err
				   }
				  return cur, err
			   }
		   }
		   hash, err := s.Submit(wallet.WithSubmitNonce(wallet.WithUserID(ctx, cur.UserID), cur.ID), cur.Chain, swapRouter(cur.Chain), cur.FilledAmount)
		   if err != nil {
			   if rb := s.failUnfreezeLocked(ctx, cur, err.Error()); rb != nil {
				   return cur, rb
			  }
			  got, _ := s.getByID(ctx, cur.ID)
			   return got, err
		   }
		   cur.TxHash = hash
		  _ = s.updateOrder(ctx, cur)
	   }
	  if err := s.settleLocked(ctx, cur); err != nil {
		  got, _ := s.getByID(ctx, cur.ID)
		  if got != nil {
			   return got, err
		  }
		  return cur, err
	   }
	   got, err := s.getByID(ctx, cur.ID)
	   if err != nil {
		  return cur, err
	   }
	  if s.OnBooks != nil {
		   s.OnBooks(ctx, got.UserID)
	   }
	   return got, nil
   }

// settleLocked 成交入账：目标资产和手续费记钱包，解冻已成交部分。
   func (s *Service) settleLocked(ctx context.Context, ord *Order) error {
	  return s.st.InTx(ctx, func(ctx context.Context) error {
		   cur, err := s.getByID(ctx, ord.ID)
		   if err != nil {
			  return err
		   }
		   if cur.Status == StatusFilled || cur.Status == StatusPartial {
			  *ord = *cur
			  return nil
		   }
		   if cur.Status != StatusFrozen {
			   return fmt.Errorf("swap: cannot settle from %s", cur.Status)
		  }
		  // Lease 互斥：争抢 settle 终态腿；双释时升 ThawCase 并拒绝。
		   if s.leaseHook != nil && ord.FreezeLeaseID != "" {
			   src, _ := s.st.GetAccount(ctx, cur.UserID, ledger.AccountWallet, cur.FromAsset)
			   frozenSnap, availSnap := "0", "0"
			  if src != nil {
				   frozenSnap = src.Frozen.String()
				  availSnap = src.Available.String()
			   }
			   _, tc, hookErr := s.leaseHook.OnSettleBegin(ctx, ord.ID, ord.FreezeLeaseID, ord.FreezeLeaseToken, frozenSnap, availSnap)
			  if hookErr != nil {
				   if tc != nil {
					  cur.ThawProofID = tc.ID
					   _ = s.updateOrder(ctx, cur)
				   }
				   return hookErr
			   }
		  }
		   prev,
			   err := s.st.ListJournal(ctx, cur.ID)
		   if err != nil {
			  return err
		   }
		   for _, e := range prev {
			   if e.Memo == "consume_frozen" {
				   if cur.FilledAmount.LessThan(cur.FromAmount) {
					  cur.Status = StatusPartial
				   } else {
					  cur.Status = StatusFilled
				  }
				   *ord = *cur
				  return s.updateOrder(ctx, cur)
			   }
		   }
		  src, err := s.st.GetAccount(ctx, cur.UserID, ledger.AccountWallet, cur.FromAsset)
		  if err != nil {
			  return err
		   }
		  dst, err := s.st.GetAccount(ctx, cur.UserID, ledger.AccountWallet, cur.ToAsset)
		   if err != nil {
			  return err
		   }
		  deskFrom, err := s.st.GetAccount(ctx, "system", ledger.AccountBridge, cur.FromAsset)
		  if err != nil {
			   return err
		  }
		  deskTo, err := s.st.GetAccount(ctx, "system", ledger.AccountBridge, cur.ToAsset)
		   if err != nil {
			   return err
		  }
		   feeAcc, err := s.st.GetAccount(ctx, "system", ledger.AccountFee, cur.ToAsset)
		  if err != nil {
			  return err
		  }
		  filledFrom := cur.FilledAmount
		  gross := filledFrom.Mul(cur.QuoteRate)
		   fee := cur.FeeAmount
		  seq := ledger.NextJournalSeq(prev)
		  lines := []ledger.JournalEntry{
			  {TransferID: cur.ID, Seq: seq, UserID: cur.UserID, AccountType: ledger.AccountWallet, Asset: cur.FromAsset, Debit: filledFrom, Memo: "consume_frozen"},
			  {TransferID: cur.ID, Seq: seq + 1, UserID: "system", AccountType: ledger.AccountBridge, Asset: cur.FromAsset, Credit: filledFrom, Memo: "bridge_in"},
			   {TransferID: cur.ID, Seq: seq + 2, UserID: "system", AccountType: ledger.AccountBridge, Asset: cur.ToAsset, Debit: gross, Memo: "bridge_out"},
			  {TransferID: cur.ID, Seq: seq + 3, UserID: cur.UserID, AccountType: ledger.AccountWallet, Asset: cur.ToAsset, Credit: gross, Memo: "swap_credit"},
		  }
		  if !fee.IsZero() {
			   lines = append(lines,
				  ledger.JournalEntry{TransferID: cur.ID, Seq: seq + 4, UserID: cur.UserID, AccountType: ledger.AccountWallet, Asset: cur.ToAsset, Debit: fee, Memo: "fee_debit"},
				  ledger.JournalEntry{TransferID: cur.ID, Seq: seq + 5, UserID: "system", AccountType: ledger.AccountFee, Asset: cur.ToAsset, Credit: fee, Memo: "fee_credit"},
			   )
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
		  if !fee.IsZero() {
			   if err := ledger.ApplyJournalToAccount(dst, lines[4]); err != nil {
				  return err
			   }
			  if err := ledger.ApplyJournalToAccount(feeAcc, lines[5]); err != nil {
				  return err
			   }
		  }
		   for _, a := range []*ledger.Account{src, dst, deskFrom, deskTo, feeAcc} {
			   if err := s.st.UpsertAccount(ctx, a); err != nil {
				   return err
			  }
		   }
		   if filledFrom.LessThan(cur.FromAmount) {
			   cur.Status = StatusPartial
		  } else {
			   cur.Status = StatusFilled
		   }
		  if ord.TxHash != "" {
			  cur.TxHash = ord.TxHash
		   }
		  metrics.SwapOrders.WithLabelValues(string(cur.Status)).Inc()
		  _ = s.emit(ctx, cur, string(cur.Status), map[string]any{"from": cur.FromAsset, "to": cur.ToAsset, "filled": filledFrom.String(), "fee": fee.String(), "tx_hash": cur.TxHash})
		   if err := s.updateOrder(ctx, cur); err != nil {
			   return err
		   }
		  *ord = *cur
		   return nil
	   })
   }

// RecoverFrozen 收尾卡住的冻单。有 hash 的走确认，没 hash 且超时才解冻。
   func (s *Service) RecoverFrozen(ctx context.Context, olderThan time.Duration) {
	   list, err := s.listFrozen(ctx)
	  if err != nil {
		   return
	  }
	  now := time.Now().UTC()
	   for _, o := range list {
		  if o.Status != StatusFrozen {
			   continue
		  }
		   if o.TxHash != "" {
			  _, _ = s.finishFrozen(ctx, o)
			   continue
		   }
		  if olderThan <= 0 {
			   continue
		   }
		  age := now.Sub(o.CreatedAt)
		   if o.CreatedAt.IsZero() || age < olderThan {
			  continue
		   }
		  _ = s.failUnfreeze(ctx, o, "stuck freeze without chain submit")
	  }
   }

// failUnfreeze 失败解冻：没有 live hash 才放回可用，钱包余额和订单状态一起失败。

  func (s *Service) failUnfreeze(ctx context.Context, ord *Order, reason string) error {
	   lease, err := s.lock.Lock(ctx, ledger.UserLockKey(ord.UserID), s.lockTTL)
	   if err != nil {
		   return err
	  }
	  defer lease.Unlock()
	   return s.failUnfreezeLocked(ctx, ord, reason)
   }

// failUnfreezeLocked is the swap counterpart of transfer rollback: frozen
// from-asset returns to available and the order is failed. Destination is
// untouched. A second call is a no-op so retries cannot unfreeze twice.
// failUnfreezeLocked 已持锁：解冻失败单。已有链上 hash 的冻结不得走这里。

   func (s *Service) failUnfreezeLocked(ctx context.Context, ord *Order, reason string) error {
	   return s.st.InTx(ctx, func(ctx context.Context) error {
		   cur,
			   err := s.getByID(ctx, ord.ID)
		   if err != nil {
			   return err
		  }
		  if cur.Status == StatusFilled || cur.Status == StatusPartial {
			   return fmt.Errorf("swap: cannot unfreeze settled %s", cur.Status)
		  }
		   if cur.TxHash != "" {
			   return fmt.Errorf("%w: swap hash already broadcast", ledger.ErrSubmittedLive)
		   }
		  // Lease 互斥：争抢 failUnfreeze 终态腿；settle 已胜出则幂等拒绝；双释则升 ThawCase。
		  if s.leaseHook != nil && ord.FreezeLeaseID != "" {
			   acc,
				  _ := s.st.GetAccount(ctx, cur.UserID, ledger.AccountWallet, cur.FromAsset)
			   frozenSnap, availSnap := "0", "0"
			   if acc != nil {
				   frozenSnap = acc.Frozen.String()
				   availSnap = acc.Available.String()
			   }
			  _,
				  tc, hookErr := s.leaseHook.OnUnfreezeBegin(ctx, ord.ID, ord.FreezeLeaseID, ord.FreezeLeaseToken, frozenSnap, availSnap)
			  if hookErr != nil {
				   if tc != nil {
					  cur.ThawProofID = tc.ID
					  _ = s.updateOrder(ctx, cur)
				   }
				   return hookErr
			   }
		   }
		  prev, err := s.st.ListJournal(ctx, cur.ID)
		  if err != nil {
			  return err
		   }
		  hasRestore := false
		  hasFreeze := false
		  for _, e := range prev {
			   if e.Memo == "restore_available" {
				   hasRestore = true
			   }
			   if e.Memo == "freeze_frozen" {
				  hasFreeze = true
			   }
		  }
		   cur.Status = StatusFailed
		  cur.FailReason = reason
		   if hasRestore || !hasFreeze {
			  *ord = *cur
			   return s.updateOrder(ctx, cur)
		  }
		  seq := ledger.NextJournalSeq(prev)
		  acc, err := s.st.GetAccount(ctx, cur.UserID, ledger.AccountWallet, cur.FromAsset)
		   if err != nil {
			   return err
		   }
		   lines := []ledger.JournalEntry{
			  {TransferID: cur.ID, Seq: seq, UserID: cur.UserID, AccountType: ledger.AccountWallet, Asset: cur.FromAsset, Debit: cur.FilledAmount, Memo: "unfreeze_frozen"},
			   {TransferID: cur.ID, Seq: seq + 1, UserID: cur.UserID, AccountType: ledger.AccountWallet, Asset: cur.FromAsset, Credit: cur.FilledAmount, Memo: "restore_available"},
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
		  metrics.SwapOrders.WithLabelValues(string(cur.Status)).Inc()
		   _ = s.emit(ctx, cur, "rolled_back", map[string]any{"reason": reason})
		  if err := s.updateOrder(ctx, cur); err != nil {
			  return err
		   }
		   *ord = *cur
		   return nil
	   })
   }

// FrozenInflight 在途兑换占用，给对账用。

  func (s *Service) FrozenInflight(ctx context.Context, userID string) (map[string]decimal.Decimal, []ledger.Mismatch, error) {
	  orders, err := s.listOrders(ctx, userID)
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
		  if len(j) > 0 {
			  if err := ledger.ValidateJournal(j); err != nil {
				  mm = append(mm, ledger.Mismatch{UserID: userID, TransferID: o.ID, State: string(o.Status), Asset: o.FromAsset, Note: "unbalanced swap journal: " + err.Error()})
			  }
		   }
		   hasFreeze,
			  hasUnfreeze, hasCredit := false, false, false
		   for _, e := range j {
			  switch e.Memo {
			   case "freeze_available", "freeze_frozen":
				   hasFreeze = true
			   case "unfreeze_frozen", "restore_available":
				   hasUnfreeze = true
			  case "consume_frozen", "swap_credit":
				  hasCredit = true
			   }
		   }
		   switch o.Status {
		  case StatusFrozen:
			  k := string(ledger.AccountWallet) + "|" + o.FromAsset
			  extra[k] = extra[k].Add(o.FilledAmount)
			  if !hasFreeze {
				   mm = append(mm, ledger.Mismatch{UserID: userID, TransferID: o.ID, State: string(o.Status), Asset: o.FromAsset, Note: "frozen swap missing freeze journal"})
			  }
		  case StatusFilled, StatusPartial:
			  if !hasCredit {
				   mm = append(mm, ledger.Mismatch{UserID: userID, TransferID: o.ID, State: string(o.Status), Asset: o.FromAsset, Note: "filled swap missing settlement journal"})
			   }
		  case StatusFailed:
			   if hasFreeze && !hasUnfreeze {
				   mm = append(mm, ledger.Mismatch{UserID: userID, TransferID: o.ID, State: string(o.Status), Asset: o.FromAsset, Note: "failed swap still frozen; rollback required"})
			  }
		  }
	  }
	   return extra, mm, nil
  }

// emit 兑换审计+出箱，和账本同一套 wallet.movements。
   func (s *Service) emit(ctx context.Context, ord *Order, action string, extra map[string]any) error {
	   if extra == nil {
		  extra = map[string]any{}
	   }
	  extra["movement_id"] = ord.ID
	   extra["kind"] = events.KindSwap
	  extra["status"] = string(ord.Status)
	  if tid := trace.ID(ctx); tid != "" {
		   extra["trace_id"] = tid
	  }
	   if err := s.st.AppendAudit(ctx, ledger.AuditEvent{
		  EntityType: "swap", EntityID: ord.ID, Action: action, Actor: "system", Detail: extra,
	  }); err != nil {
		  return err
	   }
	   metrics.OutboxEvents.WithLabelValues("enqueued").Inc()
	   return s.st.AppendOutbox(ctx, ledger.OutboxEvent{
		   ID: uuid.NewString(),
		   AggregateType: events.AggregateSwap,
		   AggregateID:   ord.ID,
		   Kind:          events.KindSwap,
		  Action:        action,
		  UserID: ord.UserID,
		   TraceID: trace.ID(ctx),
		  State:   string(ord.Status),
		   Payload: extra,
	  })
   }

// swapRouter 兑换链上出口地址。
  func swapRouter(chain string) string {
	  return wallet.DeriveAddress("system.swap", chain)
   }
