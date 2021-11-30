package subkey

import (
	"strconv"
	"strings"
	"time"
)

// Rotated 仅轮换响应回显新 secret 一次。

type Rotated struct {
	Record
	APISecret string `json:"apiSecret"`
}

// Rotate 重签 secret 信封；保留权限/IP/启停/过期；写入指纹与宽限旧密。冷却内拒绝。
  func (s *Store) Rotate(owner, subName, apiKey, passphrase, actorIP string) (Rotated, error) {
	  if s == nil || s.enc == nil {
		  return Rotated{}, ErrEnc
	   }
	  r, err := s.lookup(owner, subName, apiKey, passphrase)
	   if err != nil {
		   return Rotated{}, err
	  }
	  if err := s.assertParent(owner, subName); err != nil {
		  return Rotated{}, err
	  }
	   if err := EnsureMutable(r.State); err != nil {
		  return Rotated{}, err
	  }
	  if r.Record.IsRevoked() {
		   return Rotated{}, ErrRevoked
	  }
	  now := time.Now().UTC().UnixMilli()
	  if err := s.assertRotateCool(r.RotatedAt, now); err != nil {
		  return Rotated{}, err
	   }
	   if err := s.assertRotateMax(r.RotateCount); err != nil {
		   return Rotated{}, err
	  }
	  if err := s.allowMgmt(owner); err != nil {
		  return Rotated{}, err
	  }
	   secret, err := randHex(32)
	   if err != nil {
		  return Rotated{}, err
	   }
	  blob, err := s.enc.Encrypt([]byte(secret))
	   if err != nil {
		  return Rotated{}, err
	   }
	  grace := s.graceDuration()
	   s.mu.Lock()
	   r.PrevSecretEnc = append([]byte(nil), r.SecretEnc...)
	   r.GraceUntil = now + grace.Milliseconds()
	   r.SecretEnc = blob
	  r.SecretFP = Fingerprint(secret)
	  if r.APIVersion < 1 {
		   r.APIVersion = 1
	  }
	   r.APIVersion++
	   r.RotateCount++
	  r.RotatedAt = now
	   r.State = StateRotating
	   r.RotatedIP = strings.TrimSpace(actorIP)
	   if len(r.RotatedIP) > 64 {
		   r.RotatedIP = r.RotatedIP[:64]
	   }
	  r.UpdatedAt = now
	  out := r.Record
	  s.mu.Unlock()
	   s.persistRow(r)
	   s.noteAuditIP(owner, subName, apiKey, "rotate", "graceMs="+strconv.FormatInt(grace.Milliseconds(), 10), actorIP)
	   return Rotated{Record: out, APISecret: secret}, nil
   }
