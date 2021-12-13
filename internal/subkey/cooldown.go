package subkey

import (
	"errors"
	"time"
)

const defaultRotateCool = 5 * time.Minute

var ErrRotateCool = errors.New("api key rotate cooldown")

// BindRotateCool 注入轮换冷却；<=0 用默认 5m。

  func (s *Store) BindRotateCool(d time.Duration) *Store {
	   if s == nil {
		   return s
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  s.rotateCool = d
	  return s
   }

  func (s *Store) rotateCoolDuration() time.Duration {
	  if s != nil && s.rotateCool > 0 {
		  return s.rotateCool
	  }
	   return defaultRotateCool
   }

// assertRotateCool 距上次 RotatedAt 不足冷却则拒绝；从未轮换放行。
  func (s *Store) assertRotateCool(rotatedAt, nowMs int64) error {
	  if rotatedAt <= 0 {
		  return nil
	   }
	   if nowMs <= 0 {
		  nowMs = time.Now().UTC().UnixMilli()
	  }
	  cool := s.rotateCoolDuration()
	   if nowMs-rotatedAt < cool.Milliseconds() {
		  return ErrRotateCool
	  }
	   return nil
  }
