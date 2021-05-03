package feeoracle

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/feeeng"
	"account-assets.kucoin.net/funding/internal/oracle"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

// Service 多链手续费报价服务。
type Service struct {
	oracle *Oracle
	store *QuoteVersionStore
	feeBook   *feeeng.Book
	priceFeed *oracle.Feed
	clock     func() time.Time
	// QuoteTTL 报价代次有效期；零值使用 DefaultQuoteTTL。
	QuoteTTL time.Duration
	// DeviationBPS 超过此偏差触发熔断（覆盖 oracle.DeviationBPS）；0 表示不覆盖。
	DeviationBPS int
	// FallbackAudited 若为 true，允许 oracle 不可用时回落到静态档位；
	// 必须在有审计记录的情况下才能开启。
	FallbackAudited bool
}

// New 创建 Service，传入 Oracle、代次存储、feeeng Book（fallback）和 oracle price Feed。
   func New(o *Oracle, store *QuoteVersionStore, book *feeeng.Book, priceFeed *oracle.Feed) *Service {
	  return &Service{
		  oracle: o,
		   store:  store,
		   feeBook:   book,
		   priceFeed: priceFeed,
		  clock:     func() time.Time { return time.Now().UTC() },
	  }
   }

// NewMem 内存模式（用）：以默认 oracle、内存存储装配。

  func NewMem() *Service {
	  o := NewOracle(500)
	   store := NewVersionStore(nil)
	   book := feeeng.DefaultBook()
	  feed := oracle.Mock(time.Now().UTC())
	   return New(o, store, book, feed)
  }

// WithClock 替换时钟（用）。
  func (s *Service) WithClock(c func() time.Time) *Service {
	  if s != nil && c != nil {
		  s.clock = c
		  if s.oracle != nil {
			   s.oracle.WithClock(c)
		  }
		  if s.store != nil {
			  s.store.WithClock(c)
		   }
	   }
	  return s
   }

  // BindSQL 挂载 SQL 数据库；必须在首次报价前调用。
   func (s *Service) BindSQL(db *persist.SQLDB) *Service {
	   if s != nil && db != nil {
		  s.store = NewVersionStore(db)
		   s.store.WithClock(s.clock)
	  }
	   return s
  }

// BindFeeBook 替换 feeeng fallback Book。

  func (s *Service) BindFeeBook(book *feeeng.Book) *Service {
	   if s != nil && book != nil {
		  s.feeBook = book
	  }
	   return s
  }

// BindPriceFeed 替换 oracle price Feed。
   func (s *Service) BindPriceFeed(feed *oracle.Feed) *Service {
	  if s != nil && feed != nil {
		   s.priceFeed = feed
	  }
	   return s
   }

// Oracle 返回内部 Oracle（供 ops/HTTP 层直接访问健康信息）。

  func (s *Service) Oracle() *Oracle {
	   if s == nil {
		   return NewOracle(0)
	  }
	   return s.oracle
  }

// Store 返回内部 QuoteVersionStore（供 ops/HTTP 层访问）。
  func (s *Service) Store() *QuoteVersionStore {
	  if s == nil {
		   return NewVersionStore(nil)
	  }
	  return s.store
  }

// QuoteWithdrawFeeReq 出金预检报价请求。

type QuoteWithdrawFeeReq struct {
	// Chain 目标链名。
	Chain string
	// Family 链族（可选，空时从已注册源推断）。
	Family string
	// Asset 用户计价资产（如 "USDT"/"BTC"/"ETH"）。
	Asset string
	// Gross 用户出金总额（用户资产计价）。
	Gross string
	// QuoteTTL 本次报价有效期（零值使用 Service.QuoteTTL 或 DefaultQuoteTTL）。
	QuoteTTL time.Duration
	// ScheduleID 指定使用某个 feeeng Schedule（可选）。
	ScheduleID string
}

// QuoteWithdrawFee 出金预检时报价并生成 QuoteVersion。
// 调用方在广播前必须先调用 BindPrecheck 将代次与出金单绑定。
// 若 oracle 不可用且未开启 FallbackAudited，直接返回 ErrUnavailable。

  func (s *Service) QuoteWithdrawFee(ctx context.Context, req QuoteWithdrawFeeReq) (QuoteVersion, error) {
	  if s == nil {
		   return QuoteVersion{}, ErrUnavailable
	  }
	  chain, err := validate.Chain(req.Chain)
	  if err != nil {
		   return QuoteVersion{}, err
	   }
	   asset, err := validate.Asset(req.Asset)
	   if err != nil {
		  return QuoteVersion{}, err
	   }
	   gross, err := validate.Amount(req.Gross)
	   if err != nil {
		  return QuoteVersion{}, err
	   }
	   grossAmt, err := money.Parse(gross)
	  if err != nil {
		   return QuoteVersion{}, err
	   }
	   now := s.clock()
	  ttl := s.resolveQuoteTTL(req.QuoteTTL)
	   // 1. 从 oracle 拿聚合 tick。
	   tick, family, err := s.getAggregatedTick(chain, req.Family)
	   if err != nil {
		  // oracle 不可用时若允许 fallback 则改用静态档位。
		   if s.FallbackAudited && s.feeBook != nil {
			  return s.quoteFromSchedule(ctx, chain, asset, gross, grossAmt, req.ScheduleID, ttl, now, err)
		   }
		   return QuoteVersion{}, fmt.Errorf("%w: chain=%s: %v", ErrUnavailable, chain, err)
	  }
	  // 2. 把 tick 原生费用换算成用户资产计价。
	   quoteFeeAmt, err := s.convertNativeToAsset(tick.Normalized, asset)
	   if err != nil {
		   if s.FallbackAudited && s.feeBook != nil {
			   return s.quoteFromSchedule(ctx, chain, asset, gross, grossAmt, req.ScheduleID, ttl, now, err)
		  }
		  return QuoteVersion{}, fmt.Errorf("%w: %v", ErrNativeConvert, err)
	   }
	   // 3. 与 feeeng Schedule 的 min/max 夹值（若指定了 schedule）。
	  quoteFeeAmt, scheduleID, bps, err := s.applyScheduleBounds(chain, asset, quoteFeeAmt, req.ScheduleID)
	  if err != nil {
		  return QuoteVersion{}, err
	   }
	  // 4. 计算净额并守恒校验。
	  if quoteFeeAmt.Cmp(grossAmt) >= 0 {
		  quoteFeeAmt = grossAmt
	   }
	  netAmt, err := grossAmt.Sub(quoteFeeAmt)
	  if err != nil {
		   return QuoteVersion{}, fmt.Errorf("%w: gross=%s fee=%s", ErrConserve, grossAmt, quoteFeeAmt)
	   }
	   if err := money.Conserve(mustAddAmt(quoteFeeAmt, netAmt), grossAmt); err != nil {
		   return QuoteVersion{}, fmt.Errorf("%w: %v", ErrConserve, err)
	   }
	  v := QuoteVersion{
		   ID:    uuid.NewString(),
		   Chain: chain,
		  Family: family,
		  Asset: asset,
		   Gross:      grossAmt.String(),
		   QuoteFee:   quoteFeeAmt.String(),
		  QuoteNet:   netAmt.String(),
		  Tick:       tick,
		  ScheduleID: scheduleID,
		   BPS:       bps,
		   State:     StateActive,
		  ExpiresAt: now.Add(ttl),
		  CreatedAt: now,
	  }
	  return s.store.Save(ctx, v)
  }

// BindPrecheck 将已创建的 QuoteVersion 绑定到出金单（幂等）。
// 代次必须处于 StateActive 且未过期；过期则拒绝，不允许静默使用旧代次。
  func (s *Service) BindPrecheck(ctx context.Context, versionID, withdrawID string) (QuoteVersion, error) {
	  if s == nil {
		   return QuoteVersion{}, ErrUnavailable
	  }
	   return s.store.Bind(ctx, versionID, withdrawID)
  }

// GetVersion 按代次 ID 读取快照（ops/replay 用）。
  func (s *Service) GetVersion(ctx context.Context, versionID string) (QuoteVersion, error) {
	   if s == nil {
		   return QuoteVersion{}, ErrUnavailable
	   }
	  return s.store.Find(ctx, versionID)
   }

// GetVersionByWithdraw 按出金单读取已绑定的代次（出金管道中确认时调用）。

  func (s *Service) GetVersionByWithdraw(ctx context.Context, withdrawID string) (QuoteVersion, error) {
	  if s == nil {
		   return QuoteVersion{}, ErrUnavailable
	  }
	  return s.store.FindByWithdraw(ctx, withdrawID)
  }

// AlignActual 广播确认后只读一致实际链上手续费。
// 将代次推进为 StateSettled，不触碰 conserve FSM，不自动解冻。
// 若代次已 StateSettled，幂等返回。
   func (s *Service) AlignActual(ctx context.Context, versionID, actualFee string) (QuoteVersion, error) {
	  if s == nil {
		  return QuoteVersion{}, ErrUnavailable
	  }
	   return s.store.Settle(ctx, versionID, actualFee)
  }

// ExpireOld 定期过期超时代次（由后台 worker 调用）。
   func (s *Service) ExpireOld(ctx context.Context) (int, error) {
	  if s == nil {
		   return 0, ErrUnavailable
	  }
	   return s.store.ExpireOld(ctx)
   }

// Health 返回 oracle 整体健康快照。
   func (s *Service) Health() OracleHealth {
	  if s == nil || s.oracle == nil {
		   return OracleHealth{Healthy: false, Reason: "oracle not configured"}
	   }
	   return s.oracle.HealthReport()
  }

// RegisterSource 向 oracle 注册一个 fee 源（委托给 oracle）。
  func (s *Service) RegisterSource(cfg SourceConfig) error {
	  if s == nil {
		   return ErrUnavailable
	  }
	   return s.oracle.RegisterSource(cfg)
   }

// PushTick 向 oracle 推送单源 tick（由采集 worker 调用）。
  func (s *Service) PushTick(sourceID string, tick ChainFeeTick) error {
	  if s == nil {
		  return ErrUnavailable
	  }
	   return s.oracle.PushTick(sourceID, tick)
  }

// resolveQuoteTTL 解析报价有效期，先级：req > svc > default。
   func (s *Service) resolveQuoteTTL(req time.Duration) time.Duration {
	   if req > 0 {
		   return req
	   }
	  if s.QuoteTTL > 0 {
		   return s.QuoteTTL
	   }
	   return DefaultQuoteTTL
  }

// getAggregatedTick 从 oracle 获取聚合 tick，并推断链族。

   func (s *Service) getAggregatedTick(chain, familyHint string) (ChainFeeTick, Family, error) {
	  tick, err := s.oracle.GetAggregated(chain)
	   if err != nil {
		  return ChainFeeTick{}, "", err
	   }
	  family := tick.Family
	   if family == "" && familyHint != "" {
		  if f, ferr := ParseFamily(familyHint); ferr == nil {
			  family = f
		   }
	   }
	  if family == "" {
		  family = FamilyGeneric
	  }
	   tick.Family = family
	   return tick, family, nil
   }

// convertNativeToAsset 将规范化后的原生费用转换为用户资产计价。
// 若原生资产 == 用户资产，直接返回；否则通过 priceFeed 换算。

   func (s *Service) convertNativeToAsset(norm NormalizedFee, asset string) (money.Amount, error) {
	  nativeAsset := strings.ToUpper(strings.TrimSpace(norm.NativeAsset))
	  userAsset := strings.ToUpper(strings.TrimSpace(asset))
	   nativeStr := norm.NativeAmount.String()
	  // 原生资产与用户资产相同，无需换算。
	  if nativeAsset == userAsset || nativeAsset == "" {
		  return money.ParseBook(nativeStr)
	  }
	  // 通过 oracle price Feed 换算：native → user_asset。
	   if s.priceFeed == nil {
		   // 无 priceFeed，无法换算。
		  return money.Zero(), fmt.Errorf("no price feed to convert %s→%s", nativeAsset, userAsset)
	  }
	   converted, _, err := s.priceFeed.Convert(nativeAsset, nativeStr, userAsset)
	   if err != nil {
		  return money.Zero(), fmt.Errorf("price feed convert %s→%s: %w", nativeAsset, userAsset, err)
	   }
	   return converted, nil
  }

// applyScheduleBounds 若指定了 ScheduleID，用 feeeng Book 的 min/max 夹值。
// 返回（夹值后手续费、scheduleID、bps、error）。
   func (s *Service) applyScheduleBounds(chain, asset string, fee money.Amount, scheduleID string) (money.Amount, string, int, error) {
	   if s.feeBook == nil || scheduleID == "" {
		  return fee, "", 0, nil
	  }
	   sch, err := s.feeBook.Get(feeeng.KindWithdraw, asset)
	   if err != nil {
		  // 未找到档位：不夹值，直接使用 oracle 报价。
		  return fee, "", 0, nil
	   }
	  result := fee
	  if sch.MinFee != "" {
		   if min, err := money.ParseBook(sch.MinFee); err == nil && fee.Cmp(min) < 0 {
			   result = min
		   }
	   }
	  if sch.MaxFee != "" {
		  if max, err := money.ParseBook(sch.MaxFee); err == nil && max.IsPositive() && result.Cmp(max) > 0 {
			   result = max
		  }
	   }
	   _ = chain
	  return result, sch.ID, sch.BPS, nil
   }

// quoteFromSchedule oracle 不可用时回落到 feeeng 静态档位报价。
// oracleErr 必须记录，供审计。

   func (s *Service) quoteFromSchedule(
	  ctx context.Context, chain, asset, gross string,
	  grossAmt money.Amount, scheduleID string, ttl time.Duration, now time.Time,
	  oracleErr error,
   ) (QuoteVersion, error) {
	  q, err := s.feeBook.Quote(feeeng.KindWithdraw, asset, gross)
	   if err != nil {
		  return QuoteVersion{}, fmt.Errorf("oracle fallback schedule quote: %w", err)
	  }
	   quoteFeeAmt, err := money.ParseBook(q.Fee)
	   if err != nil {
		   return QuoteVersion{}, err
	  }
	   netAmt,
		  err := money.ParseBook(q.Net)
	  if err != nil {
		  return QuoteVersion{}, err
	  }
	   if err := money.Conserve(mustAddAmt(quoteFeeAmt, netAmt), grossAmt); err != nil {
		  return QuoteVersion{}, fmt.Errorf("%w: fallback %v", ErrConserve, err)
	  }
	   // 回落 tick 记录原因。
	   fallbackTick := ChainFeeTick{
		  SourceID: "fallback_schedule",
		   Chain:    chain,
		  Family:   FamilyGeneric,
		   Normalized: NormalizedFee{
			   Family: FamilyGeneric,
			  Chain:        chain,
			  NativeAsset:  asset,
			  NativeAmount: decimal.RequireFromString(q.Fee),
			  RawDetail:    fmt.Sprintf("fallback due to oracle error: %v", oracleErr),
		   },
		  TickedAt:   now,
		  RecordedAt: now,
	   }
	   v := QuoteVersion{
		  ID:     uuid.NewString(),
		   Chain:  chain,
		   Family: FamilyGeneric,
		  Asset:  asset,
		  Gross:    grossAmt.String(),
		   QuoteFee: quoteFeeAmt.String(),
		  QuoteNet: netAmt.String(),
		  Tick: fallbackTick,
		   ScheduleID: q.Schedule,
		   BPS:       q.BPS,
		  State:     StateActive,
		  ExpiresAt: now.Add(ttl),
		   CreatedAt: now,
	   }
	   _ = ctx
	  return s.store.Save(ctx, v)
  }

// mustAddAmt 内部辅助：panic on error（仅用于 conserve 检查，入参已验证）。
  func mustAddAmt(a, b money.Amount) money.Amount {
	  out, err := a.Add(b)
	   if err != nil {
		   panic(fmt.Sprintf("feeoracle: mustAddAmt overflow: %v", err))
	  }
	   return out
  }
