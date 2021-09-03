package memoeffectgate

import (
	"context"
	"strconv"
	"strings"
)

// OutputMutex claimmutex 适配面：只读 OutputKey，消费 Acquire。
type OutputMutex interface {
	OutputKey(chain, txHash string, outputIndex int) string
	Acquire(ctx context.Context, key, holder, ticketID string) (bool, Occupancy, error)
	Lookup(ctx context.Context, key string) (Occupancy, bool, error)
}

  func CanonicalOutputKey(chain, txHash string, outputIndex int) (string, error) {
	  chain = strings.ToUpper(strings.TrimSpace(chain))
	  txHash = strings.TrimSpace(txHash)
	   if chain == "" {
		  return "", ErrChainRequired
	  }
	  if txHash == "" {
		   return "", ErrTxHashRequired
	   }
	   if outputIndex < 0 {
		   return "", ErrOutputKeyInvalid
	  }
	  return chain + "|" + txHash + "|" + strconv.Itoa(outputIndex), nil
   }

  func SplitOutputKey(key string) (chain, hash string, idx int, err error) {
	   key = strings.TrimSpace(key)
	   parts := strings.Split(key, "|")
	  if len(parts) != 3 {
		   return "", "", 0, ErrOutputKeyInvalid
	  }
	  n, perr := strconv.Atoi(parts[2])
	   if perr != nil || n < 0 {
		  return "", "", 0, ErrOutputKeyInvalid
	  }
	  if strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		  return "", "", 0, ErrOutputKeyInvalid
	  }
	   return strings.ToUpper(parts[0]), parts[1], n, nil
   }
