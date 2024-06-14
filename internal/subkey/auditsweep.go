// @FileName: auditsweep.go
// @Summary: 子账户 API Key 审计留存清扫。按母户删除早于截止毫秒的审计行，控制表膨胀。
// olderThanMs<=0 默认保留 30 天。
// @Author: Kinsley
// @Created: 2021.11.24

package subkey

import (
	"strconv"
	"strings"
	"time"
)

const defaultAuditRetain = 30 * 24 * time.Hour

// SweepAudit 删除该母户早于截止的审计；返回删除条数。

  func (s *Store) SweepAudit(owner string, olderThanMs int64, actorIP string) (int, error) {
	  if s == nil {
		   return 0, ErrGone
	   }
	   owner = strings.TrimSpace(owner)
	   if owner == "" {
		   return 0, ErrOwner
	  }
	   now := time.Now().UTC().UnixMilli()
	   if olderThanMs <= 0 {
		  olderThanMs = now - defaultAuditRetain.Milliseconds()
	   }
	   sqlN := s.deleteAuditBefore(owner, olderThanMs)
	   auditMu.Lock()
	   kept := s.audits[:0]
	  memN := 0
	   for _, ev := range s.audits {
		   if ev.Owner == owner && ev.At < olderThanMs {
			  memN++
			   continue
		  }
		   kept = append(kept, ev)
	  }
	  s.audits = append([]AuditEvent(nil), kept...)
	  auditMu.Unlock()
	  n := sqlN
	  if n == 0 {
		   n = memN
	  }
	   if n > 0 {
		   s.noteAuditIP(owner, "", "", "sweep-audit", "count="+strconv.Itoa(n), actorIP)
	  }
	   return n, nil
   }
