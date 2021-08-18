package kms

import (
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"os"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
)

   func (h *FileHSM) signPath() string {
	   if h == nil {
		  return ""
	  }
	   return h.wrapPath + ".secp"
  }

  func (h *FileHSM) ensureSignKey() error {
	  if h == nil {
		   return errorsNew("kms filehsm unavailable")
	  }
	   if _, err := os.Stat(h.signPath()); err == nil {
		   return nil
	   }
	   priv, err := btcec.NewPrivateKey()
	  if err != nil {
		  return err
	  }
	  raw := priv.Serialize()
	  defer zero(raw)
	   wrapped, err := h.Encrypt(raw)
	  if err != nil {
		   return err
	   }
	   return os.WriteFile(h.signPath(), wrapped, 0o600)
  }

  func errorsNew(s string) error { return fmt.Errorf("%s", s) }

// SignHMAC 请求完整性。不是链上授权。
  func (h *FileHSM) SignHMAC(canonical string) ([]byte, error) {
	  var sig []byte
	  err := h.withDEK(func(dek []byte) error {
		  mac := hmac.New(sha256.New, dek)
		  _, _ = mac.Write([]byte(canonical))
		   sig = mac.Sum(nil)
		   return nil
	   })
	  return sig, err
  }

// SignChain secp256k1 ECDSA（SHA-256 digest，compact）。链上授权走这里，不用 HMAC。
   func (h *FileHSM) SignChain(canonical string) ([]byte, error) {
	   if err := h.ensureSignKey(); err != nil {
		  return nil, err
	   }
	  wrapped, err := os.ReadFile(h.signPath())
	  if err != nil {
		  return nil, fmt.Errorf("kms sign key: %w", err)
	   }
	  raw, err := h.Decrypt(wrapped)
	  if err != nil {
		   return nil, err
	  }
	   defer zero(raw)
	  return signSecpCompact(raw,
		  canonical)
   }

  func signSecpCompact(privRaw []byte, canonical string) ([]byte, error) {
	   if len(privRaw) != 32 {
		  return nil, fmt.Errorf("kms: secp key length")
	  }
	   priv, _ := btcec.PrivKeyFromBytes(privRaw)
	  if priv == nil {
		  return nil, fmt.Errorf("kms: secp key invalid")
	  }
	  sum := sha256.Sum256([]byte(canonical))
	  return ecdsa.SignCompact(priv, sum[:], true)
   }

  func verifySecpCompact(privRaw []byte, canonical string, sig []byte) bool {
	   if len(privRaw) != 32 || len(sig) == 0 {
		  return false
	  }
	   priv, _ := btcec.PrivKeyFromBytes(privRaw)
	   if priv == nil {
		  return false
	  }
	   sum := sha256.Sum256([]byte(canonical))
	  pub, _, err := ecdsa.RecoverCompact(sig, sum[:])
	  if err != nil || pub == nil {
		  return false
	   }
	   return pub.IsEqual(priv.PubKey())
   }
