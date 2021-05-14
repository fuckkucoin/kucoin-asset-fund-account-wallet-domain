package fiatbreak

import "fmt"

var (
	// ErrUnavailable 服务或依赖未就绪。
	ErrUnavailable = fmt.Errorf("fiatbreak unavailable")
	// ErrNotFound 指定记录不存在。
	ErrNotFound = fmt.Errorf("fiatbreak record not found")
	// ErrBreakState 断裂状态非法转换。
	ErrBreakState = fmt.Errorf("fiatbreak illegal break state")
	// ErrSuspenseState 挂账状态非法转换。
	ErrSuspenseState = fmt.Errorf("fiatbreak illegal suspense state")
	// ErrAmount 金额形态或精度非法。
	ErrAmount = fmt.Errorf("fiatbreak amount invalid")
	// ErrDuplicate 重复 PSP 收据（幂等键或收据 ID 冲突）。
	ErrDuplicate = fmt.Errorf("fiatbreak duplicate receipt")
	// ErrIdem 幂等键冲突但内容不符。
	ErrIdem = fmt.Errorf("fiatbreak idempotency conflict")
	// ErrBooking 轨道预约不存在或已终态。
	ErrBooking = fmt.Errorf("fiatbreak booking invalid")
	// ErrBreakKind 断裂类型未知。
	ErrBreakKind = fmt.Errorf("fiatbreak kind unknown")
	// ErrPolicy SLA 策略配置非法。
	ErrPolicy = fmt.Errorf("fiatbreak policy invalid")
	// ErrProof 双边证明生成失败。
	ErrProof = fmt.Errorf("fiatbreak bilateral proof error")
	// ErrDualControl 强制冲销需要双人授权。
	ErrDualControl = fmt.Errorf("fiatbreak dual control required")
	// ErrActor 操作人 ID 非法。
	ErrActor = fmt.Errorf("fiatbreak actor invalid")
	// ErrReason 原因说明非法或缺失。
	ErrReason = fmt.Errorf("fiatbreak reason required")
	// ErrNoBreak 指定预约无断裂记录。
	ErrNoBreak = fmt.Errorf("fiatbreak no break for booking")
)
