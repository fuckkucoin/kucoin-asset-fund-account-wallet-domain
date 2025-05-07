package stucksettle

import (
	"fmt"
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

var (
	// ErrLeaseConflict 同 transfer_id 已有活跃 Acting 租约，新申请被拒。
	ErrLeaseConflict = fmt.Errorf("stucksettle: recover lease conflict")
	// ErrDupBlocked 重复回收意图，资金动作已被封堵。
	ErrDupBlocked = fmt.Errorf("stucksettle: duplicate recover blocked")
	// ErrGenMismatch 世代号不匹配；旧世代请求拒绝。
	ErrGenMismatch = fmt.Errorf("stucksettle: generation mismatch")
	// ErrAdvanceLocked AdvanceStuck 持锁中，不允许旁路 Recover。
	ErrAdvanceLocked = fmt.Errorf("stucksettle: advance mutex locked")
	// ErrHoldBypass Hold 态禁止旁路直接 Recover。
	ErrHoldBypass = fmt.Errorf("stucksettle: hold state bypass not allowed")
	// ErrProofMissing 效果证明缺失，不得标记 Recovered。
	ErrProofMissing = fmt.Errorf("stucksettle: effect proof missing")
	// ErrProofInvalid 效果证明 checksum 校验失败。
	ErrProofInvalid = fmt.Errorf("stucksettle: effect proof invalid checksum")
	// ErrLeaseExpired 租约已超时失效。
	ErrLeaseExpired = fmt.Errorf("stucksettle: recover lease expired")
	// ErrLeaseTerminal 租约已处于终态，不可再变更。
	ErrLeaseTerminal = fmt.Errorf("stucksettle: recover lease already terminal")
	// ErrCaseNotFound 双结案件不存在。
	ErrCaseNotFound = fmt.Errorf("stucksettle: dup recover case not found")
	// ErrCaseAlreadyClosed 双结案件已关闭。
	ErrCaseAlreadyClosed = fmt.Errorf("stucksettle: dup recover case already closed")
	// ErrInvalidActor 操作人标识无效。
	ErrInvalidActor = fmt.Errorf("stucksettle: invalid actor")
	// ErrUnavailable 服务不可用（内部依赖未就绪）。
	ErrUnavailable = fmt.Errorf("stucksettle: service unavailable")
	// ErrInvalidInput 请求参数无效。
	ErrInvalidInput = fmt.Errorf("stucksettle: invalid input")
)

   func init() {
	  apierr.Register(ErrLeaseConflict, http.StatusConflict)
	  apierr.Register(ErrDupBlocked, http.StatusConflict)
	   apierr.Register(ErrGenMismatch,
		   http.StatusConflict)
	   apierr.Register(ErrAdvanceLocked, http.StatusConflict)
	   apierr.Register(ErrHoldBypass,
		   http.StatusForbidden)
	   apierr.Register(ErrProofMissing, http.StatusUnprocessableEntity)
	   apierr.Register(ErrProofInvalid, http.StatusUnprocessableEntity)
	   apierr.Register(ErrLeaseExpired, http.StatusGone)
	   apierr.Register(ErrLeaseTerminal, http.StatusConflict)
	   apierr.Register(ErrCaseNotFound, http.StatusNotFound)
	  apierr.Register(ErrCaseAlreadyClosed, http.StatusConflict)
	   apierr.Register(ErrInvalidActor, http.StatusBadRequest)
	   apierr.Register(ErrUnavailable,
		  http.StatusServiceUnavailable)
	   apierr.Register(ErrInvalidInput, http.StatusBadRequest)
  }
