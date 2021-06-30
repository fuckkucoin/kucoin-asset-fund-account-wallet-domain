package fundgraph

import "errors"

var (
	// ErrConflict 同一边键已存在且业务属性不一致。原边保持不变。
	ErrConflict = errors.New("fundgraph: conflict")
	// ErrInvalid 事实缺少主键字段、金额非法或类型不在冻结枚举内。
	ErrInvalid = errors.New("fundgraph: invalid")
	// ErrNotFound 节点、边或巡检单不存在。
	ErrNotFound = errors.New("fundgraph: not found")
	// ErrPage 分页令牌与本次查询条件不一致，或令牌损坏。
	ErrPage = errors.New("fundgraph: page token")
	// ErrStatus 巡检单状态不允许该迁移。
	ErrStatus = errors.New("fundgraph: incident status")
	// ErrDeleteDisabled 保留策略默认只标记冷热，不删除边。
	ErrDeleteDisabled = errors.New("fundgraph: delete disabled")
	// ErrConfirm 删除确认口令与计划不一致。
	ErrConfirm = errors.New("fundgraph: confirm mismatch")
	// ErrSnapshot 快照被截断，巡检拒绝在残缺图上开单。
	ErrSnapshot = errors.New("fundgraph: snapshot truncated")
	// ErrSkip 总线种类本包不消费。
	ErrSkip = errors.New("fundgraph: skip")
)

// WritePath 是本包唯一写入路径的名字。调用方用它区分图存储和其它只追加仓。
const WritePath = "fund_graph_store"
