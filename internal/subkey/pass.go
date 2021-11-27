// 功能：子账户 API Key 口令更换。旧口令校验通过后写入新口令哈希与 KMS 口令信封；不回显 secret。
// 作者：TianpengLan
// 日期：2021.11.25

package subkey

import (
	"crypto/sha256"
	"crypto/subtle"
	"strings"
	"time"
)

// ChangePassphrase 母户持旧口令换新口令；长度规则同创建。
  func (s *Store) ChangePassphrase(owner, subName, apiKey, oldPass, newPass, actorIP string) (Record, error) {
	  r,
		  err := s.lookup(owner, subName, apiKey, oldPass)
	  if err != nil {
		   return Record{}, err
	   }
	  if err := s.assertParent(owner, subName); err != nil {
		  return Record{}, err
	  }
	  newPass = strings.TrimSpace(newPass)
	   if err := checkPassphrase(newPass); err != nil {
		  return Record{}, err
	   }
	   sum := sha256.Sum256([]byte(newPass))
	   if subtle.ConstantTimeCompare(r.PassHash[:], sum[:]) == 1 {
		   return Record{}, ErrPassReuse
	  }
	   passEnc, err := s.enc.Encrypt([]byte(newPass))
	  if err != nil {
		  return Record{}, err
	  }
	   now := time.Now().UTC().UnixMilli()
	   s.mu.Lock()
	  r.PassHash = sum
	  r.PassEnc = passEnc
	  r.UpdatedAt = now
	   out := r.Record
	   s.mu.Unlock()
	   s.persistRow(r)
	  s.noteAuditIP(owner, subName, apiKey, "passphrase", "", actorIP)
	   return out, nil
  }
