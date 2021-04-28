package destlimit

import (
	"errors"
	"fmt"
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/kyt"
)

const (
	CodeCluster = "DESTLIMIT_CLUSTER"
	CodeUser    = "DESTLIMIT_USER"
	CodeDest = "DESTLIMIT_DEST"
	CodeNoPolicy = "DESTLIMIT_NO_POLICY"
	CodeClosed      = "DESTLIMIT_CLOSED"
	CodeUnavailable = "DESTLIMIT_UNAVAILABLE"
	CodeKYTDeny     = "KYT_DENY"
	CodeLimitDeny = "DESTLIMIT_DENY"
)

var (
	ErrUnavailable = errors.New("destlimit unavailable")
	ErrLimitDeny   = errors.New("destlimit denied")
	ErrHoldState   = errors.New("destlimit hold state")
	ErrDual = errors.New("destlimit dual control")
	ErrOverride = errors.New("destlimit override")
	ErrNotFound = errors.New("destlimit not found")
	ErrPolicy  = errors.New("destlimit policy")
	ErrExpired = errors.New("destlimit expired")
)

   func init() {
	  apierr.Register(ErrLimitDeny, http.StatusForbidden)
	  apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	  apierr.Register(ErrHoldState, http.StatusConflict)
	  apierr.Register(ErrDual, http.StatusForbidden)
	   apierr.Register(ErrOverride, http.StatusBadRequest)
	  apierr.Register(ErrNotFound, http.StatusNotFound)
	   apierr.Register(ErrPolicy, http.StatusBadRequest)
	  apierr.Register(ErrExpired, http.StatusConflict)
   }

// DenyError 限额拒绝。Unwrap 只指向 ErrLimitDeny，不指向 kyt.ErrDenied。
type DenyError struct {
	Code string `json:"code"`
	Reason    string `json:"reason"`
	Limit     string `json:"limit,omitempty"`
	Used      string `json:"used,omitempty"`
	ClusterID string `json:"cluster_id,omitempty"`
}

   func (e *DenyError) Error() string {
	   if e == nil {
		   return ErrLimitDeny.Error()
	   }
	   return fmt.Sprintf("%s: %s: %s", ErrLimitDeny.Error(), e.Code, e.Reason)
  }

  func (e *DenyError) Unwrap() error { return ErrLimitDeny }

// Classify 把错误分成 KYT 拒绝或限额拒绝。两者都不匹配时返回空串。
// 限额错误即使携带 KYT 特征上下文，也不得被分成 KYT。
  func Classify(err error) string {
	   if err == nil {
		   return ""
	   }
	   if errors.Is(err, ErrLimitDeny) {
		   return CodeLimitDeny
	  }
	  if errors.Is(err, kyt.ErrDenied) {
		  return CodeKYTDeny
	   }
	  return ""
   }
