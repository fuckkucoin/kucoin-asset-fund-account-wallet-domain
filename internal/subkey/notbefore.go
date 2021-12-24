package subkey

import (
	"errors"
	"time"
)

var ErrNotBefore = errors.New("api key not yet active")

// NormalizeNotBefore 负值清零；0 立即；正值原样保留（可早于或晚于 now）。
   func NormalizeNotBefore(notBefore int64) int64 {
	  if notBefore < 0 {
		   return 0
	  }
	  return notBefore
   }

// IsNotBefore 尚未生效：NotBefore>0 且 now < NotBefore。
  func (r Record) IsNotBefore(nowMs int64) bool {
	  if r.NotBefore <= 0 {
		  return false
	  }
	   if nowMs <= 0 {
		  nowMs = time.Now().UTC().UnixMilli()
	  }
	  return nowMs < r.NotBefore
  }
