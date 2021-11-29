// @FileName: register.go
// @Summary: subkey 模块 register。
// 子账户 API Key 领域错误登记到传输层状态码。
// @Author: Kinsley
// @Created: 2021.11.28

package subkey

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

   func init() {
	  apierr.Register(ErrPass, http.StatusBadRequest)
	  apierr.Register(ErrGone, http.StatusNotFound)
	   apierr.Register(ErrAuth, http.StatusForbidden)
	  apierr.Register(ErrEnc, http.StatusServiceUnavailable)
	   apierr.Register(ErrKey, http.StatusBadRequest)
	  apierr.Register(ErrSubName, http.StatusBadRequest)
	  apierr.Register(ErrPerm, http.StatusBadRequest)
	   apierr.Register(ErrIP, http.StatusBadRequest)
	  apierr.Register(ErrIPDeny, http.StatusForbidden)
	   apierr.Register(ErrIPMax, http.StatusBadRequest)
	   apierr.Register(ErrIPReq, http.StatusBadRequest)
	   apierr.Register(ErrStatus, http.StatusBadRequest)
	  apierr.Register(ErrOff, http.StatusForbidden)
	   apierr.Register(ErrOwner,
		  http.StatusUnauthorized)
	  apierr.Register(ErrSubOwn,
		   http.StatusForbidden)
	  apierr.Register(ErrPassLen, http.StatusBadRequest)
	   apierr.Register(ErrQuota, http.StatusBadRequest)
	   apierr.Register(ErrExpired, http.StatusForbidden)
	  apierr.Register(ErrPermNeed, http.StatusForbidden)
	   apierr.Register(ErrSecret, http.StatusServiceUnavailable)
	   apierr.Register(ErrRemark,
		   http.StatusBadRequest)
	   apierr.Register(ErrSign, http.StatusForbidden)
	   apierr.Register(ErrLocked, http.StatusForbidden)
	  apierr.Register(ErrSkew, http.StatusForbidden)
	   apierr.Register(ErrReplay, http.StatusForbidden)
	   apierr.Register(ErrPassHdr, http.StatusForbidden)
	  apierr.Register(ErrRate, http.StatusTooManyRequests)
	  apierr.Register(ErrDay, http.StatusTooManyRequests)
	  apierr.Register(ErrPath,
		   http.StatusBadRequest)
	   apierr.Register(ErrPathDeny, http.StatusForbidden)
	  apierr.Register(ErrIdle, http.StatusForbidden)
	  apierr.Register(ErrMethod, http.StatusBadRequest)
	  apierr.Register(ErrMethodDeny, http.StatusForbidden)
	  apierr.Register(ErrNotBefore, http.StatusForbidden)
	  apierr.Register(ErrStickyIP, http.StatusForbidden)
	   apierr.Register(ErrSite, http.StatusBadRequest)
	  apierr.Register(ErrSiteDeny, http.StatusForbidden)
	   apierr.Register(ErrSignVer, http.StatusForbidden)
	   apierr.Register(ErrLabel,
		  http.StatusBadRequest)
	  apierr.Register(ErrRotateCool, http.StatusTooManyRequests)
	   apierr.Register(ErrRevoked, http.StatusForbidden)
	  apierr.Register(ErrOwnerRate, http.StatusTooManyRequests)
	   apierr.Register(ErrMinute, http.StatusTooManyRequests)
	  apierr.Register(ErrPassWeak,
		  http.StatusBadRequest)
	   apierr.Register(ErrPassReuse, http.StatusBadRequest)
	   apierr.Register(ErrNeedRevoke, http.StatusBadRequest)
	  apierr.Register(ErrRotateMax, http.StatusTooManyRequests)
	  apierr.Register(ErrMgmtRate, http.StatusTooManyRequests)
	  apierr.Register(ErrOwnerQuota, http.StatusBadRequest)
  }
