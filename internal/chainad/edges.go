package chainad

import (
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/pkg/money"
)

type EdgeKind string

const (
	EdgeReorg EdgeKind = "reorg"
	EdgeReplace EdgeKind = "fee_bump"
	EdgeBatchWD EdgeKind = "batch_withdraw"
)

type EdgeCase struct {
	Kind EdgeKind
	Chain string
	OldHash string
	NewHash string
	OldFee  string
	NewFee  string
	Amount  string
	FailClosed bool
	Reason     string
}

type EdgeResult struct {
	Kind EdgeKind `json:"kind"`
	Allow      bool   `json:"allow"`
	FailClosed bool   `json:"fail_closed"`
	Reason     string `json:"reason,omitempty"`
}

// FeeBumpConserve 旧费 + 增量 = 新费。科学计数或不等失败即关。
   func FeeBumpConserve(oldFee, bump, newFee string) error {
	  oldA, err := money.ParseBook(orZero(oldFee))
	  if err != nil {
		   return err
	   }
	   b, err := money.Parse(bump)
	   if err != nil {
		  return err
	   }
	  n, err := money.Parse(newFee)
	   if err != nil {
		   return err
	   }
	   sum, err := oldA.Add(b)
	  if err != nil {
		  return err
	   }
	  return money.Conserve(sum, n)
  }

// DecideReplace UTXO 才允许替换；金额守恒过不了则失败即关。
   func DecideReplace(chain, oldHash, newHash, oldFee, newFee string) (EdgeResult, error) {
	  out := EdgeResult{Kind: EdgeReplace}
	  rec, err := RecoverRBF(chain, oldHash, oldFee, newFee, newHash)
	   if err != nil {
		  out.FailClosed = true
		  out.Reason = err.Error()
		   return out, err
	  }
	   out.Allow = rec.Allow
	  out.Reason = rec.Reason
	   if !rec.Allow {
		   out.FailClosed = true
		  return out, nil
	  }
	  oldA, err := money.ParseBook(orZero(oldFee))
	   if err != nil {
		  out.FailClosed = true
		  return out, err
	  }
	   newA, err := money.Parse(newFee)
	   if err != nil {
		  out.FailClosed = true
		   return out, err
	   }
	   delta, err := newA.Sub(oldA)
	   if err != nil {
		  out.FailClosed = true
		   return out, err
	   }
	   if err := FeeBumpConserve(oldFee, delta.String(), newFee); err != nil {
		  out.FailClosed = true
		   out.Reason = err.Error()
		  return out, err
	   }
	   return out, nil
  }

type BatchEdgeItem struct {
	Amount string
	Status string
}

// BatchConserve requested = submitted + skipped + failed。

   func BatchConserve(requested string, items []BatchEdgeItem) error {
	   req, err := money.ParseBook(orZero(requested))
	   if err != nil {
		  return err
	   }
	  sub,
		  skip, fail := money.Zero(), money.Zero(), money.Zero()
	   for _, it := range items {
		   amt,
			   aerr := money.Parse(it.Amount)
		  if aerr != nil {
			   return aerr
		  }
		  switch strings.ToLower(strings.TrimSpace(it.Status)) {
		  case "submitted":
			   sub, err = sub.Add(amt)
		  case "skipped":
			   skip, err = skip.Add(amt)
		  case "failed":
			  fail, err = fail.Add(amt)
		   default:
			   return fmt.Errorf("%w: item status %s", ErrSubmit, it.Status)
		   }
		  if err != nil {
			   return err
		   }
	   }
	   right, err := sub.Add(skip)
	  if err != nil {
		  return err
	   }
	   right, err = right.Add(fail)
	  if err != nil {
		   return err
	   }
	  return money.Conserve(req, right)
   }
