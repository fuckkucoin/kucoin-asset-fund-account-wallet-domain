package fundread

import "errors"

var (
	// ErrNotCaughtUp 投影连续水位低于读围栏或日志头。
	ErrNotCaughtUp = errors.New("投影未追平")
	// ErrFenceExpired 围栏已过期，不能再作为强一致依据。
	ErrFenceExpired = errors.New("读围栏已过期")
	// ErrUser 缺少用户标识。
	ErrUser = errors.New("user_id required")
)
