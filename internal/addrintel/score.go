package addrintel

import (
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/internal/wallet"
)

// Hit 一次地址观察。同一用户反复打同一地址会抬 reuse，不是换金额。
type Hit struct {
	Address string `json:"address"`
	Chain   string `json:"chain"`
	UserID string    `json:"user_id"`
	At     time.Time `json:"at"`
}

type Score struct {
	Address string `json:"address"`
	Chain string `json:"chain"`
	Hits int `json:"hits"`
	UniqueUsers int `json:"unique_users"`
	Reuse       int `json:"reuse_score"`
	LastAt string `json:"last_at,omitempty"`
}

type Book struct {
	mu sync.Mutex
	hits  []Hit
	users map[string]map[string]struct{}
	count map[string]int
	last map[string]time.Time
}

  func NewBook() *Book {
	  return &Book{
		  users: map[string]map[string]struct{}{},
		   count: map[string]int{},
		   last:  map[string]time.Time{},
	  }
  }

   func addrKey(chain, addr string) string {
	   return strings.ToUpper(strings.TrimSpace(chain)) + "|" + strings.ToLower(strings.TrimSpace(addr))
   }

  func (b *Book) Observe(chain, addr, userID string, at time.Time) (Score, error) {
	   if b == nil {
		   return Score{}, ErrUnavailable
	   }
	  ch, err := validate.Chain(chain)
	  if err != nil {
		  return Score{}, ErrChain
	  }
	  addr = strings.TrimSpace(addr)
	   if err := wallet.ValidChainAddress(ch, addr); err != nil {
		   return Score{}, ErrAddress
	  }
	  uid, err := validate.UserID(userID)
	   if err != nil {
		  return Score{}, ErrUser
	   }
	   if at.IsZero() {
		   at = time.Now().UTC()
	   }
	   k := addrKey(ch, addr)
	   b.mu.Lock()
	   defer b.mu.Unlock()
	   b.hits = append(b.hits, Hit{Address: addr, Chain: ch, UserID: uid, At: at})
	  b.count[k]++
	   if b.users[k] == nil {
		   b.users[k] = map[string]struct{}{}
	  }
	   b.users[k][uid] = struct{}{}
	  b.last[k] = at
	   return b.scoreLocked(ch, addr, k), nil
   }

  func (b *Book) ScoreOf(chain, addr string) (Score, error) {
	  if b == nil {
		  return Score{}, ErrUnavailable
	  }
	   ch, err := validate.Chain(chain)
	   if err != nil {
		  return Score{}, ErrChain
	   }
	  k := addrKey(ch, addr)
	  b.mu.Lock()
	  defer b.mu.Unlock()
	  if b.count[k] == 0 {
		  return Score{}, ErrNotFound
	  }
	   return b.scoreLocked(ch,
		   addr, k), nil
  }

   func (b *Book) scoreLocked(chain, addr, k string) Score {
	   n := b.count[k]
	   u := len(b.users[k])
	  reuse := n - u
	   if reuse < 0 {
		   reuse = 0
	   }
	  s := Score{Address: addr, Chain: chain, Hits: n, UniqueUsers: u, Reuse: reuse}
	  if t, ok := b.last[k]; ok {
		   s.LastAt = t.UTC().Format(time.RFC3339Nano)
	   }
	   return s
   }

   func (b *Book) Hits() []Hit {
	   if b == nil {
		   return nil
	  }
	   b.mu.Lock()
	  defer b.mu.Unlock()
	   out := make([]Hit, len(b.hits))
	   copy(out, b.hits)
	   return out
   }
