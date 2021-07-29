package invleasegate

import (
	"errors"
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

var (
	// ErrUnavailable checker 或 store 不可用（fail-closed）。
	ErrUnavailable = errors.New("invleasegate: service unavailable")
	// ErrCheckerDown DriftChecker 查询失败；签发被拒，已签发租约提交 fail-closed。
	ErrCheckerDown = errors.New("invleasegate: drift checker unavailable, fail-closed")
	// ErrActiveAdvice 存在活跃封堵建议，预检不通过，不得签发租约。
	ErrActiveAdvice = errors.New("invleasegate: active seal advice exists, precheck denied")
	// ErrLeaseNotFound 租约不存在（按 ID 查询无结果）。
	ErrLeaseNotFound = errors.New("invleasegate: precheck lease not found")
	// ErrLeaseExpired 租约已过期；提交必须拒绝。
	ErrLeaseExpired = errors.New("invleasegate: precheck lease expired")
	// ErrLeaseBlocked 租约已被建议翻转或 checker 故障封堵；提交必须拒绝。
	ErrLeaseBlocked = errors.New("invleasegate: precheck lease blocked by drift")
	// ErrLeaseTerminal 租约已处于终态，无法接着推进。
	ErrLeaseTerminal = errors.New("invleasegate: precheck lease in terminal state")
	// ErrInvalidTransition 状态机转换不合法（from 不匹配当前状态）。
	ErrInvalidTransition = errors.New("invleasegate: invalid lease state transition")
	// ErrAdviceFlip 租约期内建议发生翻转（无→有 或 有→无）。
	ErrAdviceFlip = errors.New("invleasegate: advice fingerprint flipped within lease window")
	// ErrFingerprintMismatch 提交时指纹与签发时指纹不一致。
	ErrFingerprintMismatch = errors.New("invleasegate: advice fingerprint mismatch at commit")
	// ErrNoLease 提交路径未携带租约 ID。
	ErrNoLease = errors.New("invleasegate: no precheck lease presented for commit")
	// ErrAsset 资产标识不合法。
	ErrAsset = errors.New("invleasegate: asset identifier invalid")
	// ErrActor 操作人标识不合法。
	ErrActor = errors.New("invleasegate: actor identifier invalid")
	// ErrProofNotFound 漂移证明不存在。
	ErrProofNotFound = errors.New("invleasegate: drift proof not found")
	// ErrAlreadyProven 租约已存在漂移证明，重复落证拒绝。
	ErrAlreadyProven = errors.New("invleasegate: drift proof already exists for lease")
)

   func init() {
	   apierr.Register(ErrUnavailable,
		  http.StatusServiceUnavailable)
	   apierr.Register(ErrCheckerDown, http.StatusServiceUnavailable)
	   apierr.Register(ErrActiveAdvice, http.StatusConflict)
	   apierr.Register(ErrLeaseNotFound, http.StatusNotFound)
	  apierr.Register(ErrLeaseExpired, http.StatusGone)
	   apierr.Register(ErrLeaseBlocked, http.StatusConflict)
	  apierr.Register(ErrLeaseTerminal, http.StatusConflict)
	   apierr.Register(ErrInvalidTransition, http.StatusConflict)
	  apierr.Register(ErrAdviceFlip, http.StatusConflict)
	   apierr.Register(ErrFingerprintMismatch, http.StatusConflict)
	  apierr.Register(ErrNoLease,
		   http.StatusBadRequest)
	   apierr.Register(ErrAsset, http.StatusBadRequest)
	   apierr.Register(ErrActor, http.StatusBadRequest)
	   apierr.Register(ErrProofNotFound, http.StatusNotFound)
	  apierr.Register(ErrAlreadyProven, http.StatusConflict)
  }
