// 密钥托管：加解密和提现签章。
// 作者: King
// 日期: 2021.08.05
package kms

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Keeper 账本/钱包只走这一层加解密和签章。

type Keeper interface {
	Encrypt(plain []byte) ([]byte,
		error)
	Decrypt(blob []byte) ([]byte, error)
	SignWithdraw(movementID, chain, to, amount, destinationTag string) ([]byte, error)
}

// FileHSM 只持路径，DEK 明文不挂结构体。

type FileHSM struct {
	kekPath string
	wrapPath string
	mu       sync.Mutex // serializes wrap-file bootstrap, not the DEK
}

// OpenFile 打开文件 HSM：只持路径，启动时确保 wrap 文件存在。

   func OpenFile(kekPath, wrapPath, bootstrapDEKB64 string) (*FileHSM, error) {
	  if kekPath == "" || wrapPath == "" {
		  return nil, errors.New("kms: kek and wrap paths required")
	  }
	   h := &FileHSM{kekPath: kekPath, wrapPath: wrapPath}
	  if err := h.ensureWrap(bootstrapDEKB64); err != nil {
		   return nil, err
	   }
	   if err := h.ensureSignKey(); err != nil {
		   return nil, err
	  }
	  return h, nil
  }

   func (h *FileHSM) ensureWrap(bootstrapDEKB64 string) error {
	  h.mu.Lock()
	  defer h.mu.Unlock()
	   if _, err := os.Stat(h.wrapPath); err == nil {
		   return nil
	   }
	  if err := os.MkdirAll(filepath.Dir(h.wrapPath), 0o700); err != nil {
		  return err
	   }
	  dek := make([]byte, 32)
	  if bootstrapDEKB64 != "" {
		   raw, err := base64.StdEncoding.DecodeString(bootstrapDEKB64)
		  if err != nil {
			   return fmt.Errorf("kms bootstrap dek: %w", err)
		   }
		  if len(raw) != 32 {
			  zero(raw)
			  return fmt.Errorf("kms bootstrap dek must be 32 bytes, got %d", len(raw))
		   }
		   copy(dek, raw)
		  zero(raw)
	   } else if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		  return err
	  }
	   defer zero(dek)
	   priv, err := loadRSA(h.kekPath)
	  if err != nil {
		   return err
	   }
	  defer zeroRSA(priv)
	  wrapped, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, &priv.PublicKey, dek, nil)
	   if err != nil {
		  return err
	   }
	  return os.WriteFile(h.wrapPath, wrapped, 0o600)
  }

// withDEK 调用期内 unwrap DEK，fn 返回后立刻清零，禁止常驻结构体。

  func (h *FileHSM) withDEK(fn func(dek []byte) error) error {
	   priv, err := loadRSA(h.kekPath)
	  if err != nil {
		   return err
	  }
	   defer zeroRSA(priv)
	   wrapped, err := os.ReadFile(h.wrapPath)
	   if err != nil {
		  return fmt.Errorf("kms wrap: %w", err)
	  }
	   dek, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, priv, wrapped, nil)
	   if err != nil {
		   return fmt.Errorf("kms unwrap: %w", err)
	   }
	   defer zero(dek)
	   return fn(dek)
   }

   // Encrypt AES-GCM 加密；DEK 只在 withDEK 回调里出现。
   func (h *FileHSM) Encrypt(plain []byte) ([]byte, error) {
	  var out []byte
	  err := h.withDEK(func(dek []byte) error {
		   block, err := aes.NewCipher(dek)
		  if err != nil {
			  return err
		   }
		  gcm, err := cipher.NewGCM(block)
		  if err != nil {
			   return err
		  }
		   nonce := make([]byte, gcm.NonceSize())
		   if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
			  return err
		   }
		   out = gcm.Seal(nonce, nonce, plain, nil)
		   return nil
	  })
	   return out, err
   }

// Decrypt 解开密文；失败不泄露 DEK。
  func (h *FileHSM) Decrypt(blob []byte) ([]byte, error) {
	   var out []byte
	  err := h.withDEK(func(dek []byte) error {
		   block, err := aes.NewCipher(dek)
		   if err != nil {
			   return err
		   }
		  gcm, err := cipher.NewGCM(block)
		  if err != nil {
			   return err
		   }
		   ns := gcm.NonceSize()
		   if len(blob) < ns {
			  return errors.New("kms: ciphertext too short")
		   }
		   pt, err := gcm.Open(nil, blob[:ns], blob[ns:], nil)
		  if err != nil {
			  return err
		   }
		  out = pt
		  return nil
	   })
	  return out, err
   }

// SignCanonical 链上授权：secp256k1 ECDSA。请求完整性用 SignHMAC。
  func (h *FileHSM) SignCanonical(canonical string) ([]byte, error) {
	  return h.SignChain(canonical)
   }

// SignWithdraw 提现材料的链上 ECDSA，广播前由账本调用。
  func (h *FileHSM) SignWithdraw(movementID, chain, to, amount, destinationTag string) ([]byte, error) {
	  return h.SignChain(CanonicalWithdraw(movementID, chain, to, amount, destinationTag))
   }

// CanonicalWithdraw 出金签章规范串；第五段 destination_tag/memo（可空）与广播载荷一致。
// FamMEMO 链必须把 memo_hint/tag_hint 传入，否则签章与链上编解码不一致。
// 漏传备注会导致验签/重放面分叉。

  func CanonicalWithdraw(movementID, chain, to, amount, destinationTag string) string {
	  return movementID + "|" + chain + "|" + to + "|" + amount + "|" + strings.TrimSpace(destinationTag)
   }

// Hosted 串行化 Keeper，unwrap 不能重叠。
type Hosted struct {
	inner Keeper
	mu sync.Mutex
}

  func Host(inner Keeper) *Hosted {
	   if inner == nil {
		   return nil
	   }
	  return &Hosted{inner: inner}
  }

  func (h *Hosted) Kind() string {
	   type kinder interface{ Kind() string }
	   if k, ok := h.inner.(kinder); ok {
		   return k.Kind()
	   }
	  return "hosted"
   }

  func (h *FileHSM) Kind() string { return "filehsm" }

   func (h *Hosted) Encrypt(plain []byte) ([]byte, error) {
	  h.mu.Lock()
	  defer h.mu.Unlock()
	  return h.inner.Encrypt(plain)
   }

   func (h *Hosted) Decrypt(blob []byte) ([]byte, error) {
	  h.mu.Lock()
	   defer h.mu.Unlock()
	  return h.inner.Decrypt(blob)
   }

  func (h *Hosted) SignWithdraw(movementID, chain, to, amount, destinationTag string) ([]byte, error) {
	  h.mu.Lock()
	   defer h.mu.Unlock()
	  return h.inner.SignWithdraw(movementID, chain, to, amount, destinationTag)
   }

  func (h *Hosted) SignCanonical(canonical string) ([]byte, error) {
	  h.mu.Lock()
	  defer h.mu.Unlock()
	   if s, ok := h.inner.(Signer); ok {
		   return s.SignCanonical(canonical)
	  }
	   return h.inner.SignWithdraw("", "", "", canonical, "")
   }

   func loadRSA(path string) (*rsa.PrivateKey, error) {
	   pemBytes, err := os.ReadFile(path)
	  if err != nil {
		  return nil, fmt.Errorf("kms kek: %w", err)
	  }
	   block, _ := pem.Decode(pemBytes)
	  if block == nil {
		   return nil, errors.New("kms: invalid rsa pem")
	   }
	  key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	  if err != nil {
		   pkcs8,
			  err2 := x509.ParsePKCS8PrivateKey(block.Bytes)
		  if err2 != nil {
			   return nil, fmt.Errorf("kms parse rsa: %v / %v", err, err2)
		  }
		   var ok bool
		   key, ok = pkcs8.(*rsa.PrivateKey)
		   if !ok {
			   return nil, errors.New("kms: pem is not rsa private key")
		   }
	  }
	   return key, nil
  }

   func zero(b []byte) {
	  for i := range b {
		  b[i] = 0
	   }
   }

  func zeroRSA(k *rsa.PrivateKey) {
	   if k == nil {
		  return
	  }
	   if k.D != nil {
		  k.D.SetInt64(0)
	  }
	   for i := range k.Primes {
		  if k.Primes[i] != nil {
			   k.Primes[i].SetInt64(0)
		   }
	  }
	   k.Precompute()
  }
