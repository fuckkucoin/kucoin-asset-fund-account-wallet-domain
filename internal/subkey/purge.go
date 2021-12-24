// olderThanMs<=0 默认 7 天；未满留存的 revoked 保留供审计。

// Author: Axel
// Date: 2021.11.28

package subkey

import (
	"strconv"
	"strings"
	"time"
)

const defaultRevokedRetain = 7 * 24 * time.Hour

// PurgeRevoked 删除该母户（可选 subName）已满留存的 revoked Key。
  func (s *Store) PurgeRevoked(owner, subName string, olderThanMs int64, actorIP string) (int, error) {
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
	   if err := s.allowMgmt(owner); err != nil {
		  return 0, err
	  }
	  now := time.Now().UTC().UnixMilli()
	   cutoff := olderThanMs
	  if cutoff <= 0 {
		  cutoff = now - defaultRevokedRetain.Milliseconds()
	  }
	   s.mu.Lock()
	   gone := make([]string, 0)
	   for _, r := range s.by {
		  if r.Owner != owner {
			  continue
		   }
		  if subName != "" && r.SubName != subName {
			   continue
		  }
		  if !r.Record.IsRevoked() {
			  continue
		   }
		  base := r.UpdatedAt
		   if base <= 0 {
			   base = r.CreatedAt
		  }
		   if base > cutoff {
			  continue
		  }
		  gone = append(gone, r.APIKey)
		  delete(s.by, r.APIKey)
	  }
	   s.mu.Unlock()
	   for _, k := range gone {
		  s.deleteRow(k)
	  }
	   n := len(gone)
	  if n > 0 {
		  s.noteAuditIP(owner, subName, "", "purge-revoked", "count="+strconv.Itoa(n), actorIP)
	   }
	   return n, nil
   }
