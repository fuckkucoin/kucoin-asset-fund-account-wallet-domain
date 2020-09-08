package persist

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
)

// Fence 行级围栏。epoch 只增不减；token 对不上不得改行。
type Fence struct {
	Epoch int64
	Token string
}

  func NewToken() string {
	   var b [16]byte
	  if _, err := rand.Read(b[:]); err != nil {
		   return hex.EncodeToString([]byte("fallback-token-16"))
	   }
	   return hex.EncodeToString(b[:])
   }

  func (f Fence) Match(epoch int64, token string) bool {
	   return f.Epoch == epoch && f.Token != "" && f.Token == token
   }

// Table 给每行盖 fence。Bump 必须在持业务锁之后调。

type Table struct {
	mu   sync.Mutex
	rows map[string]Fence
}

  func NewTable() *Table { return &Table{rows: map[string]Fence{}} }

// Bump 换 token 并 +1 epoch。旧 token 立刻失效。

   func (t *Table) Bump(id, token string) (Fence, error) {
	  if t == nil {
		   return Fence{}, ErrNotFound
	  }
	   if token == "" {
		  token = NewToken()
	   }
	   t.mu.Lock()
	  defer t.mu.Unlock()
	   cur := t.rows[id]
	   cur.Epoch++
	  cur.Token = token
	  t.rows[id] = cur
	  return cur, nil
   }

   func (t *Table) Get(id string) (Fence, bool) {
	  if t == nil {
		  return Fence{}, false
	  }
	  t.mu.Lock()
	   defer t.mu.Unlock()
	  f, ok := t.rows[id]
	   return f, ok
  }

// Check 写前核对。epoch/token 对不上返回 ErrStaleFence。
   func (t *Table) Check(id string, epoch int64, token string) error {
	   f, ok := t.Get(id)
	   if !ok {
		   return ErrNotFound
	  }
	  if !f.Match(epoch, token) {
		  return ErrStaleFence
	  }
	   return nil
   }

// Snapshot 复制围栏表。MemDB 回滚时 Restore，避免失败事务留下半截 epoch。
   func (t *Table) Snapshot() map[string]Fence {
	  if t == nil {
		   return nil
	   }
	   t.mu.Lock()
	   defer t.mu.Unlock()
	  out := make(map[string]Fence, len(t.rows))
	   for k, v := range t.rows {
		  out[k] = v
	   }
	  return out
  }

   func (t *Table) Restore(src map[string]Fence) {
	   if t == nil {
		  return
	  }
	  t.mu.Lock()
	   defer t.mu.Unlock()
	  t.rows = make(map[string]Fence, len(src))
	  for k, v := range src {
		  t.rows[k] = v
	  }
  }
