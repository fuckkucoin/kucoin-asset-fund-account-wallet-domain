package travelvasp

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/fxmark"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/pkg/money"
)

// CorridorThreshold 按走廊（origin 国家→peer 国家，或 VASP 对）配置旅行规则金额门槛。
// 未命中走廊时回落 Default；挂 USD 牌价后 MinAmount 按美元名义解释。
// 门槛只决定是否开报文，不替代 KYT；缺配置不得默认放行大额。
type CorridorThreshold struct {
	Corridor  string `json:"corridor" yaml:"corridor"` // 形如 SG>US / KC01>PEER / DEFAULT
	Asset     string `json:"asset,omitempty" yaml:"asset"`
	MinAmount string `json:"min_amount" yaml:"min_amount"`
}

type ThresholdBook struct {
	mu sync.RWMutex
	defMin string
	rows     map[string]CorridorThreshold // key = corridor|asset
	usdRates map[string]string            // 非稳定币→USD 配置牌价
	rateSrc map[string]string // asset → static|live
	rateAt   map[string]time.Time // asset → 最近刷新/写入时刻
	staleMax time.Duration        // Live 陈旧阈值；运营健康面
	sql *persist.SQLDB // 权威源；空则仅内存
	fx *fxmark.Book // Live/Static 名义源
}

// NewThresholdBook 构造走廊门槛表；defaultMin 空则视为 "0"（凡有绑定旅行服务即开报文）。
   func NewThresholdBook(defaultMin string, rows []CorridorThreshold) *ThresholdBook {
	   b := &ThresholdBook{defMin: strings.TrimSpace(defaultMin), rows: map[string]CorridorThreshold{}}
	   if b.defMin == "" {
		   b.defMin = "0"
	  }
	   for _, r := range rows {
		   b.Put(r)
	  }
	   return b
   }

  func threshKey(corridor, asset string) string {
	  c := strings.ToUpper(strings.TrimSpace(corridor))
	   a := strings.ToUpper(strings.TrimSpace(asset))
	   if c == "" {
		   c = "DEFAULT"
	  }
	  return c + "|" + a
   }

// Put 写入或覆盖一条走廊门槛。

   func (b *ThresholdBook) Put(r CorridorThreshold) {
	   if b == nil {
		   return
	  }
	  r.Corridor = strings.ToUpper(strings.TrimSpace(r.Corridor))
	   r.Asset = strings.ToUpper(strings.TrimSpace(r.Asset))
	  r.MinAmount = strings.TrimSpace(r.MinAmount)
	   if r.Corridor == "" {
		  r.Corridor = "DEFAULT"
	  }
	  if r.MinAmount == "" {
		  r.MinAmount = "0"
	   }
	  b.mu.Lock()
	   b.rows[threshKey(r.Corridor, r.Asset)] = r
	  if r.Asset != "" {
		   // also keep asset-agnostic row untouched
	   }
	  b.mu.Unlock()
  }

// Lookup 解析走廊门槛是否登记。rows 或 Default 任一命中即 found；簿空且未绑 Default 视为走廊缺失。

   func (b *ThresholdBook) Lookup(corridor, asset string) (string, bool) {
	  if b == nil {
		  return "", false
	   }
	   corridor = strings.ToUpper(strings.TrimSpace(corridor))
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	  if corridor == "" {
		   corridor = "DEFAULT"
	  }
	  b.mu.RLock()
	   defer b.mu.RUnlock()
	   for _, k := range []string{
		  threshKey(corridor, asset),
		   threshKey(corridor, ""),
		  threshKey("DEFAULT", asset),
		   threshKey("DEFAULT", ""),
	   } {
		   if r, ok := b.rows[k]; ok {
			  return r.MinAmount, true
		   }
	   }
	  if strings.TrimSpace(b.defMin) != "" {
		   return b.defMin, true
	   }
	  return "", false
   }

// Resolve 按走廊与资产解析门槛；先 corridor|asset，再 corridor|，再 DEFAULT|asset，再 DEFAULT|，最后 DefaultMin。

   func (b *ThresholdBook) Resolve(corridor, asset string) string {
	  if b == nil {
		  return "0"
	   }
	   corridor = strings.ToUpper(strings.TrimSpace(corridor))
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	  if corridor == "" {
		  corridor = "DEFAULT"
	   }
	   b.mu.RLock()
	  defer b.mu.RUnlock()
	  for _, k := range []string{
		   threshKey(corridor, asset),
		   threshKey(corridor, ""),
		   threshKey("DEFAULT", asset),
		   threshKey("DEFAULT", ""),
	   } {
		   if r, ok := b.rows[k]; ok {
			  return r.MinAmount
		  }
	  }
	   return b.defMin
  }

// CorridorKey 由两国代码或两 VASP 代码拼走廊键。
   func CorridorKey(origin, peer string) string {
	  o := strings.ToUpper(strings.TrimSpace(origin))
	   p := strings.ToUpper(strings.TrimSpace(peer))
	  if o == "" {
		   o = "XX"
	  }
	   if p == "" {
		   p = "XX"
	   }
	   return o + ">" + p
  }

// RequiresTravel 金额达到或超过走廊门槛则须开旅行规则报文。
// 门槛为 0 表示绑定服务后一律开报文；有 USD 牌价时按名义美元比较；解析失败按须开处理（失败关闭）。
// 不得因门槛配置损坏或缺非稳定币牌价而静默跳过核验闸。

  func (b *ThresholdBook) RequiresTravel(corridor, asset, amount string) (bool, string, error) {
	   minStr := "0"
	   if b != nil {
		   minStr = b.Resolve(corridor, asset)
	  }
	  minAmt, err := money.ParseBook(minStr)
	  if err != nil {
		  return true, minStr, fmt.Errorf("travel threshold invalid: %w", err)
	   }
	   if minAmt.IsZero() {
		  return true, minStr, nil
	   }
	   cmpAmount := amount
	   useUSD := b != nil && b.hasUsdRates()
	  if useUSD {
		   notional, nerr := b.UsdNotional(asset, amount)
		   if nerr != nil {
			  // 非稳定币缺牌价：失败关闭，须开报文
			   return true, minStr, nerr
		  }
		  cmpAmount = notional
	  }
	   amt, err := money.Parse(cmpAmount)
	   if err != nil {
		  return true, minStr, err
	   }
	  return amt.Cmp(minAmt) >= 0,
		  minStr, nil
  }

  func (b *ThresholdBook) hasUsdRates() bool {
	  if b == nil {
		   return false
	  }
	  b.mu.RLock()
	  defer b.mu.RUnlock()
	  return len(b.usdRates) > 0
  }

// List 导出当前门槛表（含 Default）。
  func (b *ThresholdBook) List() []CorridorThreshold {
	   if b == nil {
		   return nil
	  }
	  b.mu.RLock()
	  defer b.mu.RUnlock()
	  out := make([]CorridorThreshold,
		  0, len(b.rows)+1)
	  out = append(out, CorridorThreshold{Corridor: "DEFAULT", MinAmount: b.defMin})
	  for _, r := range b.rows {
		  out = append(out, r)
	  }
	   return out
   }

// BindUsdRates 挂非稳定币→USD 牌价，门槛按美元名义比较。
// 稳定币（USDT/USDC/USD/DAI）面值近似 USD；缺牌价的非稳定币失败关闭（须开报文）。
// 牌价须配置驱动，不得在运行时臆造市价。
  func (b *ThresholdBook) BindUsdRates(rates map[string]string) *ThresholdBook {
	  if b == nil {
		  return b
	   }
	   b.mu.Lock()
	  defer b.mu.Unlock()
	   if b.usdRates == nil {
		   b.usdRates = map[string]string{}
	   }
	  for k, v := range rates {
		  asset := strings.ToUpper(strings.TrimSpace(k))
		   rate := strings.TrimSpace(v)
		  if asset == "" || rate == "" {
			  continue
		  }
		   b.usdRates[asset] = rate
	  }
	   return b
   }

// PutUsdRate 热更新单资产 USD 牌价。

   func (b *ThresholdBook) PutUsdRate(asset, usd string) error {
	  if b == nil {
		   return fmt.Errorf("threshold book nil")
	   }
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	  usd = strings.TrimSpace(usd)
	  if asset == "" {
		   return fmt.Errorf("asset required")
	  }
	  n, err := money.Parse(usd)
	  if err != nil || !n.IsPositive() {
		   return fmt.Errorf("usd rate invalid")
	  }
	   b.mu.Lock()
	   if b.usdRates == nil {
		   b.usdRates = map[string]string{}
	  }
	  b.usdRates[asset] = n.String()
	   b.mu.Unlock()
	   return nil
   }

// ListUsdRates 导出当前 USD 牌价表。
   func (b *ThresholdBook) ListUsdRates() map[string]string {
	   if b == nil {
		   return nil
	   }
	   b.mu.RLock()
	  defer b.mu.RUnlock()
	  out := make(map[string]string, len(b.usdRates))
	   for k, v := range b.usdRates {
		  out[k] = v
	   }
	  return out
  }

  func isStableUSD(asset string) bool {
	   switch strings.ToUpper(strings.TrimSpace(asset)) {
	  case "USD", "USDT", "USDC", "DAI":
		   return true
	   default:
		  return false
	  }
  }

// UsdNotional 把资产数量折算为 USD 名义；稳定币按 1:1。
// 有牌价表时门槛按 USD；无牌价非稳定币返回错误供失败关闭。
// 不得因缺价而按 0 放行。

  func (b *ThresholdBook) UsdNotional(asset, amount string) (string, error) {
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	   amt, err := money.Parse(amount)
	  if err != nil {
		  return "", err
	  }
	   if isStableUSD(asset) {
		  return amt.String(), nil
	  }
	   if b == nil {
		   return "", fmt.Errorf("usd rate missing for %s", asset)
	   }
	  b.mu.RLock()
	   rateStr, ok := b.usdRates[asset]
	   b.mu.RUnlock()
	  if !ok || strings.TrimSpace(rateStr) == "" {
		  return "", fmt.Errorf("usd rate missing for %s", asset)
	  }
	   rate, err := money.Parse(rateStr)
	  if err != nil || !rate.IsPositive() {
		  return "", fmt.Errorf("usd rate invalid for %s", asset)
	   }
	  got := money.FromDecimal(amt.Dec().Mul(rate.Dec()))
	  if got.IsNegative() {
		   return "", fmt.Errorf("usd notional negative")
	  }
	   return got.String(), nil
   }

// Delete 删除一条走廊门槛（不含 DEFAULT 面值）。

  func (b *ThresholdBook) Delete(corridor, asset string) {
	  if b == nil {
		  return
	   }
	  b.mu.Lock()
	   delete(b.rows, threshKey(corridor, asset))
	   b.mu.Unlock()
  }

// SetDefault 热更新默认门槛金额。
   func (b *ThresholdBook) SetDefault(minAmount string) {
	   if b == nil {
		  return
	   }
	   minAmount = strings.TrimSpace(minAmount)
	  if minAmount == "" {
		   minAmount = "0"
	  }
	  b.mu.Lock()
	  b.defMin = minAmount
	   b.mu.Unlock()
  }

// DefaultMin 读取当前默认门槛。

   func (b *ThresholdBook) DefaultMin() string {
	  if b == nil {
		   return "0"
	  }
	  b.mu.RLock()
	  defer b.mu.RUnlock()
	  return b.defMin
   }

// ReplaceAll 用新表整体热替换走廊门槛（保留 USD 牌价）。
// 运营 PUT 全量回写；空表仅保留 Default。
// 替换期间读锁短暂不可见中间态，由写锁串行。

   func (b *ThresholdBook) ReplaceAll(defaultMin string, rows []CorridorThreshold) {
	   if b == nil {
		   return
	  }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	   if strings.TrimSpace(defaultMin) == "" {
		  defaultMin = "0"
	   }
	   b.defMin = strings.TrimSpace(defaultMin)
	  b.rows = map[string]CorridorThreshold{}
	  for _, r := range rows {
		  r.Corridor = strings.ToUpper(strings.TrimSpace(r.Corridor))
		  r.Asset = strings.ToUpper(strings.TrimSpace(r.Asset))
		  r.MinAmount = strings.TrimSpace(r.MinAmount)
		  if r.Corridor == "" {
			   r.Corridor = "DEFAULT"
		  }
		  if r.MinAmount == "" {
			   r.MinAmount = "0"
		  }
		   b.rows[threshKey(r.Corridor, r.Asset)] = r
	  }
   }
