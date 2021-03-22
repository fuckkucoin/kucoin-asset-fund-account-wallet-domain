package balproofgate

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"strings"
	"time"
)

// LiveObserver 活账只读接口。挂钩 ledger；注入固定视图。

type LiveObserver interface {
	Observe(ctx context.Context, userID, book, asset string) (LiveView, error)
}

// StaticObserver 用固定活账。
type StaticObserver struct {
	View LiveView
	Err  error
}

// Observe 返回注入视图。
   func (s StaticObserver) Observe(_ context.Context, _, _, _ string) (LiveView, error) {
	  if s.Err != nil {
		   return LiveView{}, s.Err
	   }
	   out := s.View
	   if out.At.IsZero() {
		  out.At = time.Now().UTC()
	   }
	   return out, nil
  }

// MutableObserver 可在中签发后贷记/冻结变化。
type MutableObserver struct {
	Avail string
	Frozen     string
	ObserveGen int64
}

// Observe 读取当前可变视图。

   func (m *MutableObserver) Observe(_ context.Context, _, _, _ string) (LiveView, error) {
	  if m == nil {
		  return LiveView{}, ErrObserveUnavailable
	   }
	   return LiveView{
		  Available: m.Avail,
		   Frozen: m.Frozen,
		   ObserveGen: m.ObserveGen,
		  At: time.Now().UTC(),
	   }, nil
  }

// CreditAvail 签发后贷记抬 Available。

  func (m *MutableObserver) CreditAvail(delta string) {
	   m.Avail = addDecimalStrings(m.Avail, delta)
	   m.ObserveGen++
   }

// AdjustFrozen 冻结变化。

  func (m *MutableObserver) AdjustFrozen(next string) {
	   m.Frozen = next
	   m.ObserveGen++
  }

// FuncObserver 函数适配。
type FuncObserver func(ctx context.Context, userID, book, asset string) (LiveView, error)

// Observe 转发。
   func (f FuncObserver) Observe(ctx context.Context, userID, book, asset string) (LiveView, error) {
	   if f == nil {
		  return LiveView{}, ErrObserveUnavailable
	  }
	   return f(ctx, userID, book, asset)
   }

// FingerprintObserveGen 在观察器未给世代时，用余额规范串生成稳定指纹。
// 余额不变则世代不变；贷记/冻结变化后指纹必变，满足漂移发现。
  func FingerprintObserveGen(avail, frozen string) int64 {
	   h := sha256.Sum256([]byte(strings.TrimSpace(avail) + "|" + strings.TrimSpace(frozen)))
	  v := binary.BigEndian.Uint64(h[0:8]) & ((1 << 48) - 1)
	  if v == 0 {
		   return 1
	  }
	   return int64(v)
   }

// addDecimalStrings 仅用于观察器的粗加（整数或一位小数），路径走 money 包。
  func addDecimalStrings(base, delta string) string {
	   base = strings.TrimSpace(base)
	   delta = strings.TrimSpace(delta)
	  if base == "" {
		   base = "0"
	  }
	  if delta == "" {
		   return base
	   }
	   // 先整数路径，避免夹具引入科学计数。
	   bi, ok1 := parseLooseInt(base)
	  di, ok2 := parseLooseInt(delta)
	   if ok1 && ok2 {
		   return itoa64(bi + di)
	  }
	  return base + "+" + delta
  }

   func parseLooseInt(s string) (int64, bool) {
	  if strings.ContainsAny(s, ".eE") {
		   return 0, false
	  }
	   var n int64
	   neg := false
	   if strings.HasPrefix(s, "-") {
		  neg = true
		   s = s[1:]
	  }
	   if s == "" {
		  return 0, false
	   }
	   for _, c := range s {
		   if c < '0' || c > '9' {
			  return 0, false
		   }
		  n = n*10 + int64(c-'0')
	   }
	  if neg {
		  n = -n
	  }
	  return n, true
   }

   func itoa64(n int64) string {
	  if n == 0 {
		   return "0"
	   }
	  neg := n < 0
	  if neg {
		   n = -n
	   }
	   var b [32]byte
	   i := len(b)
	  for n > 0 {
		  i--
		   b[i] = byte('0' + n%10)
		  n /= 10
	  }
	  if neg {
		  i--
		  b[i] = '-'
	   }
	  return string(b[i:])
  }
