package rollout

import (
	"fmt"
	"hash/fnv"
	"strings"
	"sync"
	"account-assets.kucoin.net/funding/internal/policy"
)

type Book struct {
	mu   sync.Mutex
	live int
	percent int
	pins    map[string]int
}

   func New(live int) *Book {
	   if live <= 0 {
		  live = 1
	   }
	  return &Book{live: live, percent: 100, pins: map[string]int{}}
  }

   func (b *Book) Live() int {
	   b.mu.Lock()
	   defer b.mu.Unlock()
	   return b.live
   }

// Activate 把当前代次切到 ver。ver<=0 拒。不改已 Pin 用户。
   func (b *Book) Activate(ver int) error {
	  if ver <= 0 {
		  return fmt.Errorf("policy version required")
	  }
	  b.mu.Lock()
	   b.live = ver
	  b.mu.Unlock()
	  return nil
   }

   func (b *Book) SetPercent(p int) error {
	   if p < 0 || p > 100 {
		   return fmt.Errorf("rollout percent out of [0,100]")
	  }
	   b.mu.Lock()
	   b.percent = p
	   b.mu.Unlock()
	  return nil
   }

  func (b *Book) Pin(userID string, ver int) error {
	  userID = strings.TrimSpace(userID)
	   if userID == "" || ver <= 0 {
		  return fmt.Errorf("pin requires user and version")
	   }
	  b.mu.Lock()
	  b.pins[userID] = ver
	  b.mu.Unlock()
	  return nil
   }

   func (b *Book) Unpin(userID string) {
	   b.mu.Lock()
	  delete(b.pins, strings.TrimSpace(userID))
	  b.mu.Unlock()
  }

// Pick 用户命中顺序：显式 Pin → 灰度百分比 → live。
// 百分比用 user hash，避免同一人来回跳代次。

   func (b *Book) Pick(userID string) int {
	   b.mu.Lock()
	   defer b.mu.Unlock()
	   if v, ok := b.pins[strings.TrimSpace(userID)]; ok {
		  return v
	  }
	   if b.percent >= 100 {
		   return b.live
	   }
	  if b.percent <= 0 {
		   return 1
	   }
	  h := fnv.New32a()
	  _, _ = h.Write([]byte(userID))
	   if int(h.Sum32()%100) < b.percent {
		  return b.live
	  }
	  return 1
  }

   func (b *Book) Pins() map[string]int {
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   out := make(map[string]int, len(b.pins))
	  for k, v := range b.pins {
		  out[k] = v
	   }
	  return out
  }

   func (b *Book) Status() map[string]any {
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   return map[string]any{"live": b.live, "percent": b.percent, "pins": len(b.pins)}
   }

  func DefaultV2() policy.Doc {
	   d := policy.DefaultV1()
	  d.Version = 2
	   d.MaxSingle = "3000"
	  d.DailyWithdraw = "8000"
	  d.MaxVelocity = 12
	  return d
  }
