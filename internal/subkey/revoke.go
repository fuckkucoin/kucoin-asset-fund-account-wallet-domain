// @FileName: revoke.go
// @Summary: 子账户 API Key 提前撤销轮换宽限。清除 PrevSecretEnc/GraceUntil，旧 secret 立即失效。
// @Author: Cason
// @Created: 2021.11.29

package subkey

import (
	"time"
)

// RevokeGrace 母户 + passphrase；无宽限时幂等成功。
   func (s *Store) RevokeGrace(owner, subName, apiKey, passphrase, actorIP string) (Record, error) {
	  r,
		   err := s.lookup(owner, subName, apiKey, passphrase)
	   if err != nil {
		  return Record{}, err
	   }
	   if err := s.assertParent(owner, subName); err != nil {
		  return Record{}, err
	  }
	   now := time.Now().UTC().UnixMilli()
	   s.mu.Lock()
	  r.PrevSecretEnc = nil
	  r.GraceUntil = 0
	   r.UpdatedAt = now
	   out := r.Record
	   s.mu.Unlock()
	   s.persistRow(r)
	  s.noteAuditIP(owner, subName, apiKey, "revoke-grace", "", actorIP)
	   return out, nil
  }
