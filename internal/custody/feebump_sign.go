package custody

import (
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/chainad"
	"account-assets.kucoin.net/funding/internal/kms"
)

// SignFeeBump 抬价/RBF 独立编解码面联签；材料为 CanonicalFeeBump（含 destination_tag）。
// 与主 SignWithdraw 同 HSM；规范串第五段后为 newFee|destination_tag。
// 切流后旧抬价签章不可复验；空备注仍占位，不得臆造。

  func (s *Service) SignFeeBump(transferID, chain, to, amount, newFee, destinationTag string) ([]byte, error) {
	  if s == nil || s.enc == nil {
		   return nil, fmt.Errorf("file-backed KMS unavailable")
	   }
	   canon := chainad.CanonicalFeeBump(transferID, chain, to, amount, newFee, destinationTag)
	   return s.enc.SignCanonical(canon)
  }

// SignFeeBumpWith 经 HSM 适配器签抬价规范串。
  func (s *Service) SignFeeBumpWith(adapter kms.Signer, transferID, chain, to, amount, newFee, destinationTag string) ([]byte, error) {
	   canon := chainad.CanonicalFeeBump(transferID, chain, to, amount, newFee, destinationTag)
	  if adapter == nil {
		   return s.SignFeeBump(transferID, chain, to, amount, newFee, destinationTag)
	   }
	   return adapter.SignCanonical(canon)
  }

// FeeBumpCanonical 暴露抬价签章规范串（ops/气隙核对）。

  func FeeBumpCanonical(transferID, chain, to, amount, newFee, destinationTag string) string {
	   return chainad.CanonicalFeeBump(
		   strings.TrimSpace(transferID),
		   strings.TrimSpace(chain),
		   strings.TrimSpace(to),
		  strings.TrimSpace(amount),
		   strings.TrimSpace(newFee),
		   strings.TrimSpace(destinationTag),
	  )
   }
