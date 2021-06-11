package fundmat

import "errors"

var (
	// ErrUnavailable 仓储未装配。
	ErrUnavailable = errors.New("fundmat store unavailable")
	// ErrNotFound 行、围栏或任务不存在。
	ErrNotFound = errors.New("fundmat not found")
	// ErrDuplicate 同一事件标识已进入日志。
	ErrDuplicate = errors.New("fundmat duplicate event")
	// ErrSeqConflict 流内序号已被其他事件占用。
	ErrSeqConflict = errors.New("fundmat sequence conflict")
	// ErrBadEvent 事件缺少用户、资产、科目或金额不合法。
	ErrBadEvent = errors.New("fundmat bad event")
	// ErrNegativeColumn 增量会使可用、冻结、在途或占用为负。事件不得标记为已应用。
	ErrNegativeColumn = errors.New("fundmat column would go negative")
	// ErrTransition 重建任务状态迁移非法。
	ErrTransition = errors.New("fundmat illegal job transition")
	// ErrJobActive 同一用户已有排队或执行中的重建。
	ErrJobActive = errors.New("fundmat rebuild already active")
	// ErrDrift 重建后投影与账本超过容差。
	ErrDrift = errors.New("fundmat projection drift")
	// ErrNoBooks 重建要求账本只读源，但未绑定。
	ErrNoBooks = errors.New("fundmat book source unavailable")
)
