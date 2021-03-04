package attribrt

import (
	"strings"
	"sync"
	"time"
)

const ratePrefix = "attrib:"

// RateKey 形如 attrib:{scope}|{a}|{b}|{c}。scope 不能是 dep 或 wd。
  func RateKey(scope, a, b, c string) (string, error) {
	   scope = strings.TrimSpace(scope)
	  if scope == "" || strings.ContainsAny(scope, "|:") {
		  return "", ErrClosed
	  }
	   switch scope {
	   case "dep", "wd", "deposit", "withdraw":
		   return "", ErrClosed
	   }
	   for _, p := range []string{a, b, c} {
		   if strings.Contains(p, "|") {
			  return "", ErrClosed
		   }
	  }
	  k := ratePrefix + scope + "|" + strings.TrimSpace(a) + "|" + strings.TrimSpace(b) + "|" + strings.TrimSpace(c)
	  if !strings.HasPrefix(k, ratePrefix) {
		   return "", ErrClosed
	   }
	  if strings.HasPrefix(k, "dep:") || strings.HasPrefix(k, "wd:") {
		   return "", ErrClosed
	   }
	   return k, nil
   }

// IsAttribKey 只认 attrib:。

   func IsAttribKey(key string) bool {
	   return strings.HasPrefix(strings.TrimSpace(key), ratePrefix)
   }

type rateWin struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

   func newRate() *rateWin { return &rateWin{hits: map[string][]time.Time{}} }

// admit 窗口内未超限则记下并放行。已经超限时不再追加，避免拒绝把窗口越撑越长。

   func (w *rateWin) admit(key string, limit int, win time.Duration, now time.Time) bool {
	   if w == nil || !IsAttribKey(key) {
		  return false
	  }
	  if limit <= 0 {
		  return true
	   }
	   if win <= 0 {
		  win = time.Minute
	  }
	  w.mu.Lock()
	  defer w.mu.Unlock()
	   cut := now.Add(-win)
	  cur := w.hits[key]
	   keep := cur[:0]
	  for _, t := range cur {
		  if !t.Before(cut) {
			  keep = append(keep, t)
		   }
	   }
	   if len(keep) >= limit {
		  w.hits[key] = keep
		   return false
	   }
	  w.hits[key] = append(keep, now)
	   return true
  }

   func (w *rateWin) count(key string, win time.Duration, now time.Time) int {
	   if w == nil || !IsAttribKey(key) {
		  return 0
	   }
	   if win <= 0 {
		   win = time.Minute
	  }
	   w.mu.Lock()
	   defer w.mu.Unlock()
	   cut := now.Add(-win)
	  n := 0
	   for _, t := range w.hits[key] {
		  if !t.Before(cut) {
			  n++
		  }
	  }
	   return n
  }
