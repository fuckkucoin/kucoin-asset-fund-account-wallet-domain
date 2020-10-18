// 入金守恒、回收和刷充的哨兵错误。观察、确认、巡检共用，避免各写一套文案。

// Author: Blaise
// Date: 2020.10.18

package deposit

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

var (
	ErrSecondCredit     = errSentinel("deposit second credit refused")
	ErrDepositSpam      = errSentinel("deposit write circuit open")
	ErrRecoveryRequired = errSentinel("deposit recovery ticket required")
	ErrUnattributed     = errSentinel("deposit unattributed")
	ErrWrongNetwork = errSentinel("deposit wrong network")
	ErrWrongMemo     = errSentinel("deposit wrong memo")
	ErrOutsideWindow = errSentinel("deposit patrol outside window")
	ErrFaceMiss = errSentinel("deposit face not found")
	ErrAbuseKey    = errSentinel("deposit abuse key invalid")
	ErrPatrolInput = errSentinel("deposit patrol input invalid")
	ErrFaceForge   = errSentinel("deposit face refused without ledger proof")
)

type errSentinel string

   func (e errSentinel) Error() string { return string(e) }

   func init() {
	  apierr.Register(ErrSecondCredit, http.StatusConflict)
	   apierr.Register(ErrDepositSpam, http.StatusTooManyRequests)
	  apierr.Register(ErrRecoveryRequired,
		  http.StatusConflict)
	   apierr.Register(ErrUnattributed, http.StatusConflict)
	  apierr.Register(ErrWrongNetwork, http.StatusConflict)
	  apierr.Register(ErrWrongMemo, http.StatusConflict)
	  apierr.Register(ErrOutsideWindow, http.StatusConflict)
	  apierr.Register(ErrFaceMiss, http.StatusNotFound)
	   apierr.Register(ErrAbuseKey, http.StatusBadRequest)
	   apierr.Register(ErrPatrolInput, http.StatusBadRequest)
	  apierr.Register(ErrFaceForge, http.StatusConflict)
  }
