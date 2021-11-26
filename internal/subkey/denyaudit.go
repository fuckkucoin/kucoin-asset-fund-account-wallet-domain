// 功能：子账户 API Key 调用面拒绝审计。记录验签/IP/站点/锁/重放失败，不含 secret/passphrase。
// 爆破会放大审计量，须配合 SweepAudit 留存。

// 作者：TianpengLan
// 日期：2021.11.25

package subkey

import (
	"errors"
	"strings"
)

// noteDeny 调用面拒绝落审计；未知 Key 不记，避免枚举。

   func (s *Store) noteDeny(apiKey, clientIP string, err error) {
	  if s == nil || err == nil {
		   return
	  }
	  apiKey = strings.TrimSpace(apiKey)
	   if apiKey == "" {
		  return
	  }
	   switch {
	  case errors.Is(err, ErrSign), errors.Is(err, ErrIPDeny), errors.Is(err, ErrStickyIP),
		   errors.Is(err, ErrSiteDeny), errors.Is(err, ErrLocked), errors.Is(err, ErrReplay),
		  errors.Is(err,
			   ErrPassHdr), errors.Is(err, ErrSignVer):
	   default:
		   return
	   }
	   s.mu.Lock()
	  r, ok := s.by[apiKey]
	   if !ok {
		  s.mu.Unlock()
		   return
	  }
	  owner, sub := r.Owner, r.SubName
	   s.mu.Unlock()
	   s.noteAuditIP(owner, sub, apiKey, "deny", err.Error(), clientIP)
   }
