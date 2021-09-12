// opsauth 模块 register。

// Author: Axel
// Date: 2021.09.11

package opsauth

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

  func init() {
	   apierr.Register(ErrSig, http.StatusForbidden)
	  apierr.Register(ErrNonce, http.StatusBadRequest)
	   apierr.Register(ErrReplay, http.StatusConflict)
  }
