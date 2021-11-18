package regioinv

import "errors"

const (
	CodeHotBelow     = "firewall_hot_below_watermark"
	CodeRegionHalted = "firewall_region_halted"
	CodeBlocked        = "firewall_blocked"
	CodeCrossHot       = "firewall_cross_region_hot"
	CodeWrongRebalance = "firewall_wrong_rebalance"
	CodeWindow         = "firewall_outside_window"
	CodeQuota          = "firewall_quota"
	CodeWatermark = "firewall_watermark_missing"
	CodeNotFound = "regio_not_found"
	CodeInvalid  = "regio_invalid"
	CodeCatalog    = "regio_catalog"
	CodeSameRegion = "regio_same_region"
	CodeConserve   = "regio_conserve"
	CodeInsufficient = "regio_insufficient"
	CodeMinLeg       = "regio_min_leg"
	CodeStore  = "regio_store"
	CodeNoPath = "regio_no_path"
)

var (
	// ErrHotBelowWatermark 区热库存低于水位，或本笔会把它打穿水位。
	ErrHotBelowWatermark = errors.New("regioinv: hot below watermark")
	// ErrRegionHalted 区级隔离已 Engage。与全局 custodyhalt 不是同一错误。
	ErrRegionHalted = errors.New("regioinv: region halted")
	// ErrFirewallBlocked 区防火墙为 blocked（探测失败或隔离）。
	ErrFirewallBlocked = errors.New("regioinv: firewall blocked")
	// ErrCrossRegionHot 区处于故障或漂移时禁止跨区取热。
	ErrCrossRegionHot = errors.New("regioinv: cross-region hot withdraw blocked")
	// ErrWrongRebalance 方向与水位相反，或会打穿源地板/目标天花板。
	ErrWrongRebalance = errors.New("regioinv: wrong rebalance")
	// ErrOutsideWindow 任一侧水位窗口未开。起止相等表示全天。
	ErrOutsideWindow = errors.New("regioinv: outside window")
	// ErrQuota 源区日配额不足。空日帽表示不限额。
	ErrQuota = errors.New("regioinv: quota")
	// ErrWatermark 缺少启用的水位，失败即关。
	ErrWatermark = errors.New("regioinv: watermark missing")
	// ErrNotFound 区域、快照或水位不存在。
	ErrNotFound = errors.New("regioinv: not found")
	// ErrInvalid 入参不合法。
	ErrInvalid = errors.New("regioinv: invalid")
	// ErrCatalog 区域没有绑到 sg/tyo 目录。
	ErrCatalog = errors.New("regioinv: catalog")
	// ErrSameRegion 源和目标是同一库存区。
	ErrSameRegion = errors.New("regioinv: same region")
	// ErrConserve 划转前后数量不守恒。
	ErrConserve = errors.New("regioinv: conserve")
	// ErrInsufficient 可用不足，不把可用打成负数。
	ErrInsufficient = errors.New("regioinv: insufficient")
	// ErrMinLeg 低于最小腿。
	ErrMinLeg = errors.New("regioinv: min leg")
	// ErrStore 存储不可用。
	ErrStore = errors.New("regioinv: store")
	// ErrNoPath 没有同时具备盈余和缺口的开区。
	ErrNoPath = errors.New("regioinv: no path")
	// ErrActor 操作人必填。
	ErrActor = errors.New("regioinv: actor required")
	// ErrNotHalted Clear 时区并未隔离。
	ErrNotHalted = errors.New("regioinv: not halted")
)

// CodeOf 返回稳定错误码。全局 custodyhalt 不在本表内。
  func CodeOf(err error) string {
	   switch {
	  case err == nil:
		   return ""
	  case errors.Is(err,
		  ErrHotBelowWatermark):
		  return CodeHotBelow
	  case errors.Is(err, ErrRegionHalted):
		  return CodeRegionHalted
	  case errors.Is(err, ErrFirewallBlocked):
		  return CodeBlocked
	   case errors.Is(err, ErrCrossRegionHot):
		  return CodeCrossHot
	  case errors.Is(err, ErrWrongRebalance):
		   return CodeWrongRebalance
	  case errors.Is(err,
		  ErrOutsideWindow):
		   return CodeWindow
	   case errors.Is(err, ErrQuota):
		   return CodeQuota
	  case errors.Is(err, ErrWatermark):
		   return CodeWatermark
	  case errors.Is(err, ErrNotFound):
		   return CodeNotFound
	   case errors.Is(err, ErrCatalog):
		   return CodeCatalog
	   case errors.Is(err, ErrSameRegion):
		   return CodeSameRegion
	   case errors.Is(err, ErrConserve):
		   return CodeConserve
	   case errors.Is(err, ErrInsufficient):
		   return CodeInsufficient
	  case errors.Is(err, ErrMinLeg):
		   return CodeMinLeg
	   case errors.Is(err, ErrStore):
		   return CodeStore
	  case errors.Is(err, ErrNoPath):
		   return CodeNoPath
	  default:
		   return CodeInvalid
	  }
  }
