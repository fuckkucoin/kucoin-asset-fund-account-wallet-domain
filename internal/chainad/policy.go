package chainad

import (
	"fmt"
	"strings"
	"sync"
	"account-assets.kucoin.net/funding/internal/validate"
)

type ConfirmPolicy struct {
	Chain string `json:"chain"`
	Asset string `json:"asset"`
	Need  int    `json:"need"`
	ReorgDepth int `json:"reorg_depth"`
}

type Policies struct {
	mu sync.Mutex
	rows map[string]ConfirmPolicy
}

   func NewPolicies() *Policies {
	  p := &Policies{rows: map[string]ConfirmPolicy{}}
	   _ = p.Put(ConfirmPolicy{Chain: "ETH", Asset: "USDT", Need: 12, ReorgDepth: 12})
	  _ = p.Put(ConfirmPolicy{Chain: "ETH", Asset: "USDC", Need: 12, ReorgDepth: 12})
	  _ = p.Put(ConfirmPolicy{Chain: "BTC", Asset: "BTC", Need: 3, ReorgDepth: 6})
	  _ = p.Put(ConfirmPolicy{Chain: "TRON", Asset: "USDT", Need: 19, ReorgDepth: 19})
	   _ = p.Put(ConfirmPolicy{Chain: "SOL", Asset: "USDT", Need: 32, ReorgDepth: 32})
	  _ = p.Put(ConfirmPolicy{Chain: "XLM", Asset: "XLM", Need: 1, ReorgDepth: 1})
	   _ = p.Put(ConfirmPolicy{Chain: "XRP", Asset: "XRP", Need: 1, ReorgDepth: 1})
	  _ = p.Put(ConfirmPolicy{Chain: "ATOM", Asset: "ATOM", Need: 20, ReorgDepth: 20})
	   _ = p.Put(ConfirmPolicy{Chain: "TON", Asset: "TON", Need: 16, ReorgDepth: 16})
	   _ = p.Put(ConfirmPolicy{Chain: "EOS", Asset: "EOS", Need: 1, ReorgDepth: 1})
	  _ = p.Put(ConfirmPolicy{Chain: "POLYGON", Asset: "USDT", Need: 64, ReorgDepth: 64})
	   _ = p.Put(ConfirmPolicy{Chain: "ARBITRUM", Asset: "USDT", Need: 20, ReorgDepth: 20})
	  _ = p.Put(ConfirmPolicy{Chain: "OPTIMISM", Asset: "USDT", Need: 20, ReorgDepth: 20})
	  _ = p.Put(ConfirmPolicy{Chain: "BASE", Asset: "USDT", Need: 20, ReorgDepth: 20})
	   _ = p.Put(ConfirmPolicy{Chain: "AVAX", Asset: "USDT", Need: 12, ReorgDepth: 12})
	  _ = p.Put(ConfirmPolicy{Chain: "BSC", Asset: "USDT", Need: 15, ReorgDepth: 15})
	  _ = p.Put(ConfirmPolicy{Chain: "LINEA", Asset: "ETH", Need: 20, ReorgDepth: 20})
	   _ = p.Put(ConfirmPolicy{Chain: "SCROLL", Asset: "ETH", Need: 20, ReorgDepth: 20})
	  _ = p.Put(ConfirmPolicy{Chain: "BLAST", Asset: "ETH", Need: 20, ReorgDepth: 20})
	   _ = p.Put(ConfirmPolicy{Chain: "MANTLE", Asset: "ETH", Need: 20, ReorgDepth: 20})
	   _ = p.Put(ConfirmPolicy{Chain: "FTM", Asset: "USDT", Need: 12, ReorgDepth: 12})
	  _ = p.Put(ConfirmPolicy{Chain: "ZKSYNC", Asset: "ETH", Need: 20, ReorgDepth: 20})
	   _ = p.Put(ConfirmPolicy{Chain: "GNOSIS", Asset: "USDT", Need: 12, ReorgDepth: 12})
	  _ = p.Put(ConfirmPolicy{Chain: "CELO", Asset: "USDT", Need: 12, ReorgDepth: 12})
	  _ = p.Put(ConfirmPolicy{Chain: "MOONBEAM", Asset: "GLMR", Need: 12, ReorgDepth: 12})
	  _ = p.Put(ConfirmPolicy{Chain: "OPBNB", Asset: "BNB", Need: 15, ReorgDepth: 15})
	   _ = p.Put(ConfirmPolicy{Chain: "TAIKO", Asset: "ETH", Need: 20, ReorgDepth: 20})
	  _ = p.Put(ConfirmPolicy{Chain: "CRONOS", Asset: "CRO", Need: 15, ReorgDepth: 15})
	  _ = p.Put(ConfirmPolicy{Chain: "UNICHAIN", Asset: "ETH", Need: 20, ReorgDepth: 20})
	   return p
   }

   func policyKey(chain, asset string) string {
	  return strings.ToUpper(strings.TrimSpace(chain)) + "|" + strings.ToUpper(strings.TrimSpace(asset))
   }

   func (p *Policies) Put(row ConfirmPolicy) error {
	  if p == nil {
		  return ErrUnavailable
	   }
	  ch, err := validate.Chain(row.Chain)
	  if err != nil {
		  return ErrChain
	   }
	   as, err := validate.Asset(row.Asset)
	  if err != nil {
		   return ErrAmount
	   }
	  if row.Need <= 0 || row.ReorgDepth < 0 {
		   return ErrDepth
	  }
	  row.Chain, row.Asset = ch, as
	  p.mu.Lock()
	  p.rows[policyKey(ch, as)] = row
	   p.mu.Unlock()
	  return nil
   }

   func (p *Policies) Need(chain, asset string) int {
	  if p == nil {
		  return 12
	  }
	  p.mu.Lock()
	  row, ok := p.rows[policyKey(chain, asset)]
	  p.mu.Unlock()
	   if ok {
		  return row.Need
	   }
	  fam, err := FamilyOf(chain)
	   if err != nil {
		   if spec, merr := MemoOf(chain); merr == nil {
			   _ = spec
			   return 1
		   }
		  return 12
	  }
	  return ConfirmNeed(fam)
  }

   func (p *Policies) Check(chain, asset string, have int) error {
	  need := p.Need(chain, asset)
	   if have < need {
		  return fmt.Errorf("%w: have=%d need=%d asset=%s", ErrDepth, have, need, asset)
	  }
	   return nil
  }

  func (p *Policies) Get(chain, asset string) (ConfirmPolicy, error) {
	   if p == nil {
		   return ConfirmPolicy{}, ErrUnavailable
	  }
	   p.mu.Lock()
	   defer p.mu.Unlock()
	  row, ok := p.rows[policyKey(chain, asset)]
	  if !ok {
		  return ConfirmPolicy{}, ErrNotFound
	   }
	   return row, nil
  }

  func (p *Policies) List() []ConfirmPolicy {
	  if p == nil {
		  return nil
	   }
	  p.mu.Lock()
	   defer p.mu.Unlock()
	  out := make([]ConfirmPolicy, 0, len(p.rows))
	  for _, r := range p.rows {
		   out = append(out, r)
	  }
	  return out
  }
