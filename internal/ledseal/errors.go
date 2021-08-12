package ledseal

import "fmt"

var (
	// ErrUnavailable 服务未初始化或存储不可用。
	ErrUnavailable = fmt.Errorf("ledseal unavailable")
	// ErrPlanNotFound 调度计划不存在。
	ErrPlanNotFound = fmt.Errorf("ledseal plan not found")
	// ErrPlanDuplicate 计划名称已存在。
	ErrPlanDuplicate = fmt.Errorf("ledseal plan duplicate name")
	// ErrPlanState 计划状态不允许此操作。
	ErrPlanState = fmt.Errorf("ledseal plan state invalid for operation")
	// ErrSliceNotFound 快照切片不存在。
	ErrSliceNotFound = fmt.Errorf("ledseal snap slice not found")
	// ErrSliceGap 快照切片序号不连续，检测到间隙。
	ErrSliceGap = fmt.Errorf("ledseal snap slice gap detected")
	// ErrSliceRollback 快照切片序号回退，可能发生回放攻击。
	ErrSliceRollback = fmt.Errorf("ledseal snap slice seq rollback")
	// ErrBatchNotFound 差分批次不存在。
	ErrBatchNotFound = fmt.Errorf("ledseal diff batch not found")
	// ErrBatchState 批次状态不允许此操作。
	ErrBatchState = fmt.Errorf("ledseal diff batch state invalid")
	// ErrBatchAlreadySealed 批次已封印，不得重复封印。
	ErrBatchAlreadySealed = fmt.Errorf("ledseal diff batch already sealed")
	// ErrDriftNotFound 漂移行不存在。
	ErrDriftNotFound = fmt.Errorf("ledseal drift row not found")
	// ErrSealRootNotFound 封印根不存在。
	ErrSealRootNotFound = fmt.Errorf("ledseal seal root not found")
	// ErrSealRootMismatch 封印根哈希校验失败。
	ErrSealRootMismatch = fmt.Errorf("ledseal seal root hash mismatch")
	// ErrStale 围栏代次过期，需重试。
	ErrStale = fmt.Errorf("ledseal stale fence")
	// ErrUser 用户 ID 无效。
	ErrUser = fmt.Errorf("ledseal user invalid")
	// ErrParam 请求参数无效。
	ErrParam = fmt.Errorf("ledseal request param invalid")
	// ErrReplay 回放一致性校验失败。
	ErrReplay = fmt.Errorf("ledseal replay fingerprint mismatch")
	// ErrAmount 金额形态无效。
	ErrAmount = fmt.Errorf("ledseal amount invalid")
)
