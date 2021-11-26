// 子账户 API Key 过期。expireAt=0 永不过期；过期后调用面拒绝，列表仍可见供母户回收。

// Author: Axel
// Date: 2021.11.25

package subkey

import (
	"errors"
	"time"
)

var ErrExpired = errors.New("api key expired")

// NormalizeExpire 负值清零（永不过期）；正值须晚于当前毫秒。

   func NormalizeExpire(expireAt int64, nowMs int64) (int64, error) {
	  if expireAt < 0 {
		   return 0, nil
	   }
	  if expireAt == 0 {
		  return 0, nil
	   }
	  if nowMs <= 0 {
		   nowMs = time.Now().UTC().UnixMilli()
	   }
	   if expireAt <= nowMs {
		  return 0, ErrExpired
	   }
	   return expireAt, nil
   }

   func (r Record) IsExpired(nowMs int64) bool {
	  if r.ExpireAt <= 0 {
		   return false
	  }
	  if nowMs <= 0 {
		  nowMs = time.Now().UTC().UnixMilli()
	  }
	   return nowMs >= r.ExpireAt
  }
