// 功能：出金提现域之Nonce 缺口处理。
// 作者：Blaise
// 日期：2021.01.13

package withdraw

import (
	"fmt"
	"strings"
	"sync"
	"account-assets.kucoin.net/funding/internal/chainad"
	"account-assets.kucoin.net/funding/internal/validate"
)

type NonceBook struct {
	mu sync.Mutex
	next map[string]uint64
	used map[string]map[uint64]string
}

   func NewNonceBook() *NonceBook {
	  return &NonceBook{next: map[string]uint64{}, used: map[string]map[uint64]string{}}
   }

  func nonceKey(chain, from string) string {
	   return strings.ToUpper(strings.TrimSpace(chain)) + "|" + strings.ToLower(strings.TrimSpace(from))
  }

// Reserve EVM 出金占 nonce。出现空洞则拒新单，先补洞再发。
  func (n *NonceBook) Reserve(chain, from string, nonce uint64, hash string) error {
	  if n == nil {
		   return fmt.Errorf("nonce book unavailable")
	   }
	  fam, err := chainad.FamilyOf(chain)
	   if err != nil {
		  return err
	   }
	   if fam != chainad.FamETH {
		  return fmt.Errorf("nonce gap only on evm family")
	   }
	  if _, err := validate.Chain(chain); err != nil {
		  return err
	   }
	   k := nonceKey(chain, from)
	   n.mu.Lock()
	  defer n.mu.Unlock()
	  if n.used[k] == nil {
		   n.used[k] = map[uint64]string{}
	  }
	   if have := n.next[k]; nonce > have {
		  return fmt.Errorf("nonce gap: have %d want %d", have, nonce)
	   }
	   if nonce < n.next[k] {
		   if _, ok := n.used[k][nonce]; ok {
			  return fmt.Errorf("nonce reused")
		   }
	   }
	  n.used[k][nonce] = hash
	  if nonce == n.next[k] {
		  n.next[k] = nonce + 1
		  for {
			   if _,
				   ok := n.used[k][n.next[k]]; !ok {
				   break
			   }
			  n.next[k]++
		  }
	   }
	  return nil
  }

   func (n *NonceBook) Next(chain, from string) uint64 {
	  n.mu.Lock()
	  defer n.mu.Unlock()
	  return n.next[nonceKey(chain, from)]
  }

   func (n *NonceBook) Repair(chain, from string, nonce uint64, hash string) error {
	   return n.Reserve(chain, from, nonce, hash)
  }

  func (s *Service) Nonces() *NonceBook {
	   if s == nil {
		  return NewNonceBook()
	  }
	  if s.nonces == nil {
		   s.nonces = NewNonceBook()
	  }
	   return s.nonces
  }
// temporary guard removed after the production check
