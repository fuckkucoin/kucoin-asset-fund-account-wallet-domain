package sagaresidue

import (
	"fmt"
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

var (
	// ErrUnavailable 服务未初始化或关键依赖缺失。
	ErrUnavailable = fmt.Errorf("sagaresidue service unavailable")
	// ErrSagaRunNotFound 找不到指定的 safesaga Run。
	ErrSagaRunNotFound = fmt.Errorf("sagaresidue saga run not found")
	// ErrResidueRunNotFound 找不到 ResidueRun 记录。
	ErrResidueRunNotFound = fmt.Errorf("sagaresidue residue run not found")
	// ErrNoResidues 扫描后无残留，saga Run 已完全补偿。
	ErrNoResidues = fmt.Errorf("sagaresidue no residues detected")
	// ErrStaleGen 世代锁竞争失败：当前持有者世代号已过期。
	ErrStaleGen = fmt.Errorf("sagaresidue stale generation")
	// ErrGenLocked 世代锁已被另一 worker 持有。
	ErrGenLocked = fmt.Errorf("sagaresidue mutex gen locked by another holder")
	// ErrDualAuth 双人授权不满足（同一人 / 空值）。
	ErrDualAuth = fmt.Errorf("sagaresidue dual authorization required")
	// ErrAlreadyClosed Run 已 closed，幂等返回。
	ErrAlreadyClosed = fmt.Errorf("sagaresidue run already closed")
	// ErrProofMissing 收口腿已完成但证明未生成，禁止标 closed。
	ErrProofMissing = fmt.Errorf("sagaresidue close proof missing")
	// ErrProofInvalid 证明摘要校验失败。
	ErrProofInvalid = fmt.Errorf("sagaresidue close proof invalid")
	// ErrCompensateBlock Closeout 正在进行，补偿请求被互斥锁阻止。
	ErrCompensateBlock = fmt.Errorf("sagaresidue compensate blocked by active closeout")
	// ErrLegNotPending 收口腿不处于 pending 态，不可重复执行或跳过。
	ErrLegNotPending = fmt.Errorf("sagaresidue leg not pending")
	// ErrPlanConflict 同 Run 已存在活跃 CloseoutPlan。
	ErrPlanConflict = fmt.Errorf("sagaresidue closeout plan conflict")
	// ErrBadRunState Run 当前状态不允许该操作（eg. 对 closing 状态发起 Detect）。
	ErrBadRunState = fmt.Errorf("sagaresidue run state disallows operation")
	// ErrItemNotFound 找不到 ResidueItem。
	ErrItemNotFound = fmt.Errorf("sagaresidue residue item not found")
	// ErrLegNotFound 找不到 CloseoutLeg。
	ErrLegNotFound = fmt.Errorf("sagaresidue closeout leg not found")
	// ErrProofNotFound 找不到 CloseProof。
	ErrProofNotFound = fmt.Errorf("sagaresidue close proof not found")
	// ErrStorageNil 存储层未注入。
	ErrStorageNil = fmt.Errorf("sagaresidue storage is nil")
	// ErrMaxRetryExceeded 收口腿重试次数超上限，需人工处理。
	ErrMaxRetryExceeded = fmt.Errorf("sagaresidue leg max retry exceeded")
	// ErrIdemEmpty 幂等键为空。
	ErrIdemEmpty = fmt.Errorf("sagaresidue idempotency key empty")
	// ErrAmountInvalid 金额形态不合法。
	ErrAmountInvalid = fmt.Errorf("sagaresidue amount invalid")
)

   func init() {
	  apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	  apierr.Register(ErrSagaRunNotFound, http.StatusNotFound)
	  apierr.Register(ErrResidueRunNotFound, http.StatusNotFound)
	  apierr.Register(ErrNoResidues, http.StatusOK)
	  apierr.Register(ErrStaleGen, http.StatusConflict)
	   apierr.Register(ErrGenLocked, http.StatusConflict)
	   apierr.Register(ErrDualAuth, http.StatusBadRequest)
	  apierr.Register(ErrAlreadyClosed, http.StatusConflict)
	  apierr.Register(ErrProofMissing, http.StatusUnprocessableEntity)
	  apierr.Register(ErrProofInvalid, http.StatusUnprocessableEntity)
	  apierr.Register(ErrCompensateBlock, http.StatusConflict)
	  apierr.Register(ErrLegNotPending, http.StatusConflict)
	   apierr.Register(ErrPlanConflict, http.StatusConflict)
	  apierr.Register(ErrBadRunState, http.StatusConflict)
	  apierr.Register(ErrItemNotFound,
		  http.StatusNotFound)
	  apierr.Register(ErrLegNotFound, http.StatusNotFound)
	  apierr.Register(ErrProofNotFound, http.StatusNotFound)
	  apierr.Register(ErrStorageNil, http.StatusInternalServerError)
	   apierr.Register(ErrMaxRetryExceeded, http.StatusConflict)
	  apierr.Register(ErrIdemEmpty,
		   http.StatusBadRequest)
	   apierr.Register(ErrAmountInvalid, http.StatusBadRequest)
  }
