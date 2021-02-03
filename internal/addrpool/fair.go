package addrpool

import (
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/validate"
)

// PickFairChain 从候选链中挑已分配最少的一条。

   func PickFairChain(pool *Pool, userID string, chains []string) (string, error) {
	   uid, err := validate.UserID(userID)
	   if err != nil {
		   return "", err
	  }
	   cands := make([]string, 0, len(chains))
	  seen := map[string]struct{}{}
	  for _, ch := range chains {
		   got, err := validate.Chain(ch)
		  if err != nil {
			  continue
		   }
		  if _, ok := seen[got]; ok {
			   continue
		   }
		  seen[got] = struct{}{}
		   cands = append(cands, got)
	   }
	   if len(cands) == 0 {
		   return "", fmt.Errorf("assign fair chains required")
	   }
	  counts := map[string]int{}
	   if pool != nil {
		  for _, b := range pool.List(uid) {
			  if b.Allocated {
				  counts[b.Chain]++
				  if b.Family != "" {
					  counts[b.Family]++
				  }
			   }
		   }
	  }
	  best := cands[0]
	   bestN := counts[best]
	  for _, ch := range cands[1:] {
		   n := counts[ch]
		   if n < bestN {
			   best, bestN = ch, n
		   }
	  }
	   return best, nil
   }

// AssignFair 多链公平领址。chains 空则走 req.Chain。
   func AssignFair(pool *Pool, shared *SharedMemoPool, binder MemoBinder, req AssignReq, chains []string) (AssignedView, error) {
	  if strings.TrimSpace(req.Chain) != "" && len(chains) == 0 {
		  return Assign(pool, shared, binder, req)
	  }
	   if len(chains) == 0 && strings.TrimSpace(req.Chain) == "" {
		  return AssignedView{}, fmt.Errorf("assign fair chains required")
	   }
	   if strings.TrimSpace(req.Chain) != "" {
		  chains = append([]string{req.Chain}, chains...)
	   }
	  picked, err := PickFairChain(pool, req.UserID, chains)
	  if err != nil {
		   return AssignedView{}, err
	  }
	  req.Chain = picked
	  return Assign(pool, shared, binder, req)
   }
