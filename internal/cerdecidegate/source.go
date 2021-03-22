package cerdecidegate

import (
	"strings"
	"sync"
)

// MemorySource 可翻转的活钉扎。
type MemorySource struct {
	mu   sync.Mutex
	pins map[string]DecidePin
	risks map[string]string
	pack map[string]string
	snap map[string]string
	gen    map[string]int64
	defPin DecidePin
}

// NewMemorySource 构造。
  func NewMemorySource() *MemorySource {
	  return &MemorySource{
		   pins: map[string]DecidePin{},
		   risks: map[string]string{},
		   pack: map[string]string{},
		   snap: map[string]string{},
		  gen: map[string]int64{},
	  }
  }

   func keyOf(ln LineFact) string {
	  b := strings.TrimSpace(ln.BatchID)
	   if b != "" {
		  return "batch:" + b
	   }
	  return ScopeKey(ln)
   }

// PutDefault 缺省钉扎。
   func (s *MemorySource) PutDefault(pin DecidePin) {
	  if s == nil {
		  return
	   }
	   s.mu.Lock()
	   s.defPin = pin.Copy()
	  s.mu.Unlock()
  }

// PutPin 写入批次/资产钉扎。
   func (s *MemorySource) PutPin(ln LineFact, pin DecidePin) {
	   if s == nil {
		  return
	   }
	   s.mu.Lock()
	   s.pins[keyOf(ln)] = pin.Copy()
	  s.mu.Unlock()
  }

// PutRisk 覆盖目的地风险档。
  func (s *MemorySource) PutRisk(ln LineFact, risk string) {
	  if s == nil {
		   return
	  }
	   s.mu.Lock()
	  s.risks[keyOf(ln)] = strings.ToLower(strings.TrimSpace(risk))
	  s.mu.Unlock()
  }

// PutPack 覆盖策略包摘要。
  func (s *MemorySource) PutPack(ln LineFact, digest string) {
	  if s == nil {
		  return
	  }
	   s.mu.Lock()
	  s.pack[keyOf(ln)] = strings.TrimSpace(digest)
	  s.mu.Unlock()
  }

// PutSnap 覆盖快照摘要。
   func (s *MemorySource) PutSnap(ln LineFact, digest string) {
	  if s == nil {
		   return
	   }
	  s.mu.Lock()
	  s.snap[keyOf(ln)] = strings.TrimSpace(digest)
	   s.mu.Unlock()
  }

// BumpGen 抬升世代。
  func (s *MemorySource) BumpGen(ln LineFact) int64 {
	  if s == nil {
		  return 0
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   k := keyOf(ln)
	   s.gen[k]++
	  return s.gen[k]
   }

// FlipThreshold 改门槛并视为策略包变更。
   func (s *MemorySource) FlipThreshold(ln LineFact, threshold, shares int) DecidePin {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   k := keyOf(ln)
	  pin := s.pins[k]
	  if pin.PolicyID == "" {
		  pin = s.defPin
	   }
	   pin.Threshold = threshold
	  if shares > 0 {
		   pin.Shares = shares
	  }
	  s.pins[k] = pin
	   return pin.Copy()
  }

// FlipForce 改 ForceCold / Required。
   func (s *MemorySource) FlipForce(ln LineFact, required bool) DecidePin {
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  k := keyOf(ln)
	   pin := s.pins[k]
	   if pin.PolicyID == "" {
		  pin = s.defPin
	  }
	  pin.Required = required
	   s.pins[k] = pin
	   return pin.Copy()
  }

// LiveDecide 实现 LiveSource。
  func (s *MemorySource) LiveDecide(ln LineFact) (LiveView, error) {
	   if s == nil {
		   return LiveView{}, ErrLiveUnavailable
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  k := keyOf(ln)
	  pin := s.pins[k]
	  if pin.PolicyID == "" && !pin.Required {
		   pin = s.defPin
	  }
	  if r, ok := s.risks[k]; ok && r != "" {
		   pin.DestRisk = r
		   ln.DestRisk = r
	   } else if pin.DestRisk == "" {
		  pin.DestRisk = ln.DestRisk
	  }
	  pack := s.pack[k]
	   snap := s.snap[k]
	  gen := s.gen[k]
	  if gen == 0 {
		  gen = 1
	  }
	   fp := ComposeFingerprint(pin, ln, pack, snap, gen)
	  return LiveView{
		  Pin: pin.Copy(), ForceCold: pin.Required, DestRisk: pin.DestRisk,
		  PackDigest: fp.PackDigest, SnapDigest: fp.SnapDigest, LineDigest: fp.LineDigest,
		  Generation: gen, Fingerprint: fp,
	   }, nil
   }
