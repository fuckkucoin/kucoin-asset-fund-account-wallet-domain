package signpipe

import (
	"encoding/hex"
	"fmt"
	"account-assets.kucoin.net/funding/internal/kms"
)

// FromKMS 签章管线走 HSM 适配器。signer 失败整单标 failed，旧 token 不能重试成功路径。
  func FromKMS(s kms.Signer) Signer {
	  return func(canonical string) (string, error) {
		  if s == nil {
			   return "", fmt.Errorf("signer unavailable")
		  }
		  b, err := s.SignCanonical(canonical)
		   if err != nil {
			  return "", err
		  }
		   return hex.EncodeToString(b), nil
	  }
   }
