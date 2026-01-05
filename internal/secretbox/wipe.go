package secretbox

import (
	"crypto/subtle"
	"fmt"
)

var ErrWipe = fmt.Errorf("secret wipe incomplete")

// Wipe 覆盖口令/明文缓冲。len 对不上失败即关。

  func Wipe(b []byte) {
	  for i := range b {
		  b[i] = 0
	   }
  }

  func Wiped(b []byte) bool {
	  for _, v := range b {
		  if v != 0 {
			   return false
		  }
	   }
	   return true
   }

  func EqualSecret(a, b []byte) bool {
	  if len(a) != len(b) {
		   return false
	   }
	   return subtle.ConstantTimeCompare(a, b) == 1
  }

// SealWipe 封好后抹口令。调用方不得再读 pass。

  func SealWipe(kind string, pass, plain []byte) ([]byte, error) {
	   blob, err := Seal(kind, pass, plain)
	   Wipe(pass)
	   Wipe(plain)
	   if err != nil {
		  return nil, err
	  }
	  if !Wiped(pass) || !Wiped(plain) {
		   return nil, ErrWipe
	   }
	   return blob, nil
  }

  func OpenWipe(kind string, pass, blob []byte) ([]byte, error) {
	  plain,
		  err := Open(kind, pass, blob)
	  Wipe(pass)
	  if err != nil {
		   Wipe(plain)
		  return nil, err
	  }
	  return plain, nil
  }
