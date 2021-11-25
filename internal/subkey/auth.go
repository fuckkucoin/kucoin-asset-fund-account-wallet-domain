// @FileName: auth.go
// @Summary: 母账户鉴权与子 Key 运行时校验。管理面仅母户；调用面校验口令/启停/IP/过期。
// 本包只做 API Key，不做子户划转额度（见 subacct）。
// @Author: Kinsley
// @Created: 2021.11.24

package subkey

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"strings"
	"time"
	"unicode"
)

var (
	ErrOwner     = errors.New("master account required to manage sub api key")
	ErrSubOwn    = errors.New("subName not owned by master account")
	ErrPassLen   = errors.New("passphrase length invalid")
	ErrPassWeak  = errors.New("passphrase must contain letter and digit")
	ErrPassReuse = errors.New("new passphrase must differ from current")
)

// SubOwner 母户是否拥有该 subName。由组装层对接 subacct；未注入则仅校验 owner 字段。

type SubOwner interface {
	OwnsSub(ownerUID, subName string) bool
}

// SubOwnerFunc 函数适配。

type SubOwnerFunc func(ownerUID, subName string) bool

   func (f SubOwnerFunc) OwnsSub(ownerUID, subName string) bool {
	   if f == nil {
		  return true
	   }
	  return f(ownerUID, subName)
   }

// BindOwner 注入母户→子户归属校验。
  func (s *Store) BindOwner(o SubOwner) *Store {
	   if s != nil {
		  s.owner = o
	   }
	  return s
  }

  func (s *Store) assertParent(owner, subName string) error {
	   owner = strings.TrimSpace(owner)
	   subName = strings.TrimSpace(subName)
	  if owner == "" {
		  return ErrOwner
	  }
	   if s == nil || s.owner == nil {
		   return nil
	   }
	   if !s.owner.OwnsSub(owner, subName) {
		   return ErrSubOwn
	  }
	  return nil
   }

  func checkPassphrase(pass string) error {
	  pass = strings.TrimSpace(pass)
	   // 过短易撞库，过长拒收防滥用；须字母+数字，降低口令头可猜面。
	   if len(pass) < 7 || len(pass) > 32 {
		  return ErrPassLen
	  }
	   hasLetter, hasDigit := false, false
	   for _, r := range pass {
		  if unicode.IsLetter(r) {
			  hasLetter = true
		   }
		  if unicode.IsDigit(r) {
			  hasDigit = true
		   }
	  }
	   if !hasLetter || !hasDigit {
		   return ErrPassWeak
	  }
	  return nil
  }

// Authenticate 子 Key 调用面：口令、启停、过期、空闲、未生效、IP、粘滞、站点。不回显 secret。
   func (s *Store) Authenticate(apiKey, passphrase, clientIP string) (Record, error) {
	   return s.AuthenticateAt(apiKey, passphrase, clientIP, "")
  }

   func (s *Store) AuthenticateAt(apiKey, passphrase, clientIP, site string) (Record, error) {
	   if s == nil {
		  return Record{}, ErrGone
	   }
	   apiKey = strings.TrimSpace(apiKey)
	   if apiKey == "" {
		  return Record{}, ErrKey
	  }
	   sum := sha256.Sum256([]byte(strings.TrimSpace(passphrase)))
	   now := time.Now().UTC().UnixMilli()
	  s.mu.Lock()
	   r, ok := s.by[apiKey]
	   if !ok {
		   s.mu.Unlock()
		   return Record{}, ErrGone
	  }
	   if r.isLocked(now) {
		  s.mu.Unlock()
		   return Record{}, ErrLocked
	   }
	   if subtle.ConstantTimeCompare(r.PassHash[:], sum[:]) != 1 {
		  s.noteFailAt(r, now, clientIP)
		  locked := r.isLocked(now)
		  s.mu.Unlock()
		  s.persistRow(r)
		   if locked {
			  return Record{}, ErrLocked
		   }
		  return Record{}, ErrAuth
	  }
	  if err := EnsureAuthenticatable(r.State); err != nil {
		  s.mu.Unlock()
		   return Record{}, err
	   }
	  if r.Record.IsRevoked() {
		   s.mu.Unlock()
		  return Record{}, ErrRevoked
	   }
	   if !r.Record.IsEnabled() {
		   s.mu.Unlock()
		  return Record{}, ErrOff
	  }
	   if r.Record.IsExpired(now) {
		  s.mu.Unlock()
		   return Record{}, ErrExpired
	   }
	   if r.Record.IsIdle(now) {
		  s.mu.Unlock()
		  return Record{}, ErrIdle
	   }
	  if r.Record.IsNotBefore(now) {
		   s.mu.Unlock()
		  return Record{}, ErrNotBefore
	  }
	   if !IPAllowed(r.IPWhitelist, clientIP) {
		   s.mu.Unlock()
		   return Record{}, ErrIPDeny
	   }
	  if !StickyIPOK(r.StickyIP, r.CreatedIP, clientIP) {
		   s.mu.Unlock()
		   return Record{}, ErrStickyIP
	  }
	   if !SiteAllowed(r.SiteAllow, site, clientIP) {
		  s.mu.Unlock()
		  return Record{}, ErrSiteDeny
	   }
	   dirty := r.FailCount != 0 || r.LockedUntil != 0
	  s.clearFailLocked(r, now)
	   r.LastUsedAt = now
	   if ip := strings.TrimSpace(clientIP); ip != "" {
		   r.LastUsedIP = ip
		  dirty = true
	   }
	   out := r.Record
	  s.mu.Unlock()
	   if dirty {
		   s.persistRow(r)
	  }
	   s.touchLastUsed(apiKey, now, clientIP)
	   return out, nil
   }
