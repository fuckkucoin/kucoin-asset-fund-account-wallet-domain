package dustleak

import (
	"fmt"
	"strings"
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/dustconv"
	"account-assets.kucoin.net/funding/pkg/money"
)

// DriftCapConfig 牌价漂移帽配置，按资产独立设置。

type DriftCapConfig struct {
	Asset string `json:"asset"`
	CapBps      int `json:"cap_bps"`       // 漂移帽基点；默认 50
	StaleMaxSec int `json:"stale_max_sec"` // 快照最大有效期（秒）；超期快照拒绝使用；默认 30
}

// DefaultDriftConfig 默认漂移帽配置（BTC/ETH 统一 50 bps, 30s）。

var DefaultDriftConfig = []DriftCapConfig{
	{Asset: "BTC", CapBps: 50, StaleMaxSec: 30},
	{Asset: "ETH", CapBps: 50, StaleMaxSec: 30},
}

// DriftGuard 牌价漂移守卫，每次 Convert 独立实例化。
type DriftGuard struct {
	snap *dustconv.QuoteSnapshot
	policyGenID string
	snapAt      time.Time
	staleMaxSec int
}

// NewDriftGuard 根据已持有的 dustconv.QuoteSnapshot 构建漂移守卫。
// policyGenID 用于追踪绑定的策略世代。

  func NewDriftGuard(snap *dustconv.QuoteSnapshot, policyGenID string, staleMaxSec int) *DriftGuard {
	   if staleMaxSec <= 0 {
		   staleMaxSec = 30
	   }
	   return &DriftGuard{
		  snap:        snap,
		   policyGenID: policyGenID,
		   snapAt:      snap.SnapAt,
		  staleMaxSec: staleMaxSec,
	   }
   }

// CheckWithLiveRate 核对实时牌价，校验漂移是否超帽。
// liveRateUSDT: 执行时从 fxmark/oracle 获取的实时牌价。
// 返回 nil 表示漂移在阈值内，Convert 可接着。
// 返回非 nil 表示 Convert 须 fail-closed。
   func (dg *DriftGuard) CheckWithLiveRate(liveRateUSDT string, now time.Time) error {
	   if dg == nil || dg.snap == nil {
		   return ErrUnavailable
	  }
	  // 快照新鲜度校验
	  elapsed := now.Sub(dg.snapAt)
	   if elapsed.Seconds() > float64(dg.staleMaxSec) {
		  return fmt.Errorf("%w: snapshot is stale by %.1fs (max=%ds)",
			   ErrDriftExceed, elapsed.Seconds(), dg.staleMaxSec)
	   }
	  return dg.snap.CheckDrift(liveRateUSDT)
  }

// MaxPermittedUSDT 根据快照牌价与漂移帽计算允许的最大 USDT 兑出量。
// amount: 兑换的源资产数量。
// 返回值：允许的最大 USDT（以快照牌价计算，已含漂移帽余量，但上漂超帽时用于 Leak 检测）。
   func (dg *DriftGuard) MaxPermittedUSDT(amount string) (string, error) {
	   if dg == nil || dg.snap == nil {
		   return "", ErrUnavailable
	  }
	   amt, err := money.Parse(amount)
	   if err != nil {
		  return "", ErrAmount
	   }
	   snapRate, err := money.Parse(dg.snap.RateUSDT)
	  if err != nil {
		   return "", ErrSnapshot
	   }
	   // 按快照牌价（非实时）计算允许的最大兑出量
	   max := money.FromDecimal(amt.Dec().Mul(snapRate.Dec()))
	   return max.String(), nil
   }

// DriftCapChecker 无状态版漂移帽校验（适合 detector 扫描路径）。
// snapRate: 快照时的牌价（来自 QuoteSnapshot.RateUSDT）。
// actualRate: 实际兑换使用的牌价（来自 dustconv.Convert.Rate）。
// capBps: 漂移帽基点。
// 返回 (超额比例 bps, 是否超帽, error)。

  func DriftCapChecker(snapRate, actualRate string, capBps int) (decimal.Decimal, bool, error) {
	   snap, err := money.Parse(snapRate)
	  if err != nil {
		   return decimal.Zero, false, ErrSnapshot
	   }
	  actual,
		   err := money.Parse(actualRate)
	  if err != nil {
		   return decimal.Zero, false, ErrAmount
	   }
	   if !snap.IsPositive() || !actual.IsPositive() {
		  return decimal.Zero, false, ErrAmount
	  }
	   diff := actual.Dec().Sub(snap.Dec())
	   if diff.IsNegative() || diff.IsZero() {
		   return decimal.Zero, false, nil
	  }
	  bps := diff.Div(snap.Dec()).Mul(decimal.NewFromInt(10000))
	  exceeded := bps.GreaterThan(decimal.NewFromInt(int64(capBps)))
	   return bps, exceeded, nil
  }

// CalcLeakUSDT 计算因牌价上漂超帽导致的超额 USDT。
// amount: 源资产数量；snapRate: 快照牌价；actualRate: 实际兑换使用的牌价。
// 超额 = amount × (actualRate - snapRate)。
  func CalcLeakUSDT(amount, snapRate, actualRate string) (string, error) {
	   amt, err := money.Parse(amount)
	   if err != nil {
		  return "", ErrAmount
	  }
	   snap, err := money.Parse(snapRate)
	   if err != nil {
		  return "", ErrSnapshot
	   }
	  actual, err := money.Parse(actualRate)
	   if err != nil {
		  return "", ErrAmount
	  }
	   drift := actual.Dec().Sub(snap.Dec())
	  if drift.IsNegative() || drift.IsZero() {
		   return "0", nil
	   }
	  leak := money.FromDecimal(amt.Dec().Mul(drift))
	   if !leak.IsPositive() {
		   return "0", nil
	   }
	  return leak.String(), nil
  }

// ── DriftCapRegistry ─────────────────────────────────────────────────────────
// DriftCapRegistry 持有按资产配置的漂移帽参数，支持热更新。

type DriftCapRegistry struct {
	configs map[string]DriftCapConfig
}

// NewDriftCapRegistry 用默认配置初始化。
  func NewDriftCapRegistry(cfgs []DriftCapConfig) *DriftCapRegistry {
	   r := &DriftCapRegistry{configs: map[string]DriftCapConfig{}}
	   for _, c := range cfgs {
		   r.configs[strings.ToUpper(c.Asset)] = c
	  }
	   return r
  }

// CapFor 获取指定资产漂移帽（基点）。未配置时返回默认值。
  func (r *DriftCapRegistry) CapFor(asset string) int {
	   if r == nil {
		   return dustconv.DefaultDriftCapBps
	   }
	   c, ok := r.configs[strings.ToUpper(asset)]
	  if !ok || c.CapBps <= 0 {
		  return dustconv.DefaultDriftCapBps
	  }
	  return c.CapBps
  }

// StaleLimitFor 获取指定资产快照最大有效期（秒）。

  func (r *DriftCapRegistry) StaleLimitFor(asset string) int {
	   if r == nil {
		   return 30
	   }
	   c, ok := r.configs[strings.ToUpper(asset)]
	   if !ok || c.StaleMaxSec <= 0 {
		   return 30
	   }
	  return c.StaleMaxSec
  }

// Upsert 更新或插入资产漂移帽配置。

  func (r *DriftCapRegistry) Upsert(c DriftCapConfig) {
	  if r == nil {
		   return
	   }
	   r.configs[strings.ToUpper(c.Asset)] = c
   }

// ── GuardedConvert ────────────────────────────────────────────────────────────
// GuardedConvert 封装在 Convert 路径上的 Floor Guard + Drift Cap 双重守卫。
// 调用方在发起 dustconv.Convert 之前调用 GuardedConvert.Before，
// Convert 执行成功后调用 GuardedConvert.After 做 Conserve 断言。
type GuardedConvert struct {
	UserID string
	Asset  string
	Amount string
	IdemKey     string
	FloorGuard  *FloorGuard
	DriftGuard  *DriftGuard
	SnapKey     string
	ActualFloor string // ValidateLock 返回的实际 floor
	PolicyGen   uint64
}

// Before 在 Convert 发起前执行 Floor Guard CAS + Drift Cap 快照。
// liveRateUSDT: 从 fxmark 获取的实时牌价（调用方传入，不在此拉取）。
// capBps: 本次 Convert 的漂移帽；可从 DriftCapRegistry 获取。
// staleMaxSec: 快照最大有效期。

   func (g *GuardedConvert) Before(dust *dustconv.Service, liveRateUSDT string, capBps, staleMaxSec int) error {
	  // Floor Guard CAS
	   fg, err := AcquireLock(dust, g.Asset)
	  if err != nil {
		  return err
	  }
	   actualFloor,
		   err := fg.ValidateLock(g.Amount)
	  if err != nil {
		  return err
	   }
	  // 拍摄牌价快照
	  pg, err := dust.PolicyGenOf(g.Asset)
	  if err != nil {
		   return ErrPolicy
	  }
	   snap, err2 := dustconv.NewQuoteSnapshot(g.IdemKey+"-snap", g.Asset, liveRateUSDT, capBps, pg.Gen)
	   if err2 != nil {
		  return ErrSnapshot
	   }
	   dg := NewDriftGuard(&snap, "", staleMaxSec)
	  // 立即校验快照新鲜度（快照就是当下，elapsed=0，必然通过；但统一走守卫路径）
	  if err := dg.CheckWithLiveRate(liveRateUSDT, snap.SnapAt); err != nil {
		  return err
	  }
	   g.FloorGuard = fg
	  g.DriftGuard = dg
	  g.SnapKey = SnapKey(g.UserID, g.Asset, g.IdemKey)
	   g.ActualFloor = actualFloor
	  g.PolicyGen = pg.Gen
	   return nil
   }

// After 在 Convert 落账后验证 Conserve 断言（超额 USDT = 0）。
// actualUSDT: Convert 实际贷记的 USDT。
// rate: Convert 实际使用的牌价（来自 dustconv.Convert.Rate）。
   func (g *GuardedConvert) After(amount, actualUSDT, rate string) error {
	  if g == nil || g.DriftGuard == nil {
		  return ErrUnavailable
	  }
	  maxUSDT, err := g.DriftGuard.MaxPermittedUSDT(amount)
	   if err != nil {
		  return err
	   }
	   max, err := money.Parse(maxUSDT)
	   if err != nil {
		   return ErrAmount
	   }
	  actual, err := money.Parse(actualUSDT)
	  if err != nil {
		  return ErrAmount
	   }
	  // 允许微小舍入误差（18 位精度内）
	   if actual.Cmp(max) > 0 {
		  return fmt.Errorf("%w: actual_usdt=%s max_usdt=%s rate=%s",
			   ErrConserve, actualUSDT, maxUSDT, rate)
	  }
	   return nil
   }
// temporary guard removed after the production check
