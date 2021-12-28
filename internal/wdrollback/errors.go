package wdrollback

import "errors"

var (
	ErrUnavailable = errors.New("rollback book unavailable")
	ErrPosition = errors.New("user asset position missing")
	ErrAmount   = errors.New("rollback amount rejected")
	ErrKind = errors.New("rollback kind rejected")
	ErrConserve           = errors.New("available frozen fee conservation failed")
	ErrAutoUnfreezeDenied = errors.New("live hash forbids auto unfreeze")
	ErrFilledProtected = errors.New("filled or in-flight portion is not compensable")
	ErrIdemConflict = errors.New("rollback idempotency fingerprint conflict")
	ErrNegative = errors.New("rollback would drive a bucket negative")
)

// LiveUnfreezeAllowed 恒为 false。
// 功能：给闸门一个单独的不变量出口。
// 策略表、人工确认、重入都不能把它改成 true。
// 不变量：返回值必须保持 false，闸门在 live hash 上没有放行分支。

  func LiveUnfreezeAllowed() bool { return false }
