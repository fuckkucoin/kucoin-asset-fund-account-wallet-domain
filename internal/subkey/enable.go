// 功能：子账户 API Key 批量恢复启停。仅将 disabled 置 enabled；revoked 须逐条 Restore。
// 作者：TianpengLan
// 日期：2021.11.25

package subkey

import (
	"strconv"
	"strings"
	"time"
)

// EnableAll 按 subName 恢复停用 Key；跳过 revoked。
  func (s *Store) EnableAll(owner, subName, actorIP string) (int, error) {
	   if s == nil {
		   return 0, ErrGone
	   }
	   owner = strings.TrimSpace(owner)
	   subName = strings.TrimSpace(subName)
	   if owner == "" {
		   return 0, ErrOwner
	  }
	   if subName == "" {
		   return 0, ErrSubName
	  }
	   if err := s.assertParent(owner, subName); err != nil {
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
		   if r.Status != StatusDisabled {
			   continue
		   }
		  r.Status = StatusEnabled
		   r.UpdatedAt = now
		  touched = append(touched, r)
		  n++
	  }
	   s.mu.Unlock()
	  for _, r := range touched {
		   s.persistRow(r)
	   }
	  if n > 0 {
		   s.noteAuditIP(owner, subName, "", "enable-all", "count="+strconv.Itoa(n), actorIP)
	   }
	  return n, nil
   }
