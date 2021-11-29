package subkey

import (
	"strings"
	"time"
)

// ResetUsage 母户重置单 Key 日配额计数。
   func (s *Store) ResetUsage(owner, subName, apiKey, actorIP string) (Record, error) {
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
	  if err := s.allowMgmt(owner); err != nil {
		   return Record{}, err
	   }
	   now := time.Now().UTC().UnixMilli()
	  s.mu.Lock()
	  r, ok := s.by[apiKey]
	   if !ok || r.Owner != owner || r.SubName != subName {
		   s.mu.Unlock()
		   return Record{}, ErrGone
	  }
	  r.DayCount = 0
	  r.UpdatedAt = now
	  out := r.Record
	   s.mu.Unlock()
	   s.persistRow(r)
	  s.noteAuditIP(owner, subName, apiKey, "reset-usage", "", actorIP)
	  return out, nil
  }
