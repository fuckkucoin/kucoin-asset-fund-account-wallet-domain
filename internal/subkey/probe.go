package subkey

import (
	"strings"
	"time"
)

// Probe 母户只读过闸预检；失败返回与调用面相同错误。
   func (s *Store) Probe(owner, subName, apiKey, clientIP, site, method, path, wantPerm string) (Record, error) {
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
	   s.mu.Lock()
	   r, ok := s.by[apiKey]
	  if !ok || r.Owner != owner || r.SubName != subName {
		  s.mu.Unlock()
		  return Record{}, ErrGone
	  }
	  rec := r.Record
	   now := time.Now().UTC().UnixMilli()
	  if r.isLocked(now) {
		   s.mu.Unlock()
		   return Record{}, ErrLocked
	  }
	  s.mu.Unlock()
	  if rec.IsRevoked() {
		  return Record{}, ErrRevoked
	  }
	   if !rec.IsEnabled() {
		   return Record{}, ErrOff
	   }
	   if rec.IsExpired(now) {
		   return Record{}, ErrExpired
	   }
	   if rec.IsIdle(now) {
		   return Record{}, ErrIdle
	   }
	   if rec.IsNotBefore(now) {
		  return Record{}, ErrNotBefore
	   }
	  if !IPAllowed(rec.IPWhitelist, clientIP) {
		   return Record{}, ErrIPDeny
	   }
	  if !StickyIPOK(rec.StickyIP, rec.CreatedIP, clientIP) {
		  return Record{}, ErrStickyIP
	   }
	   if !SiteAllowed(rec.SiteAllow, site, clientIP) {
		   return Record{}, ErrSiteDeny
	  }
	  if !PathAllowed(rec.PathAllow, path) {
		  return Record{}, ErrPathDeny
	  }
	   if !MethodAllowed(rec.MethodAllow, method) {
		  return Record{}, ErrMethodDeny
	   }
	   if want := strings.TrimSpace(wantPerm); want != "" && !HasPerm(rec.Permission, want) {
		   return Record{}, ErrPermNeed
	   }
	   return rec, nil
  }
