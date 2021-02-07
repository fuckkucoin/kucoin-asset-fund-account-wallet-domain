package agreeleasegate

// LeaseFSMDoc 供运维与对账引用的状态机摘要。
var LeaseFSMDoc = []string{
	"agreed 仅由 AgreeWithLease 进入；同意码必须为空。",
	"planned 仅由 PlanAssert 自 agreed 进入；必须复验 bal+rule。",
	"executing 仅由 ExecAssert 自 planned 进入；禁止跳过 Plan。",
	"consumed 仅自 executing 进入；同一租约不得二次消费。",
	"expired 由 TTL 扫描或实时守卫进入；不可回滚。",
	"revoked 由 bal/rule/kinds/水位/运维进入；必须可关联证明。",
}

// FailClosedRules 资金写路径失败关闭规则。
var FailClosedRules = []string{
	"缺 agree_lease_id 的 Plan/Exec 必须拒绝并尽量落 missing_lease 证明。",
	"同意码非空不得签发租约，不得映射为成功。",
	"open 证明或半截挡写期间禁止放腿。",
	"漂移复验失败后不得 appendLink、不得 Fill。",
	"不改写 coolmarkgate / trexecgate / sweeppol.Evaluate / treasury 主链。",
}

// ProofTypesDoc 证明类型与拒因对应。

var ProofTypesDoc = map[DriftType]string{
	DriftBal:       "影子余额指纹变化或跌破水位",
	DriftRule:      "规则指纹变化",
	DriftVersion:   "规则版本抬升",
	DriftWaterline: "水位参数变化或余额不再超额",
	DriftMissingLease: "Plan/Exec 未携带租约",
	DriftExpired:  "TTL 外推进",
	DriftHalfPlan: "先 Exec 后 Plan 或 planned 超时",
	DriftKinds:       "归集种类集合变化",
	DriftAgreeFailed: "活读闸同意码非空",
}
