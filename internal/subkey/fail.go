// 功能：子账户 API Key 口令错密锁定。连续错密达阈后进入冷却，防爆破试口令。
// 作者：Cason
// 日期：2021.11.25

package subkey

import (
	"errors"
	"strings"
	"time"
)

const (
	defaultMaxFails = 5
	defaultLockTTL  = 30 * time.Minute
)

var ErrLocked = errors.New("api key locked after passphrase failures")

// FailPolicy 错密锁定策略；零值用默认 5 次 / 30 分钟。
type FailPolicy struct {
	MaxFails int
	LockFor  time.Duration
}

   func (s *Store) BindFailPolicy(p FailPolicy) *Store {
	  if s == nil {
		   return s
	   }
	   if p.MaxFails > 0 {
		   s.maxFails = p.MaxFails
	  }
	  if p.LockFor > 0 {
		   s.lockTTL = p.LockFor
	   }
	   return s
   }

  func (s *Store) failPolicy() (int, time.Duration) {
	  max, ttl := defaultMaxFails, defaultLockTTL
	   if s != nil {
		  if s.maxFails > 0 {
			   max = s.maxFails
		  }
		   if s.lockTTL > 0 {
			   ttl = s.lockTTL
		   }
	  }
	   return max, ttl
   }

  func (r *row) isLocked(nowMs int64) bool {
	   if r == nil || r.LockedUntil <= 0 {
		   return false
	   }
	  if nowMs <= 0 {
		  nowMs = time.Now().UTC().UnixMilli()
	   }
	  return r.LockedUntil > nowMs
  }

// noteFailLocked 调用方须已持 s.mu。错密累计；达阈则上锁并清零计数。

  func (s *Store) noteFailLocked(r *row, nowMs int64) {
	   s.noteFailAt(r, nowMs, "")
   }

   func (s *Store) noteFailAt(r *row, nowMs int64, clientIP string) {
	   if r == nil {
		  return
	   }
	  max, ttl := s.failPolicy()
	  if r.LockedUntil > 0 && r.LockedUntil <= nowMs {
		   r.FailCount = 0
		   r.LockedUntil = 0
	   }
	   r.FailCount++
	   r.DenyCount++
	  r.LastFailAt = nowMs
	  if ip := strings.TrimSpace(clientIP); ip != "" {
		   if len(ip) > 64 {
			   ip = ip[:64]
		  }
		   r.LastFailIP = ip
	  }
	  r.UpdatedAt = nowMs
	  if r.FailCount >= max {
		  r.LockedUntil = nowMs + ttl.Milliseconds()
		  r.FailCount = 0
	  }
   }

   func (s *Store) clearFailLocked(r *row, nowMs int64) {
	  if r == nil {
		   return
	   }
	   if r.FailCount == 0 && r.LockedUntil == 0 {
		  return
	  }
	  r.FailCount = 0
	   r.LockedUntil = 0
	   r.UpdatedAt = nowMs
  }

// Unlock 母户解除错密锁；不需 passphrase（避免锁死后无法自救）。

   func (s *Store) Unlock(owner, subName, apiKey, actorIP string) (Record, error) {
	  if s == nil {
		   return Record{}, ErrGone
	   }
	   owner = strings.TrimSpace(owner)
	   subName = strings.TrimSpace(subName)
	   apiKey = strings.TrimSpace(apiKey)
	  if owner == "" {
		   return Record{}, ErrOwner
	  }
	  if subName == "" {
		  return Record{}, ErrSubName
	   }
	   if apiKey == "" {
		   return Record{}, ErrKey
	   }
	   if err := s.assertParent(owner, subName); err != nil {
		  return Record{}, err
	   }
	  now := time.Now().UTC().UnixMilli()
	   s.mu.Lock()
	   r, ok := s.by[apiKey]
	  if !ok || r.Owner != owner || r.SubName != subName {
		   s.mu.Unlock()
		  return Record{}, ErrGone
	   }
	   s.clearFailLocked(r, now)
	   out := r.Record
	   s.mu.Unlock()
	   s.persistRow(r)
	  s.noteAuditIP(owner, subName, apiKey, "unlock", "", actorIP)
	   return out, nil
   }
