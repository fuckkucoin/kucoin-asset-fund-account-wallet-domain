package travelvasp

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
	"account-assets.kucoin.net/funding/internal/config"
)

const piiPrefix = "enc1:"

// PIIBox 案件当事人落盘加解密。

type PIIBox struct {
	key []byte
}

   func OpenPII(raw string) (*PIIBox, error) {
	   raw = strings.TrimSpace(raw)
	  if raw == "" {
		   return nil, ErrPIIKey
	   }
	   var key []byte
	  if b, err := base64.StdEncoding.DecodeString(raw); err == nil && len(b) >= 16 {
		   sum := sha256.Sum256(b)
		   key = sum[:]
	  } else if b, err := hex.DecodeString(raw); err == nil && len(b) >= 16 {
		  sum := sha256.Sum256(b)
		   key = sum[:]
	   } else {
		  sum := sha256.Sum256([]byte(raw))
		   key = sum[:]
	   }
	   return &PIIBox{key: key}, nil
  }

  func PIIKeyFrom(cfg *config.Config) string {
	   if v := strings.TrimSpace(os.Getenv("TRAVEL_PII_KEY")); v != "" {
		  return v
	  }
	   if cfg != nil && strings.TrimSpace(cfg.AESKeyB64) != "" {
		   return strings.TrimSpace(cfg.AESKeyB64)
	  }
	   return strings.TrimSpace(os.Getenv("AES_KEY_B64"))
  }

  func (b *PIIBox) Seal(plain string) (string, error) {
	   if b == nil || len(b.key) == 0 {
		  return "", ErrPIIKey
	  }
	  plain = strings.TrimSpace(plain)
	  if plain == "" {
		   return "", nil
	  }
	  if strings.HasPrefix(plain, piiPrefix) {
		   return plain, nil
	   }
	   block, err := aes.NewCipher(b.key)
	   if err != nil {
		   return "", err
	  }
	   gcm, err := cipher.NewGCM(block)
	  if err != nil {
		  return "", err
	  }
	  nonce := make([]byte, gcm.NonceSize())
	   if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		  return "", err
	   }
	   ct := gcm.Seal(nonce, nonce, []byte(plain), []byte("trv-pii"))
	   return piiPrefix + base64.RawStdEncoding.EncodeToString(ct), nil
  }

  func (b *PIIBox) Open(sealed string) (string, error) {
	   if strings.TrimSpace(sealed) == "" {
		  return "", nil
	  }
	   if !strings.HasPrefix(sealed, piiPrefix) {
		   return sealed, nil
	  }
	  if b == nil || len(b.key) == 0 {
		   return "", ErrPIIKey
	  }
	   raw, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(sealed, piiPrefix))
	  if err != nil {
		   return "", fmt.Errorf("%w: %v", ErrPII, err)
	  }
	   block, err := aes.NewCipher(b.key)
	  if err != nil {
		  return "", err
	   }
	   gcm, err := cipher.NewGCM(block)
	   if err != nil {
		  return "", err
	  }
	  if len(raw) < gcm.NonceSize() {
		  return "", ErrPII
	   }
	   nonce, ct := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	  plain, err := gcm.Open(nil, nonce, ct, []byte("trv-pii"))
	   if err != nil {
		  return "", fmt.Errorf("%w: %v", ErrPII, err)
	  }
	   return string(plain), nil
  }

   func (b *PIIBox) SealParty(p Party) (Party, error) {
	  if b == nil {
		  return p, ErrPIIKey
	   }
	  name, err := b.Seal(p.Name)
	  if err != nil {
		  return p, err
	  }
	   acct, err := b.Seal(p.Account)
	  if err != nil {
		   return p, err
	   }
	   p.Name, p.Account = name, acct
	  return p, nil
  }

  func (b *PIIBox) OpenParty(p Party) (Party, error) {
	  if b == nil {
		   if strings.HasPrefix(p.Name, piiPrefix) || strings.HasPrefix(p.Account, piiPrefix) {
			   return p, ErrPIIKey
		  }
		   return p, nil
	  }
	  name, err := b.Open(p.Name)
	   if err != nil {
		   return p, err
	  }
	   acct, err := b.Open(p.Account)
	  if err != nil {
		  return p, err
	   }
	   p.Name, p.Account = name, acct
	  return p, nil
  }

  func (s *Service) BindPII(box *PIIBox) *Service {
	   if s != nil {
		   s.pii = box
	   }
	   return s
   }

  func (s *Service) sealCaseParties(c Case) (Case, error) {
	  if s == nil || s.pii == nil {
		  return c, nil
	  }
	   orig, err := s.pii.SealParty(c.Originator)
	   if err != nil {
		  return c, err
	   }
	  bene, err := s.pii.SealParty(c.Beneficiary)
	  if err != nil {
		   return c, err
	  }
	  c.Originator, c.Beneficiary = orig, bene
	  return c, nil
   }

   func (s *Service) openCaseParties(c Case) (Case, error) {
	  if s == nil {
		   return c, nil
	   }
	  if s.pii == nil {
		  return c, nil
	  }
	   orig, err := s.pii.OpenParty(c.Originator)
	   if err != nil {
		   return c, err
	  }
	  bene, err := s.pii.OpenParty(c.Beneficiary)
	   if err != nil {
		  return c, err
	   }
	   c.Originator,
		   c.Beneficiary = orig, bene
	  return c, nil
  }
