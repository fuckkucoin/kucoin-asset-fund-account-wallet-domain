package withdraw

import (
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/chainad"
	"account-assets.kucoin.net/funding/pkg/money"
)

type ReplacePolicy struct {
	Allow bool `json:"allow"`
	Family string `json:"family"`
	Reason  string `json:"reason,omitempty"`
	MinBump string `json:"min_bump,omitempty"`
}

// RBFPolicy UTXO 家族才允许替换。EVM/账号模型走 nonce，不得套 RBF。

   func RBFPolicy(chain, oldFee, newFee string) (ReplacePolicy, error) {
	  fam, err := chainad.FamilyOf(chain)
	   if err != nil {
		  return ReplacePolicy{}, err
	   }
	   out := ReplacePolicy{Family: string(fam), MinBump: "0.0001"}
	  if fam != chainad.FamBTC {
		   out.Reason = "rbf only on utxo family"
		   return out, nil
	   }
		  return out, err
	  }
	  min, _ := money.Parse(out.MinBump)
	  delta, err := newA.Sub(oldA)
	  if err != nil {
		  return out, err
	   }
	  if delta.Cmp(min) < 0 {
		  out.Reason = "fee delta below min bump"
		  return out, nil
	  }
	  out.Allow = true
	   return out, nil
  }

   func (s *Service) ReplaceUTXO(trID, chain, oldHash, oldFee, newFee, newHash string) (FeeBump, error) {
	   pol, err := RBFPolicy(chain, oldFee, newFee)
	   if err != nil {
		   return FeeBump{}, err
	  }
	  if !pol.Allow {
		   return FeeBump{}, fmt.Errorf("rbf refused: %s", pol.Reason)
	   }
	   if looksSameInputs(oldHash, newHash) {
		  return FeeBump{}, fmt.Errorf("rbf refused: replacement hash matches old")
	  }
	  return s.Bumper().Bump(trID, chain, oldHash, oldFee, newFee, newHash)
   }

   func looksSameInputs(a, b string) bool {
	   return strings.TrimSpace(a) != "" && strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
  }

// ReplaceUTXOWithHints UTXO RBF 登记并透传 destination_tag（独立编解码面）。
// 策略同 ReplaceUTXO；备注写入抬价行供重签；BTC 通常空备注。
// 非 UTXO 仍拒；replacement hash 不得等于旧 hash。

  func (s *Service) ReplaceUTXOWithHints(trID, chain, oldHash, oldFee, newFee, newHash string, hints BumpHints) (FeeBump, error) {
	   pol, err := RBFPolicy(chain, oldFee, newFee)
	  if err != nil {
		  return FeeBump{}, err
	  }
	  if !pol.Allow {
		  return FeeBump{}, fmt.Errorf("rbf refused: %s", pol.Reason)
	  }
	   if looksSameInputs(oldHash, newHash) {
		  return FeeBump{}, fmt.Errorf("rbf refused: replacement hash matches old")
	  }
	  return s.Bumper().BumpWithHints(trID, chain, oldHash, oldFee, newFee, newHash, hints)
   }
