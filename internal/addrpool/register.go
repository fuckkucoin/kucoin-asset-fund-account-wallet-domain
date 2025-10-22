package addrpool

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

   func init() {
	   // 种子 Ensure 拒绝 → 503（须运营 Register 后再领址）。
	   apierr.Register(ErrSeedEnsureRefused,
		   http.StatusServiceUnavailable)
  }
