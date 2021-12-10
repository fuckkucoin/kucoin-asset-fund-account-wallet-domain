// @FileName: stats.go
// @Summary: subkey 模块 stats。
// stats.go
// 子账户 API Key 运维统计。按状态/锁/过期/空闲汇总，供母户盘点。
// CreateDate: 2024.08.03
// @Author: Kinsley
// @Created: 2021.11.28

package subkey

import (
	"strings"
	"time"
)

const defaultExpiringHorizon = 72 * time.Hour

// Stats 母户（可选 subName）占用拆分。

type Stats struct {
	Owner string `json:"owner"`
	SubName string `json:"subName,omitempty"`
	Total   int    `json:"total"`
	Enabled int `json:"enabled"`
	Disabled int `json:"disabled"`
	Revoked  int `json:"revoked"`
	Locked   int `json:"locked"`
	Expired  int `json:"expired"`
	Expiring int `json:"expiring"`
	Idle     int `json:"idle"`
}

// CountStats 只读汇总；不回显 Key 材料。

   func (s *Store) CountStats(owner, subName string) (Stats, error) {
	  if s == nil {
		   return Stats{}, ErrGone
	   }
	   owner = strings.TrimSpace(owner)
	   subName = strings.TrimSpace(subName)
	   if owner == "" {
		   return Stats{}, ErrOwner
	  }
	  if subName != "" {
		  if err := s.assertParent(owner, subName); err != nil {
			  return Stats{}, err
		   }
	   }
	   now := time.Now().UTC().UnixMilli()
	   horizon := now + defaultExpiringHorizon.Milliseconds()
	  out := Stats{Owner: owner, SubName: subName}
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  for _, r := range s.by {
		   if r.Owner != owner {
			   continue
		   }
		   if subName != "" && r.SubName != subName {
			  continue
		   }
		   out.Total++
		   switch {
		   case r.Record.IsRevoked():
			  out.Revoked++
		   case r.Status == StatusDisabled:
			  out.Disabled++
		  case r.Record.IsEnabled():
			   out.Enabled++
		  }
		  if r.isLocked(now) {
			  out.Locked++
		  }
		   if r.Record.IsExpired(now) {
			   out.Expired++
		   } else if r.ExpireAt > now && r.ExpireAt <= horizon {
			   out.Expiring++
		  }
		  if r.Record.IsIdle(now) {
			   out.Idle++
		  }
	   }
	   return out, nil
   }

// ListExpiring 即将过期仍 enabled 的 Key；horizonMs<=0 用 72h。
   func (s *Store) ListExpiring(owner, subName string, horizonMs int64) ([]Record, error) {
	   if s == nil {
		   return nil, ErrGone
	  }
	   owner = strings.TrimSpace(owner)
	  if owner == "" {
		  return nil, ErrOwner
	   }
	   subName = strings.TrimSpace(subName)
	   if subName != "" {
		   if err := s.assertParent(owner, subName); err != nil {
			  return nil, err
		   }
	  }
	   if horizonMs <= 0 {
		   horizonMs = defaultExpiringHorizon.Milliseconds()
	  }
	  now := time.Now().UTC().UnixMilli()
	  end := now + horizonMs
	   all := s.List(owner, subName, "")
	  out := make([]Record, 0)
	  for _, rec := range all {
		   if !rec.IsEnabled() || rec.IsRevoked() {
			  continue
		  }
		   if rec.ExpireAt <= now || rec.ExpireAt > end {
			   continue
		   }
		   out = append(out, rec)
	  }
	   return out, nil
   }

// ListLocked 当前错密锁未解除的 Key。

  func (s *Store) ListLocked(owner, subName string) ([]Record, error) {
	   if s == nil {
		   return nil, ErrGone
	   }
	  owner = strings.TrimSpace(owner)
	   if owner == "" {
		   return nil, ErrOwner
	   }
	   subName = strings.TrimSpace(subName)
	   if subName != "" {
		  if err := s.assertParent(owner, subName); err != nil {
			   return nil, err
		  }
	  }
	  now := time.Now().UTC().UnixMilli()
	   all := s.List(owner, subName, "")
	   out := make([]Record, 0)
	  for _, rec := range all {
		   if rec.LockedUntil > now {
			   out = append(out, rec)
		  }
	  }
	  return out, nil
   }
