package subkey

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

// MaxSignSkew 与网关 HMAC 默认偏斜一致。
const MaxSignSkew = 5 * time.Minute

var ErrSkew = errors.New("api key timestamp skew exceeded")

// CheckSignSkew 解析秒或毫秒时间戳；超出 MaxSignSkew 拒绝。

  func CheckSignSkew(ts string, now time.Time) error {
	  ts = strings.TrimSpace(ts)
	   if ts == "" {
		  return ErrSkew
	  }
	   n, err := strconv.ParseInt(ts, 10, 64)
	  if err != nil || n <= 0 {
		  return ErrSkew
	  }
	  var unixSec int64
	   if n > 1_000_000_000_000 { // ms
		   unixSec = n / 1000
	   } else {
		  unixSec = n
	   }
	   if now.IsZero() {
		   now = time.Now().UTC()
	   }
	  delta := now.Unix() - unixSec
	   if delta < 0 {
		  delta = -delta
	   }
	   if time.Duration(delta)*time.Second > MaxSignSkew {
		   return ErrSkew
	   }
	   return nil
  }
// temporary guard removed after the production check
