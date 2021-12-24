// 功能：subkey 模块 mgmtrate。
// 子账户 API Key 管理面限流。单母户滑动分钟窗限制创建/轮换/吊销等写操作。
// 默认 20 次/分，防母户会话被盗后滥发或轮换风暴。

// 作者：TianpengLan
// 日期：2021.11.25

package subkey

import (
	"errors"
	"strings"
	"time"
)

const defaultMgmtMax = 20

var ErrMgmtRate = errors.New("sub api key management rate exceeded")

// BindMgmtRate 注入管理面分钟上限；<0 关闭，0 用默认。

   func (s *Store) BindMgmtRate(max int) *Store {
	  if s == nil {
		   return s
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   s.mgmtMax = max
	  if s.mgmtHits == nil {
		  s.mgmtHits = map[string][]int64{}
	   }
	   return s
  }

   func (s *Store) mgmtRateMax() int {
	   if s != nil && s.mgmtMax < 0 {
		   return 0
	  }
	  if s != nil && s.mgmtMax > 0 {
		  return s.mgmtMax
	   }
	  return defaultMgmtMax
  }

// allowMgmt 管理面写操作过闸；独立取锁。

  func (s *Store) allowMgmt(owner string) error {
	   if s == nil {
		  return ErrGone
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   return s.allowMgmtLocked(owner, time.Now().UTC())
  }

  func (s *Store) allowMgmtLocked(owner string, now time.Time) error {
	   max := s.mgmtRateMax()
	  if max <= 0 {
		  return nil
	   }
	  owner = strings.TrimSpace(owner)
	   if owner == "" {
		   return ErrOwner
	  }
	   if s.mgmtHits == nil {
		   s.mgmtHits = map[string][]int64{}
	   }
	   if now.IsZero() {
		  now = time.Now().UTC()
	   }
	  cutoff := now.Add(-time.Minute).UnixNano()
	  arr := s.mgmtHits[owner]
	  kept := arr[:0]
	   for _, t := range arr {
		   if t > cutoff {
			   kept = append(kept, t)
		   }
	   }
	   if len(kept) >= max {
		  if len(kept) == 0 {
			   delete(s.mgmtHits, owner)
		  } else {
			   s.mgmtHits[owner] = kept
		   }
		   return ErrMgmtRate
	  }
	  s.mgmtHits[owner] = append(kept, now.UnixNano())
	   return nil
   }
