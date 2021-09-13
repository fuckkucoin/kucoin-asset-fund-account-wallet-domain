package oracle

// TickLeaseID 只读字段名。本包不签发租约，由 oracletickgate 产出。
const TickLeaseIDField = "tick_lease_id"

// ValuationPurpose 资金副作用估值入口。
type ValuationPurpose string

const (
	PurposeConvert ValuationPurpose = "convert"
	PurposeMTM     ValuationPurpose = "mark_to_market"
	PurposeLTV     ValuationPurpose = "margin_ltv"
)

// FundWriteSurface 只读枚举，供调用点一致。不改 Convert/MTM 公式。
type FundWriteSurface string

const (
	SurfaceCollateralMTM FundWriteSurface = "collateral_mtm_post"
	SurfaceLTVWrite      FundWriteSurface = "ltv_liquidation_write"
	SurfaceConvertPost FundWriteSurface = "convert_post"
)
