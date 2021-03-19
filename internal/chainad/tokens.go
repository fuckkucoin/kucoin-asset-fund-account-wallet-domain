package chainad

import (
	"fmt"
	"strings"
	"sync"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
)

type Token struct {
	Chain string `json:"chain"`
	Asset    string `json:"asset"`
	Contract string `json:"contract"`
	Decimals int    `json:"decimals"`
}

type TokenBook struct {
	mu sync.Mutex
	rows map[string]Token
	sql  *persist.SQLDB
}

  func NewTokenBook() *TokenBook {
	   return &TokenBook{rows: map[string]Token{}}
   }

  func tokenKey(chain, asset string) string {
	  return strings.ToUpper(strings.TrimSpace(chain)) + "|" + strings.ToUpper(strings.TrimSpace(asset))
   }

   func (b *TokenBook) Put(t Token) error {
	  if b == nil {
		   return ErrUnavailable
	  }
	   ch, err := validate.Chain(t.Chain)
	   if err != nil {
		  return ErrChain
	  }
	  as, err := validate.Asset(t.Asset)
	   if err != nil {
		   return ErrAmount
	   }
	   t.Contract = strings.TrimSpace(t.Contract)
	   if t.Contract == "" {
		   return ErrAddress
	  }
	  if t.Decimals < 0 || t.Decimals > 18 {
		  return ErrAmount
	   }
	   t.Chain, t.Asset = ch, as
	   b.mu.Lock()
	   b.rows[tokenKey(ch, as)] = t
	   b.mu.Unlock()
	  if err := b.persistToken(t); err != nil {
		  b.mu.Lock()
		  delete(b.rows, tokenKey(ch, as))
		   b.mu.Unlock()
		   return err
	   }
	   return nil
  }

   func (b *TokenBook) Get(chain, asset string) (Token, error) {
	   if b == nil {
		   return Token{}, ErrUnavailable
	   }
	  if got, ok := b.loadToken(chain, asset); ok {
		  return got, nil
	  }
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  t, ok := b.rows[tokenKey(chain, asset)]
	  if !ok {
		  return Token{}, fmt.Errorf("%w: %s %s", ErrNotFound, chain, asset)
	  }
	   return t, nil
   }

   func (b *TokenBook) List() []Token {
	   if b == nil {
		   return nil
	  }
	   if got, ok := b.listTokens(); ok {
		   return got
	   }
	  b.mu.Lock()
	   defer b.mu.Unlock()
	   out := make([]Token, 0, len(b.rows))
	  for _, t := range b.rows {
		   out = append(out, t)
	   }
	  return out
  }
