package fx

import (
	"fmt"
	"sync"
)

type Rec struct {
	Key    string
	Result string
	Err  string
	Done bool
}

// Memo 进程内幂等。同一 key 第二次返回第一次结果，payload 不一致 409。
type Memo struct {
	mu   sync.Mutex
	rows map[string]Rec
	pay map[string]string
}

   func NewMemo() *Memo {
	  return &Memo{rows: map[string]Rec{}, pay: map[string]string{}}
  }

   func (m *Memo) Begin(key, payload string) (Rec, bool, error) {
	  if m == nil {
		   return Rec{}, false, fmt.Errorf("idempotency memo unavailable")
	   }
	   key = trim(key)
	  if key == "" {
		   return Rec{}, false, fmt.Errorf("idempotency key required")
	  }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  if cur, ok := m.rows[key]; ok {
		  if m.pay[key] != payload {
			  return cur, true, fmt.Errorf("idempotency conflict")
		   }
		  return cur, true, nil
	   }
	  m.rows[key] = Rec{Key: key}
	  m.pay[key] = payload
	  return m.rows[key], false, nil
  }

  func (m *Memo) Succeed(key, result string) {
	  if m == nil {
		   return
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  cur := m.rows[key]
	   cur.Result = result
	   cur.Done = true
	   cur.Err = ""
	  m.rows[key] = cur
   }

  func (m *Memo) Fail(key string, err error) {
	  if m == nil {
		   return
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   cur := m.rows[key]
	   if err != nil {
		  cur.Err = err.Error()
	  }
	  m.rows[key] = cur
  }

  func (m *Memo) Get(key string) (Rec, bool) {
	  if m == nil {
		  return Rec{}, false
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   cur, ok := m.rows[trim(key)]
	   return cur, ok
  }

  func (m *Memo) Forget(key string) {
	   if m == nil {
		  return
	   }
	   key = trim(key)
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   delete(m.rows, key)
	  delete(m.pay, key)
   }

  func (m *Memo) Inflight() int {
	  if m == nil {
		  return 0
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   n := 0
	   for _, r := range m.rows {
		  if !r.Done && r.Err == "" {
			  n++
		   }
	  }
	   return n
  }

// Replay 只回放已成功的结果。未完成或失败返回 error，调用方不得当成功。
  func (m *Memo) Replay(key string) (Rec, error) {
	  rec, ok := m.Get(key)
	  if !ok {
		   return Rec{}, fmt.Errorf("idempotency key not found")
	   }
	  if !rec.Done {
		  return rec, fmt.Errorf("idempotency in flight")
	  }
	   if rec.Err != "" {
		   return rec, fmt.Errorf("%s", rec.Err)
	  }
	  return rec, nil
   }

   func trim(s string) string {
	   for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		   s = s[1:]
	   }
	   for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		  s = s[:len(s)-1]
	  }
	   return s
   }
