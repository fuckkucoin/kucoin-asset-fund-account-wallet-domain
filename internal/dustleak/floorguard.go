package dustleak

import (
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/dustconv"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

// FloorGuard 封装针对一次 Convert 请求的 Floor Guard CAS。
// 非并发安全；每次 Convert 请求独立实例化。
type FloorGuard struct {
	asset string
	snapGen   uint64
	snapFloor string
	dust      *dustconv.Service
}

// AcquireLock 在 Convert 请求进入临界区之前快照策略世代。
// 返回的 FloorGuard 须在整个 Convert 执行链路中持有，直到 ValidateLock 调用。

  func AcquireLock(dust *dustconv.Service, asset string) (*FloorGuard, error) {
	   if dust == nil {
		   return nil, ErrUnavailable
	   }
	  as, err := validate.Asset(asset)
	  if err != nil {
		  return nil, ErrAsset
	  }
	   pg, err := dust.PolicyGenOf(as)
	  if err != nil {
		   return nil, ErrPolicy
	   }
	   if !pg.Enabled {
		  return nil, ErrPolicy
	   }
	  return &FloorGuard{
		  asset:     as,
		  snapGen:   pg.Gen,
		   snapFloor: pg.Floor,
		  dust:      dust,
	   }, nil
  }

// ValidateLock 在 Convert 执行时再次校验世代。
// amount 为本次兑换金额（须 < floorToUse 才是尘）。
// 返回 (floorToUse, nil) 表示校验通过；返回 ("", ErrGenMismatch) 须终止整单。
// 不变量：返回的 floorToUse 永远 ≥ 快照时的 floor。
// 即：策略可以变紧（floor 增大），不能因策略变松而扩大兑出上限。

   func (fg *FloorGuard) ValidateLock(amount string) (string, error) {
	  if fg == nil || fg.dust == nil {
		   return "", ErrUnavailable
	  }
	   cur, err := fg.dust.PolicyGenOf(fg.asset)
	  if err != nil {
		  return "", ErrPolicy
	   }
	  if cur.Gen == fg.snapGen {
		  // 世代未变，直接使用快照 floor
		  return fg.snapFloor, nil
	   }
	  // 世代已变
	   snapF, err2 := money.Parse(fg.snapFloor)
	  if err2 != nil {
		  return "", ErrAmount
	  }
	   curF, err3 := money.Parse(cur.Floor)
	   if err3 != nil {
		  return "", ErrAmount
	  }
	   switch curF.Cmp(snapF) {
	  case 1:
		   // 当前 floor 更大（更严）：使用更严的 floor，Convert 可接着但须重新验证 amount
		   amt, err := money.Parse(amount)
		   if err != nil {
			   return "", ErrAmount
		   }
		  if amt.Cmp(curF) >= 0 {
			   // amount >= 新 floor：不再是尘，Convert 须拒绝
			   return "", fmt.Errorf("%w: amount=%s no longer below new_floor=%s gen_snap=%d gen_cur=%d",
				  ErrFloor, amount, cur.Floor, fg.snapGen, cur.Gen)
		  }
		  return cur.Floor, nil
	   case -1:
		   // 当前 floor 更小（更松）：策略放松会扩大兑出上限，不允许
		   return "", fmt.Errorf("%w: floor relaxed from=%s to=%s gen_snap=%d gen_cur=%d",
			   ErrGenMismatch, fg.snapFloor, cur.Floor, fg.snapGen, cur.Gen)
	  default:
		   // floor 相同，只是其他字段变更，世代前进但不影响兑出量
		  return cur.Floor, nil
	   }
  }

// SnapGen 返回快照时的世代号（用于日志与追踪）。

   func (fg *FloorGuard) SnapGen() uint64 {
	   if fg == nil {
		   return 0
	  }
	   return fg.snapGen
   }

// SnapFloor 返回快照时的 floor 值。

   func (fg *FloorGuard) SnapFloor() string {
	   if fg == nil {
		  return ""
	  }
	  return fg.snapFloor
  }

// FloorGuardForConvert 一体化 AcquireLock + ValidateLock 调用，
// 适合在 Convert 同步路径下单次验证（快照与验证之间无中间步骤时使用）。
  func FloorGuardForConvert(dust *dustconv.Service, asset, amount string) (string, uint64, error) {
	  fg, err := AcquireLock(dust, asset)
	   if err != nil {
		   return "", 0, err
	   }
	  floor, err := fg.ValidateLock(amount)
	   if err != nil {
		   return "", fg.snapGen, err
	  }
	   return floor, fg.snapGen, nil
  }

// PolicyGenAssertion 断言：在 Convert 完成后，实际 floor 必须 ≤ 快照 floor。
// 若违反（即策略在 Convert 执行期间被放松且本次兑出已超新 floor），
// 视为潜在泄漏，须立即触发 LeakDetector 扫描。
// actualFloor: Convert 最终使用的 floor（FloorGuard 返回值）
// snapFloor: 快照时的 floor
// 返回 true 表示无潜在泄漏；false 表示须发起扫描。

   func PolicyGenAssertion(actualFloor, snapFloor string) (bool, error) {
	  af, err := money.Parse(actualFloor)
	   if err != nil {
		   return false, ErrAmount
	   }
	  sf,
		  err := money.Parse(snapFloor)
	   if err != nil {
		  return false, ErrAmount
	   }
	  // actualFloor 应 ≥ snapFloor（只能更严，不能更松）
	  if af.Cmp(sf) < 0 {
		   return false, fmt.Errorf("floor regression detected: actual_floor=%s snap_floor=%s",
			   actualFloor, snapFloor)
	   }
	   return true, nil
   }

// ValidateAmountBelowFloor 校验 amount < floor（是尘）。
// 内联于 Convert 守卫；避免调用方遗漏此基础校验。
  func ValidateAmountBelowFloor(amount, floor string) error {
	  amt, err := money.Parse(amount)
	   if err != nil {
		  return ErrAmount
	  }
	   f, err := money.Parse(floor)
	  if err != nil {
		  return ErrAmount
	  }
	   if amt.Cmp(f) >= 0 {
		   return fmt.Errorf("%w: amount=%s floor=%s", ErrFloor, amount, floor)
	   }
	   return nil
   }

// ValidateAmountAboveMin 校验 amount >= minConvert（最小兑换量）。
  func ValidateAmountAboveMin(amount, minConvert string) error {
	   amt, err := money.Parse(amount)
	   if err != nil {
		  return ErrAmount
	   }
	   min, err := money.Parse(minConvert)
	   if err != nil {
		  return ErrAmount
	   }
	  if amt.Cmp(min) < 0 {
		   return fmt.Errorf("%w: amount=%s min_convert=%s", ErrAmount, amount, minConvert)
	  }
	  return nil
  }

// AssertFloorChain 完整 floor 链断言（用于单元与集成验证）：
// 1. snapFloor ≤ curFloor（只能变严，不能放松）
// 2. amount < curFloor（确实是尘）

   func AssertFloorChain(amount, snapFloor, curFloor string) error {
	   sf, err := money.Parse(snapFloor)
	   if err != nil {
		   return ErrAmount
	   }
	   cf, err := money.Parse(curFloor)
	  if err != nil {
		  return ErrAmount
	   }
	  if cf.Cmp(sf) < 0 {
		   return fmt.Errorf("%w: floor relaxed snap=%s cur=%s", ErrGenMismatch, snapFloor, curFloor)
	   }
	   return ValidateAmountBelowFloor(amount, curFloor)
   }

// PolicyGenLabel 便于日志输出的世代标签。
   func PolicyGenLabel(asset string, gen uint64) string {
	   return fmt.Sprintf("%s#%d", strings.ToUpper(strings.TrimSpace(asset)), gen)
   }
