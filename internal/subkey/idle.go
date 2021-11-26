// @FileName: idle.go
// @Summary: 子账户 API Key 空闲过期。IdleTTL>0 时自最近使用（无则创建）起超时则调用面拒绝。
// @Author: Kinsley
// @Created: 2021.11.24

package subkey

import (
	"errors"
	"time"
)

var ErrIdle = errors.New("api key idle timeout exceeded")

// IsIdle 空闲超时：IdleTTL 毫秒；0 关闭。

   func (r Record) IsIdle(nowMs int64) bool {
	   if r.IdleTTL <= 0 {
		  return false
	   }
	  if nowMs <= 0 {
		   nowMs = time.Now().UTC().UnixMilli()
	  }
	  base := r.LastUsedAt
	   if base <= 0 {
		   base = r.CreatedAt
	  }
	  if base <= 0 {
		  return false
	   }
	  return nowMs-base >= r.IdleTTL
   }
