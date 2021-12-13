// @FileName: rotatemax.go
// @Summary: 子账户 API Key 轮换次数上限。默认终身 50 次，抑制冷却绕过的长期轮换风暴。
// @Author: Kinsley
// @Created: 2021.11.29

package subkey

import "errors"

const defaultMaxRotate = 50

var ErrRotateMax = errors.New("api key rotate count exceeded")

// BindMaxRotate 注入终身轮换上限；<=0 用默认 50。

   func (s *Store) BindMaxRotate(n int) *Store {
	   if s == nil {
		  return s
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   s.maxRotate = n
	  return s
  }

  func (s *Store) maxRotateN() int {
	   if s != nil && s.maxRotate > 0 {
		   return s.maxRotate
	  }
	   return defaultMaxRotate
  }

   func (s *Store) assertRotateMax(count int) error {
	   if count >= s.maxRotateN() {
		   return ErrRotateMax
	   }
	   return nil
   }
