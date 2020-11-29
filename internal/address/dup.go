package address

import (
	"context"
	"sort"
	"strings"
	"account-assets.kucoin.net/funding/internal/validate"
)

// DupHit 跨链重复命中。

type DupHit struct {
	UserID  string   `json:"user_id"`
	Address string   `json:"address"`
	Chains  []string `json:"chains"`
	Count int `json:"count"`
	Disabled bool `json:"has_disabled,omitempty"`
}

// DetectCrossChainDups 按用户检出跨链重复地址。

  func (s *Service) DetectCrossChainDups(ctx context.Context, userID string) ([]DupHit, error) {
	   uid, err := validate.UserID(userID)
	   if err != nil {
		  return nil, err
	   }
	  list, err := s.ListBook(ctx, uid)
	   if err != nil {
		   return nil, err
	  }
	   byAddr := map[string][]string{}
	  disabled := map[string]bool{}
	   for _, e := range list {
		   addr := strings.TrimSpace(e.Address)
		  if addr == "" {
			   continue
		  }
		   ch := strings.ToUpper(strings.TrimSpace(e.Chain))
		   seen := false
		   for _, old := range byAddr[addr] {
			  if old == ch {
				  seen = true
				  break
			  }
		   }
		  if !seen {
			  byAddr[addr] = append(byAddr[addr], ch)
		   }
		  if e.Disabled {
			   disabled[addr] = true
		  }
	  }
	   out := make([]DupHit, 0)
	   for addr, chains := range byAddr {
		  if len(chains) < 2 {
			   continue
		  }
		   sort.Strings(chains)
		   out = append(out, DupHit{
			   UserID: uid, Address: addr, Chains: chains, Count: len(chains),
			  Disabled: disabled[addr],
		  })
	  }
	  sort.Slice(out, func(i, j int) bool {
		   if out[i].Address == out[j].Address {
			   return out[i].Count > out[j].Count
		   }
		  return out[i].Address < out[j].Address
	  })
	   return out, nil
   }
