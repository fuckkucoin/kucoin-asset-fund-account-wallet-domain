// 划转半截收口错误定义。

// Author: Dax
// Date: 2021.12.30

package xferclose

import "errors"

var (
	ErrNotFound = errors.New("xferclose: record not found")
	ErrConflict = errors.New("xferclose: conflicting record")
	ErrStaleFence      = errors.New("xferclose: stale fence")
	ErrAlreadyTerminal = errors.New("xferclose: hop already in terminal state")
	ErrPlanRunning     = errors.New("xferclose: closeout plan already running")
	ErrPlanDone        = errors.New("xferclose: closeout plan already done")
	ErrLegConflict     = errors.New("xferclose: closeout leg kind conflict")
	ErrNoSLA      = errors.New("xferclose: no SLA rule matches")
	ErrZeroAmount = errors.New("xferclose: amount must be positive")
	ErrBadHopID   = errors.New("xferclose: hop_id required")
	ErrIdempotent = errors.New("xferclose: duplicate closeout prevented (idempotent)")
	ErrActorRequired = errors.New("xferclose: actor identity required for manual ops")
	ErrXShardHop = errors.New("xferclose: cross-shard hop not owned by this package")
)
