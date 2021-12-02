package subkey

import (
	"strconv"
	"strings"
	"time"
)

// SweepExpired 按母户（可选 subName）停用已过期 Key；返回停用条数。

   func (s *Store) SweepExpired(owner, subName, actorIP string) (int, error) {
	  if s == nil {
		   return 0, ErrGone
	  }
	  owner = strings.TrimSpace(owner)
	   subName = strings.TrimSpace(subName)
	   if owner == "" {
		   return 0, ErrOwner
	  }
	   if subName != "" {
		  if err := s.assertParent(owner, subName); err != nil {
			  return 0, err
		   }
	  }
	  now := time.Now().UTC().UnixMilli()
	   s.mu.Lock()
	   n := 0
	   touched := make([]*row, 0)
	   for _, r := range s.by {
		  if r.Owner != owner {
			  continue
		   }
		   if subName != "" && r.SubName != subName {
			   continue
		   }
		  if r.Status == StatusDisabled || r.Status == StatusRevoked {
			  continue
		  }
		   if !r.Record.IsExpired(now) {
			  continue
		  }
		  r.Status = StatusDisabled
		  r.UpdatedAt = now
		  touched = append(touched, r)
		  n++
	  }
	  s.mu.Unlock()
	   for _, r := range touched {
		  s.persistRow(r)
	   }
	  if n > 0 {
		   s.noteAuditIP(owner, subName, "", "sweep-expired", "count="+strconv.Itoa(n), actorIP)
	   }
	  return n, nil
  }

// SweepIdle 按母户（可选 subName）停用已空闲超时仍 enabled 的 Key；返回停用条数。

   func (s *Store) SweepIdle(owner, subName, actorIP string) (int, error) {
	   if s == nil {
		  return 0, ErrGone
	   }
	  owner = strings.TrimSpace(owner)
	  subName = strings.TrimSpace(subName)
	   if owner == "" {
		  return 0, ErrOwner
	   }
	   if subName != "" {
		  if err := s.assertParent(owner,
			  subName); err != nil {
			   return 0, err
		  }
	   }
	  now := time.Now().UTC().UnixMilli()
	   s.mu.Lock()
	   n := 0
	   touched := make([]*row, 0)
	   for _, r := range s.by {
		  if r.Owner != owner {
			  continue
		  }
		  if subName != "" && r.SubName != subName {
			  continue
		   }
		  if r.Status == StatusDisabled || r.Status == StatusRevoked {
			  continue
		  }
		   if !r.Record.IsIdle(now) {
			  continue
		   }
		   r.Status = StatusDisabled
		   r.UpdatedAt = now
		  touched = append(touched, r)
		   n++
	   }
	  s.mu.Unlock()
	  for _, r := range touched {
		  s.persistRow(r)
	   }
	   if n > 0 {
		  s.noteAuditIP(owner, subName, "", "sweep-idle", "count="+strconv.Itoa(n), actorIP)
	  }
	   return n, nil
  }

// SweepGrace 按母户（可选 subName）清除已到期轮换宽限（PrevSecretEnc/GraceUntil）；返回清理条数。
  func (s *Store) SweepGrace(owner, subName, actorIP string) (int, error) {
	   if s == nil {
		  return 0, ErrGone
	  }
	   owner = strings.TrimSpace(owner)
	  subName = strings.TrimSpace(subName)
	   if owner == "" {
		  return 0, ErrOwner
	  }
	   if subName != "" {
		   if err := s.assertParent(owner, subName); err != nil {
			  return 0, err
		  }
	   }
	  now := time.Now().UTC().UnixMilli()
	   s.mu.Lock()
	   n := 0
	  touched := make([]*row, 0)
	   for _, r := range s.by {
		  if r.Owner != owner {
			  continue
		  }
		  if subName != "" && r.SubName != subName {
			   continue
		   }
		   if r.GraceUntil <= 0 || r.GraceUntil > now {
			   continue
		   }
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
		   s.noteAuditIP(owner, subName, "", "sweep-grace", "count="+strconv.Itoa(n), actorIP)
	   }
	  return n, nil
  }
