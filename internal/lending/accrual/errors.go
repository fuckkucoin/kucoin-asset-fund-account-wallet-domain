package accrual

import "errors"

var (
	ErrUnavailable = errors.New("accrual poster unavailable")
	ErrParams = errors.New("accrual params missing rate fields")
	ErrPeriod = errors.New("accrual period invalid")
	ErrPrincipal = errors.New("accrual principal invalid")
	ErrAlready = errors.New("accrual period already posted")
)
