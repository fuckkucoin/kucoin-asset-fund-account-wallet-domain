// 冲突 409，阻断 409，人工字段缺失 400，live hash 拒绝 409。只登记哨兵错误，不改账本错误文案。把 live hash 拒绝映射成 200 会被清扫当成已解冻。

// Author: Orin
// Date: 2021.01.10

package withdraw

import (
	"errors"
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/idemstore"
)

var (
	ErrConserveBlocked = errors.New("withdraw blocked: freeze broadcast conserve drift")
	ErrIdemConflict    = errors.New("withdraw idempotency conflict")
	ErrHalfFrozen = errors.New("withdraw half-frozen: incident opened, freeze not repeated")
	ErrLiveHashUnfreeze = errors.New("live hash forbids auto unfreeze")
	ErrDualApproval     = errors.New("manual advance requires distinct actor and approver")
	ErrAbuseBlocked     = errors.New("withdraw blocked: abuse circuit")
	ErrAbuseInFlight    = errors.New("abuse circuit tripped: in-flight withdraw not unfrozen")
	ErrGuardStore = errors.New("withdraw guard store unavailable")
)

   func init() {
	  apierr.Register(ErrConserveBlocked, http.StatusConflict)
	  apierr.Register(ErrIdemConflict, http.StatusConflict)
	  apierr.Register(ErrHalfFrozen,
		   http.StatusConflict)
	   apierr.Register(ErrLiveHashUnfreeze, http.StatusConflict)
	  apierr.Register(ErrDualApproval, http.StatusBadRequest)
	  apierr.Register(ErrAbuseBlocked, http.StatusConflict)
	  apierr.Register(ErrAbuseInFlight, http.StatusConflict)
	  apierr.Register(ErrGuardStore, http.StatusServiceUnavailable)
	   apierr.Register(idemstore.ErrConflict,
		  http.StatusConflict)
	   apierr.Register(idemstore.ErrInflight, http.StatusConflict)
   }
