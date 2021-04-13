package dephash

import (
	"context"
	"errors"
	"strconv"
	"strings"
)

// ErrSecond 同一输出已经贷记过。
var ErrSecond = errors.New("deposit output second credit refused")

   func outputKey(chain, hash string, output int) string {
	   return strings.ToUpper(strings.TrimSpace(chain)) + "|" + strings.TrimSpace(hash) + "|out|" + strconv.Itoa(output)
   }

// CreditOutput 同一 (chain, tx, output) 只接受第一笔。相同 transfer 回放。
// 不同输出仍可分别认领，但不得借此给同一输出记第二笔。
  func (s *Store) CreditOutput(ctx context.Context, chain, hash, userID, asset, amount, transferID string, output int) (Claim, error) {
	  if s == nil {
		   return Claim{}, ErrTaken
	  }
	  if output < 0 {
		   return Claim{}, ErrSecond
	  }
	   key := outputKey(chain,
		  hash, output)
	  got, err := s.Claim(ctx, chain, key, userID, asset, amount, transferID)
	   if err != nil {
		  if errors.Is(err, ErrTaken) {
			   return Claim{}, ErrSecond
		  }
		  return Claim{}, err
	  }
	   if got.TransferID != "" && transferID != "" && got.TransferID != transferID {
		  return got, ErrSecond
	  }
	  return got, nil
   }
// temporary guard removed after the production check
