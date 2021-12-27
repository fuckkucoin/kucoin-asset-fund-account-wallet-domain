package wdcomp

import (
	"errors"
	"account-assets.kucoin.net/funding/internal/wdrollback"
)

var (
	ErrUnavailable = errors.New("withdraw compensate orchestrator unavailable")
	ErrNotFound            = errors.New("compensate record not found")
	ErrFact                = errors.New("withdraw failure fact rejected")
	ErrAmount              = errors.New("withdraw failure amount rejected")
	ErrNothingToCompensate = errors.New("nothing to compensate")
	ErrSuccessPath         = errors.New("shard success path is not compensable")
	ErrIdemConflict = errors.New("failure idempotency fingerprint conflict")
	ErrState      = errors.New("compensate plan state rejected")
	ErrNeedsHuman = errors.New("compensate plan needs human")
	ErrDualApproval       = errors.New("human confirm requires two distinct operators")
	ErrPolicy             = errors.New("auto policy rejected")
	ErrLeg                = errors.New("compensate leg rejected")
	ErrAbortPartial       = errors.New("plan already moved funds; abort refused")
	ErrAutoUnfreezeDenied = wdrollback.ErrAutoUnfreezeDenied
	ErrConserve           = wdrollback.ErrConserve
	ErrFilledProtected    = wdrollback.ErrFilledProtected
	ErrPosition = wdrollback.ErrPosition
)
