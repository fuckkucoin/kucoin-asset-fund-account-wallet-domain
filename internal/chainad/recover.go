package chainad

import (
	"context"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/pkg/money"
)

type NonceReplay struct {
	Chain string `json:"chain"`
	From  string `json:"from"`
	Nonce string `json:"nonce"`
	Hash     string `json:"hash"`
	Replayed bool   `json:"replayed"`
}

// RecoverNonce EVM 同一 nonce 必须回放原哈希。二次 Submit 不得另造一笔。
   func RecoverNonce(ctx context.Context, sub Submitter, req Outbound) (NonceReplay, error) {
	  if sub == nil {
		  return NonceReplay{}, ErrUnavailable
	  }
	  if strings.TrimSpace(req.Nonce) == "" {
		   return NonceReplay{}, ErrNonce
	  }
	   rec,
		  err := SubmitForWithdraw(ctx, sub, req)
	  if err != nil {
		  return NonceReplay{}, err
	  }
	   again, err := SubmitForWithdraw(ctx, sub, req)
	  if err != nil {
		  return NonceReplay{}, err
	  }
	   if again.Hash != rec.Hash {
		   return NonceReplay{}, fmt.Errorf("%w: hash changed on replay", ErrNonce)
	   }
	   return NonceReplay{
		   Chain: req.Chain, From: req.From, Nonce: req.Nonce,
		   Hash: rec.Hash,
		  Replayed: again.Replayed || again.Hash == rec.Hash,
	   }, nil
  }

type RBFRecover struct {
	Chain   string `json:"chain"`
	OldHash string `json:"old_hash"`
	NewHash string `json:"new_hash"`
	OldFee  string `json:"old_fee"`
	NewFee  string `json:"new_fee"`
	Allow bool `json:"allow"`
	Reason string `json:"reason,omitempty"`
}

// RecoverRBF UTXO 家族才允许替换。新费必须高于旧费，替换哈希不得等于旧哈希。
  func RecoverRBF(chain, oldHash, oldFee, newFee, newHash string) (RBFRecover, error) {
	   out := RBFRecover{Chain: strings.ToUpper(strings.TrimSpace(chain)),
		   OldHash: oldHash, NewHash: newHash, OldFee: oldFee, NewFee: newFee}
	   fam, err := FamilyOf(chain)
	   if err != nil {
		   return out, err
	  }
	   if fam != FamBTC {
		   out.Reason = "rbf only on utxo family"
		  return out, nil
	  }
	  if strings.TrimSpace(oldHash) == "" || strings.EqualFold(strings.TrimSpace(oldHash), strings.TrimSpace(newHash)) {
		   out.Reason = "replacement hash matches old"
		  return out, nil
	  }
	  oldA, err := money.ParseBook(orZero(oldFee))
	  if err != nil {
		  return out, err
	  }
	  newA, err := money.Parse(newFee)
	  if err != nil {
		   return out, err
	  }
	  min, _ := money.Parse("0.0001")
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

   func orZero(s string) string {
	   if strings.TrimSpace(s) == "" {
		  return "0"
	   }
	   return s
  }
