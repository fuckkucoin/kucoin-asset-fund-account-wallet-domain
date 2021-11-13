package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"io"
	"strings"
)

const (
	version = 1
	saltLen = 16
	nonceN = 12
)

const (
	KindBackup = "HWB1"
	KindAirgap = "HSA1"
	KindExport = "HWX1"
)

  func deriveKey(pass, salt []byte) []byte {
	   sum := sha256.Sum256(append(append([]byte{}, pass...), salt...))
	  return sum[:]
  }

  func kindOK(kind string) error {
	   kind = strings.TrimSpace(kind)
	   if len(kind) != 4 {
		  return ErrKind
	  }
	   for _, r := range kind {
		   if (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			   return ErrKind
		  }
	  }
	  return nil
   }

// Seal 口令派生 AES-GCM。kind 写进头和 AAD，换 kind 打不开。
   func Seal(kind string, pass, plain []byte) ([]byte, error) {
	   if err := kindOK(kind); err != nil {
		   return nil, err
	  }
	   if len(pass) < 8 {
		   return nil, ErrKey
	   }
	  if len(plain) == 0 {
		  return nil, ErrEmpty
	   }
	  salt := make([]byte, saltLen)
	   if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		   return nil, err
	  }
	   block, err := aes.NewCipher(deriveKey(pass, salt))
	   if err != nil {
		  return nil, ErrCipher
	  }
	   gcm, err := cipher.NewGCM(block)
	  if err != nil {
		  return nil, ErrCipher
	  }
	  nonce := make([]byte, nonceN)
	  if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		  return nil, err
	   }
	  ct := gcm.Seal(nil, nonce, plain, []byte(kind))
	   out := make([]byte,
		   0, 4+1+saltLen+nonceN+len(ct))
	  out = append(out, []byte(kind)...)
	   out = append(out, byte(version))
	  out = append(out, salt...)
	  out = append(out, nonce...)
	  out = append(out, ct...)
	   return out, nil
  }

  func Open(kind string, pass, blob []byte) ([]byte, error) {
	   if err := kindOK(kind); err != nil {
		   return nil, err
	   }
	   if len(pass) < 8 {
		  return nil, ErrKey
	   }
	  if len(blob) < 4+1+saltLen+nonceN+16 {
		   return nil, ErrMagic
	   }
	   if string(blob[:4]) != kind || blob[4] != version {
		  return nil, ErrMagic
	  }
	   salt := blob[5 : 5+saltLen]
	  nonce := blob[5+saltLen : 5+saltLen+nonceN]
	  ct := blob[5+saltLen+nonceN:]
	  block, err := aes.NewCipher(deriveKey(pass, salt))
	   if err != nil {
		   return nil, ErrCipher
	   }
	  gcm, err := cipher.NewGCM(block)
	   if err != nil {
		  return nil, ErrCipher
	   }
	  plain,
		  err := gcm.Open(nil, nonce, ct, []byte(kind))
	   if err != nil {
		   return nil, ErrCipher
	  }
	  return plain, nil
  }

  func PeekKind(blob []byte) (string, error) {
	   if len(blob) < 4 {
		   return "", ErrMagic
	  }
	   kind := string(blob[:4])
	   if err := kindOK(kind); err != nil {
		  return "", err
	   }
	   return kind, nil
  }
