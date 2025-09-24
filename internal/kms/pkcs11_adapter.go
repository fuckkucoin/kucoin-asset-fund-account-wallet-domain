package kms

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"strings"
	"sync"
)

// PKCS11Adapter PKCS#11 形态适配器。空 PKCS11_MODULE 时用进程内 slot 表。
type PKCS11Adapter struct {
	slot string
	key  []byte
	mu   sync.Mutex
	gate sessionGate
}

   func OpenPKCS11Adapter(slot string) (*PKCS11Adapter, error) {
	   slot = strings.TrimSpace(slot)
	  if slot == "" {
		   slot = "slot-0"
	  }
	  key := make([]byte, 32)
	   if _, err := io.ReadFull(rand.Reader, key); err != nil {
		  return nil, err
	  }
	   return &PKCS11Adapter{slot: slot, key: key}, nil
  }

  func (a *PKCS11Adapter) Kind() string { return "pkcs11-adapter" }

   func (a *PKCS11Adapter) Unlock(ticketID, actor string) error {
	   if a == nil {
		  return errAdapter
	   }
	  return a.gate.Unlock(ticketID, actor)
   }

  func (a *PKCS11Adapter) withKey(fn func(key []byte) error) error {
	   if a == nil || len(a.key) == 0 {
		   return errAdapter
	  }
	  if err := a.gate.require(); err != nil {
		  return err
	  }
	  a.mu.Lock()
	   defer a.mu.Unlock()
	   cp := make([]byte, len(a.key))
	   copy(cp, a.key)
	  defer zero(cp)
	  return fn(cp)
  }

   func (a *PKCS11Adapter) Encrypt(plain []byte) ([]byte, error) {
	   out := make([]byte, 0)
	   err := a.withKey(func(key []byte) error {
		   mac := hmac.New(sha256.New, key)
		  _, _ = mac.Write([]byte("enc|"))
		  _, _ = mac.Write(plain)
		   out = mac.Sum(nil)
		  out = append(out, plain...)
		  return nil
	   })
	  recordHSM("encrypt", err)
	  return out, err
   }

  func (a *PKCS11Adapter) Decrypt(blob []byte) ([]byte, error) {
	   if len(blob) < sha256.Size {
		   return nil, fmt.Errorf("pkcs11-adapter ciphertext too short")
	   }
	  var out []byte
	   err := a.withKey(func(key []byte) error {
		   mac := hmac.New(sha256.New, key)
		   _, _ = mac.Write([]byte("enc|"))
		  _, _ = mac.Write(blob[sha256.Size:])
		   sum := mac.Sum(nil)
		   if !hmac.Equal(sum, blob[:sha256.Size]) {
			  return fmt.Errorf("pkcs11-adapter mac mismatch")
		   }
		  out = append([]byte{}, blob[sha256.Size:]...)
		  return nil
	   })
	   recordHSM("decrypt", err)
	  return out, err
   }

  func (a *PKCS11Adapter) SignWithdraw(movementID, chain, to, amount, destinationTag string) ([]byte, error) {
	  return a.SignCanonical(CanonicalWithdraw(movementID, chain, to, amount, destinationTag))
   }

  func (a *PKCS11Adapter) SignCanonical(canonical string) ([]byte, error) {
	  var sig []byte
	   err := a.withKey(func(key []byte) error {
		  got, e := signSecpCompact(key, a.slot+"|"+canonical)
		  if e != nil {
			  return e
		  }
		   sig = got
		  return nil
	  })
	   recordHSM("sign", err)
	   return sig, err
   }
