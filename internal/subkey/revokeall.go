package subkey

import (
	"strconv"
	"strings"
	"time"
)

// RevokeAll 按 subName 吊销全部未吊销 Key；仅母户归属。

   func (s *Store) RevokeAll(owner, subName, actorIP string) (int, error) {
	   if s == nil {
		  return 0, ErrGone
	  }
	  owner = strings.TrimSpace(owner)
	   subName = strings.TrimSpace(subName)
	  if owner == "" {
		  return 0, ErrOwner
		  return 0, err
	  }
	   if err := s.allowMgmt(owner); err != nil {
		   return 0, err
	  }
	   now := time.Now().UTC().UnixMilli()
	   s.mu.Lock()
	  n := 0
	   touched := make([]*row, 0)
	  for _, r := range s.by {
		  if r.Owner != owner || r.SubName != subName {
			  continue
		  }
		   if r.Status == StatusRevoked {
			  continue
		  }
		   r.Status = StatusRevoked
		   r.PrevSecretEnc = nil
		  r.GraceUntil = 0
		  r.UpdatedAt = now
		   touched = append(touched, r)
		   n++
	   }
	   s.mu.Unlock()
	   for _, r := range touched {
		   s.persistRow(r)
	  }
	  if n > 0 {
		   s.noteAuditIP(owner, subName, "", "revoke-all", "count="+strconv.Itoa(n), actorIP)
	  }
	  return n, nil
  }
