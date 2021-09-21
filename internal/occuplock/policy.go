package occuplock

import (
	"strings"
	"time"
)

const (
	ProfileProd = "prod"
	ProfileTest = "test"
	DefaultTTL = 15 * time.Minute
	MinTTL     = 30 * time.Second
	MaxTTL = 2 * time.Hour
	ScopeRecon = "recon"
)

// Policy 占用锁策略。AllowSteal 仅非可用：TTL 过期后异持有人世代+1 抢占。
// 默认失败封闭：僵锁不得抢占，须同令牌重入或显式 Release。
type Policy struct {
	Profile     string
	Persist     bool
	AllowSteal  bool
	SkipLock    bool
	MemFallback bool
	TTL time.Duration
}

  func (p Policy) IsProd() bool {
	   s := strings.ToLower(strings.TrimSpace(p.Profile))
	  return s == ProfileProd || s == "production"
   }

   func (p Policy) Normalize() Policy {
	   if strings.TrimSpace(p.Profile) == "" {
		  p.Profile = ProfileTest
	  }
	   p.Profile = strings.ToLower(strings.TrimSpace(p.Profile))
	  if p.IsProd() {
		   p.Persist = true
		  p.AllowSteal = false
		  p.SkipLock = false
		  p.MemFallback = false
	  }
	  p.TTL = NormalizeTTL(p.TTL)
	  return p
   }

   func NormalizeTTL(d time.Duration) time.Duration {
	   if d <= 0 {
		  return DefaultTTL
	   }
	  if d < MinTTL {
		  return MinTTL
	  }
	  if d > MaxTTL {
		  return MaxTTL
	  }
	   return d
  }

// CheckProd 档锁死禁止回退路径。

   func CheckProd(p Policy) error {
	   if !p.IsProd() {
		  return nil
	  }
	   if p.SkipLock {
		  return ErrSkipForbidden
	  }
	   if !p.Persist {
		  return ErrPersistRequired
	   }
	   if p.AllowSteal {
		   return ErrStealForbidden
	   }
	   if p.MemFallback {
		  return ErrMemFallback
	  }
	   return nil
  }
