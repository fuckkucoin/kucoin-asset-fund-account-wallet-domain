package dustleak

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/dustconv"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

const (
	// DoubleDustWindowSec 双花检测窗口（秒）。同用户同资产在此窗口内两笔以上认为可疑。
	DoubleDustWindowSec = 300
	// DetectorSourceWorker 扫描来源：后台 worker。
	DetectorSourceWorker = "detector-worker"
	// DetectorSourceAPI 扫描来源：运营触发。
	DetectorSourceAPI = "detector-api"
)

// ConvertRecord 对已落账 dustconv.Convert 的轻量快照，供 Detector 扫描使用。
// 字段语义与 dustconv.Convert 对应；不持有原始指针以避免竞态。
type ConvertRecord struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
	Asset string `json:"asset"`
	Amount string `json:"amount"` // 源资产数量
	USDT string `json:"usdt"` // 实际贷记 USDT
	Rate string `json:"rate"` // 实际兑换牌价
	At      time.Time `json:"at"`
	IdemKey string    `json:"idem_key"`
}

// DetectorResult 单次扫描结果。
type DetectorResult struct {
	Scanned int `json:"scanned"`
	Detected int `json:"detected"`
	Cases []*LeakCase `json:"cases"`
	Errors []string `json:"errors"`
}

// Detector 泄漏扫描器。
type Detector struct {
	svc *Service
	dust     *dustconv.Service
	driftReg *DriftCapRegistry
}

// NewDetector 创建扫描器。

  func NewDetector(svc *Service, dust *dustconv.Service, driftReg *DriftCapRegistry) *Detector {
	  return &Detector{svc: svc, dust: dust, driftReg: driftReg}
   }

// ScanConverts 对一批 ConvertRecord 执行全量泄漏扫描。
// source: 来源标签（DetectorSourceWorker / DetectorSourceAPI）。

  func (d *Detector) ScanConverts(records []ConvertRecord, source string) *DetectorResult {
	   result := &DetectorResult{}
	  if d == nil || d.svc == nil {
		  return result
	   }
	   if source == "" {
		   source = DetectorSourceWorker
	   }
	   result.Scanned = len(records)
	   for _, rec := range records {
		  rec := rec // 避免闭包捕获
		  cases,
			  errs := d.scanOne(rec, source)
		  result.Cases = append(result.Cases, cases...)
		  result.Errors = append(result.Errors, errs...)
		  result.Detected += len(cases)
	  }
	  // 双花扫描（跨记录分析）
	  doubleCases, errs := d.scanDoubleDust(records, source)
	   result.Cases = append(result.Cases, doubleCases...)
	   result.Errors = append(result.Errors, errs...)
	   result.Detected += len(doubleCases)
	  return result
   }

// scanOne 对单条 ConvertRecord 进行 OverFloor 与 DriftOverpay 扫描。

   func (d *Detector) scanOne(rec ConvertRecord, source string) ([]*LeakCase, []string) {
	  var cases []*LeakCase
	   var errs []string
	   // 跳过已有 LeakCase 的 Convert
	   if existing, _ := d.svc.GetLeakCaseByConvert(rec.ID); existing != nil {
		  return nil, nil
	   }
	   // 1. OverFloor 扫描
	  if lc, err := d.checkOverFloor(rec, source); err != nil {
		   errs = append(errs, fmt.Sprintf("over_floor scan convert=%s: %v", rec.ID, err))
	  } else if lc != nil {
		  cases = append(cases, lc)
		  return cases, errs // 同一笔 Convert 已有 LeakCase，不再做 DriftOverpay
	   }
	   // 2. DriftOverpay 扫描
	   if lc, err := d.checkDriftOverpay(rec, source); err != nil {
		   errs = append(errs, fmt.Sprintf("drift_overpay scan convert=%s: %v", rec.ID, err))
	  } else if lc != nil {
		  cases = append(cases, lc)
	  }
	  return cases, errs
  }

// checkOverFloor 检查兑换金额是否超过当时策略 floor。
// 策略世代：从 QuoteSnapshot 关联的 PolicyGenID 回查当时 floor；
// 若无快照，则用当前策略 floor 做保守估计。
  func (d *Detector) checkOverFloor(rec ConvertRecord, source string) (*LeakCase, error) {
	   as, err := validate.Asset(rec.Asset)
	   if err != nil {
		  return nil, err
	  }
	   // 尝试获取转换时的策略快照
	   snapKey := SnapKey(rec.UserID, as, rec.IdemKey)
	   snap, _ := d.svc.SnapshotByKey(snapKey)
	  var floor string
	  var policyGenID string
	   var policyGen uint64
	   if snap != nil {
		  // 先使用快照绑定的世代对应的 floor
		   pg, err2 := d.svc.PolicyGenByGenNum(as, snap.PolicyGen)
		  if err2 == nil && pg != nil {
			   floor = pg.Floor
			   policyGenID = pg.ID
			   policyGen = pg.Gen
		  }
	   }
	   if floor == "" {
		  // 无快照时使用当前策略 floor（保守估计）
		  cur, err2 := d.dust.PolicyGenOf(as)
		  if err2 != nil {
			  return nil, ErrPolicy
		   }
		   floor = cur.Floor
		   policyGen = cur.Gen
	  }
	  f, err := money.Parse(floor)
	   if err != nil {
		  return nil, ErrAmount
	  }
	   amt, err := money.Parse(rec.Amount)
	  if err != nil {
		  return nil, ErrAmount
	   }
	  if amt.Cmp(f) < 0 {
		   // 金额确实低于 floor，无 OverFloor 泄漏
		   return nil, nil
	   }
	   // amount >= floor：非尘，超额兑出
	   // 超额量 = amount - (floor - epsilon)；简化为 amount 本身
	  leakAmt := rec.Amount
	  rate, err := money.Parse(rec.Rate)
	   if err != nil {
		   return nil, ErrAmount
	  }
	   actualUSDT := rec.USDT
	  // 允许的最大 USDT = (floor - 1 unit) * rate，简化为 0（因为 >= floor 根本不该兑）
	  maxUSDT := "0"
	   leakUSDT := actualUSDT
	   snapID := ""
	  if snap != nil {
		  snapID = snap.ID
	  }
	   _ = rate
	   lc, err := d.svc.OpenLeakCase(rec.ID, rec.UserID, as, LeakKindOverFloor,
		  rec.Amount, actualUSDT,
		   floor, maxUSDT,
		  leakAmt, leakUSDT,
		  snapID, policyGenID, source,
	   )
	   if err != nil {
		  return nil, err
	  }
	  DustLeakDetected.WithLabelValues(as,
		   string(LeakKindOverFloor)).Inc()
	   DustLeakLeakedUSDT.WithLabelValues(as, string(LeakKindOverFloor)).Add(mustParseFloat64(leakUSDT))
	   _ = policyGen
	  return lc, nil
  }

// checkDriftOverpay 检查实际兑换牌价是否相对快照上漂超帽。
  func (d *Detector) checkDriftOverpay(rec ConvertRecord, source string) (*LeakCase, error) {
	  as,
		  err := validate.Asset(rec.Asset)
	  if err != nil {
		   return nil, err
	  }
	  snapKey := SnapKey(rec.UserID,
		   as, rec.IdemKey)
	   snap, err := d.svc.SnapshotByKey(snapKey)
	  if err != nil || snap == nil {
		   // 无快照，无法做漂移比较（不产出 false positive）
		  return nil, nil
	   }
	  capBps := d.driftReg.CapFor(as)
	   bps, exceeded, err := DriftCapChecker(snap.RateUSDT, rec.Rate, capBps)
	   if err != nil {
		   return nil, err
	  }
	   if !exceeded {
		  return nil, nil
	   }
	   // 超帽：计算泄漏 USDT
	  leakUSDTStr, err := CalcLeakUSDT(rec.Amount, snap.RateUSDT, rec.Rate)
	   if err != nil {
		   return nil, err
	   }
	   maxUSDTStr,
		   err := calcMaxUSDT(rec.Amount, snap.RateUSDT)
	  if err != nil {
		   return nil, err
	  }
	   lc, err := d.svc.OpenLeakCase(rec.ID, rec.UserID, as, LeakKindDriftOverpay,
		  rec.Amount, rec.USDT,
		  snap.RateUSDT, maxUSDTStr,
		  "0", leakUSDTStr,
		  snap.ID, snap.PolicyGenID, source,
	  )
	  if err != nil {
		  return nil, err
	  }
	   DustLeakDetected.WithLabelValues(as, string(LeakKindDriftOverpay)).Inc()
	  DustLeakDetectedDriftBps.WithLabelValues(as).Observe(bps.InexactFloat64())
	  DustLeakLeakedUSDT.WithLabelValues(as, string(LeakKindDriftOverpay)).Add(mustParseFloat64(leakUSDTStr))
	   return lc, nil
   }

// scanDoubleDust 检测批量尘兑双花：同用户同资产在 DoubleDustWindowSec 内
// 非幂等多笔兑换（不同 idemKey）视为双花嫌疑，对超出第一笔的所有后续笔打 LeakCase。
  func (d *Detector) scanDoubleDust(records []ConvertRecord, source string) ([]*LeakCase, []string) {
	  var cases []*LeakCase
	  var errs []string
	   // 分组：userID+asset → sorted by time
	   type groupKey struct {
		   user,
		   asset string
	   }
	   groups := map[groupKey][]ConvertRecord{}
	  for _, rec := range records {
		  k := groupKey{rec.UserID,
			   rec.Asset}
		  groups[k] = append(groups[k], rec)
	   }
	  for k, recs := range groups {
		  if len(recs) <= 1 {
			   continue
		   }
		   sort.Slice(recs, func(i, j int) bool { return recs[i].At.Before(recs[j].At) })
		   // 滑动窗口
		  for i := 0; i < len(recs); i++ {
			   window := []ConvertRecord{recs[i]}
			  for j := i + 1; j < len(recs); j++ {
				  if recs[j].At.Sub(recs[i].At).Seconds() <= DoubleDustWindowSec {
					   window = append(window, recs[j])
				  } else {
					  break
				   }
			  }
			  if len(window) <= 1 {
				  continue
			   }
			  // 第 0 笔视为合法首笔；后续笔视为双花嫌疑
			  for _, dup := range window[1:] {
				   if existing, _ := d.svc.GetLeakCaseByConvert(dup.ID); existing != nil {
					  continue
				  }
				  lc, err := d.svc.OpenLeakCase(dup.ID, k.user, k.asset, LeakKindDoubleDust,
					  dup.Amount, dup.USDT,
					  "0", "0",
					  dup.Amount, dup.USDT,
					   "", "", source,
				  )
				  if err != nil {
					   errs = append(errs, fmt.Sprintf("double_dust detect convert=%s: %v", dup.ID, err))
					   continue
				   }
				  cases = append(cases, lc)
				   DustLeakDetected.WithLabelValues(k.asset, string(LeakKindDoubleDust)).Inc()
				  DustLeakLeakedUSDT.WithLabelValues(k.asset, string(LeakKindDoubleDust)).Add(mustParseFloat64(dup.USDT))
			   }
		   }
	  }
	   return cases, errs
   }

// ── 辅助方法（在 Service 上暴露给 Detector 使用）──────────────────────────────
// GetLeakCaseByConvert 按 convertID 查找已有 LeakCase（幂等守卫）。
   func (s *Service) GetLeakCaseByConvert(convertID string) (*LeakCase, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	  }
	  s.mu.Lock()
	   leakID, ok := s.leaksByConvert[convertID]
	  s.mu.Unlock()
	  if !ok {
		  if s.store != nil {
			  return s.store.LeakCaseByConvert(convertID)
		   }
		  return nil, nil
	   }
	  s.mu.Lock()
	  lc := s.leaks[leakID]
	   s.mu.Unlock()
	  if lc != nil {
		  cp := lc.Copy()
		   return &cp, nil
	   }
	  return nil, nil
  }

// SnapshotByKey 按 snapKey 获取 QuoteSnapshot。
  func (s *Service) SnapshotByKey(snapKey string) (*QuoteSnapshot, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	  }
	   s.mu.Lock()
	  snap := s.snapshots[snapKey]
	   s.mu.Unlock()
	  if snap != nil {
		   cp := snap.Copy()
		  return &cp, nil
	  }
	  if s.store != nil {
		  return s.store.SnapshotByKey(snapKey)
	   }
	   return nil, ErrNotFound
  }

// PolicyGenByGenNum 按资产 + 世代号查找 PolicyGen 记录。

   func (s *Service) PolicyGenByGenNum(asset string, gen uint64) (*PolicyGen, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	  }
	   s.mu.Lock()
	  list := s.gens[asset]
	   for _, g := range list {
		  if g.Gen == gen {
			  cp := g.Copy()
			   s.mu.Unlock()
			  return &cp, nil
		  }
	  }
	   s.mu.Unlock()
	  return nil, ErrNotFound
  }

// ── 内部工具 ─────────────────────────────────────────────────────────────────

   func calcMaxUSDT(amount, snapRate string) (string, error) {
	   amt, err := money.Parse(amount)
	  if err != nil {
		   return "", ErrAmount
	   }
	   rate, err := money.Parse(snapRate)
	  if err != nil {
		   return "", ErrSnapshot
	  }
	   max := money.FromDecimal(amt.Dec().Mul(rate.Dec()))
	   return max.String(), nil
   }

  func mustParseFloat64(s string) float64 {
	  s = strings.TrimSpace(s)
	  if s == "" || s == "0" {
		   return 0
	  }
	  v, err := money.Parse(s)
	  if err != nil {
		  return 0
	   }
	   f, _ := v.Dec().Float64()
	   return f
   }
