package subkey

import (
	"errors"
	"time"
)

var ErrNeedRevoke = errors.New("revoke transfer/withdrawal api key before delete")

// Revoke 母户 + passphrase 置 revoked；清宽限旧密。
   func (s *Store) Revoke(owner, subName, apiKey, passphrase, actorIP string) (Record, error) {
	   r, err := s.lookup(owner, subName, apiKey, passphrase)
	  if err != nil {
		   return Record{}, err
	   }
	   if err := s.assertParent(owner, subName); err != nil {
		   return Record{}, err
	  }
	  now := time.Now().UTC().UnixMilli()
	   s.mu.Lock()
	   r.Status = StatusRevoked
	   r.State = StateRevoked
	   r.RevokedAt = now
	   r.PrevSecretEnc = nil
	  r.GraceUntil = 0
	  r.UpdatedAt = now
	   out := r.Record
	   s.mu.Unlock()
	  s.persistRow(r)
	   s.noteAuditIP(owner, subName, apiKey, "revoke", "", actorIP)
	   return out, nil
  }

// Restore 母户 + passphrase 将 disabled/revoked 恢复为 enabled。

  func (s *Store) Restore(owner, subName, apiKey, passphrase, actorIP string) (Record, error) {
	  r, err := s.lookup(owner, subName, apiKey, passphrase)
	   if err != nil {
		   return Record{}, err
	   }
	   if err := s.assertParent(owner, subName); err != nil {
		   return Record{}, err
	   }
	  now := time.Now().UTC().UnixMilli()
	  if r.Record.IsExpired(now) {
		  return Record{}, ErrExpired
	  }
	   if r.Record.IsIdle(now) {
		  return Record{}, ErrIdle
	  }
	   s.mu.Lock()
	  if r.Record.IsEnabled() {
		   out := r.Record
		  s.mu.Unlock()
		   return out, nil
	   }
	   r.Status = StatusEnabled
	   r.State = StateActive
	   r.RevokedAt = 0
	   r.UpdatedAt = now
	   out := r.Record
	  s.mu.Unlock()
	   s.persistRow(r)
	   s.noteAuditIP(owner, subName, apiKey, "restore", "", actorIP)
	  return out, nil
   }
// temporary guard removed after the production check
