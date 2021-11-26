// 子账户 API Key 轮换宽限。轮换后旧 secret 在 GraceTTL 内仍可验签，降低切换窗口断服。
// 宽限期内双密有效；到期后 PrevSecretEnc 作废。

// Author: Axel
// Date: 2021.11.24

package subkey

import (
	"time"
)

const defaultGraceTTL = 10 * time.Minute

	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  s.graceTTL = d
	  return s
   }

  func (s *Store) graceDuration() time.Duration {
	  if s != nil && s.graceTTL > 0 {
		  return s.graceTTL
	   }
	  return defaultGraceTTL
  }

  func (r *row) withinGrace(nowMs int64) bool {
	  if r == nil || r.GraceUntil <= 0 || len(r.PrevSecretEnc) == 0 {
		  return false
	   }
	  if nowMs <= 0 {
		  nowMs = time.Now().UTC().UnixMilli()
	   }
	   return nowMs < r.GraceUntil
  }
