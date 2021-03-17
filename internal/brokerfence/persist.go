package brokerfence

import (
	"strings"
	"sync"
	"time"
)

// ──────────────────────────────────────────────
// 内存 GenerationStore
// ──────────────────────────────────────────────
// MemGenerationStore 内存世代仓库（适用于与单机部署）。
type MemGenerationStore struct {
	mu sync.Mutex
	data map[string]*FenceGeneration
}

// NewMemGenerationStore 构造内存世代仓库。
   func NewMemGenerationStore() *MemGenerationStore {
	  return &MemGenerationStore{data: make(map[string]*FenceGeneration)}
  }

   func (s *MemGenerationStore) GetGeneration(subID string) (FenceGeneration, error) {
	   subID = strings.TrimSpace(subID)
	   s.mu.Lock()
	  g, ok := s.data[subID]
	  s.mu.Unlock()
	   if !ok {
		   // 返回零值世代（初始状态）。
		   return FenceGeneration{SubID: subID,
			   Generation: 0, Fenced: false, UpdatedAt: time.Now().UTC()}, nil
	  }
	  return *g, nil
   }

// BumpGeneration 原子 CAS：仅当 expectGen 匹配时将世代+1。
   func (s *MemGenerationStore) BumpGeneration(subID string, expectGen int64, fenced bool, now time.Time) (FenceGeneration, error) {
	  subID = strings.TrimSpace(subID)
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  cur, ok := s.data[subID]
	  if !ok {
		  if expectGen != 0 {
			  return FenceGeneration{}, ErrGenerationMismatch
		   }
		  cur = &FenceGeneration{SubID: subID, Generation: 0, Fenced: false}
	  }
	   if cur.Generation != expectGen {
		  return FenceGeneration{}, ErrGenerationMismatch
	   }
	  cur.Generation++
	   cur.Fenced = fenced
	   cur.UpdatedAt = now
	   if fenced {
		  cur.FencedSince = &now
	  } else {
		   cur.FencedSince = nil
	   }
	   s.data[subID] = cur
	   return *cur, nil
   }

  func (s *MemGenerationStore) ListFencedSubs() ([]string, error) {
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   var out []string
	   for _, g := range s.data {
		  if g.Fenced {
			  out = append(out, g.SubID)
		  }
	   }
	   return out, nil
  }

// ──────────────────────────────────────────────
// 内存 LeaseStore
// ──────────────────────────────────────────────
// LeaseStore 围栏租约持久化接口。

type LeaseStore interface {
	SaveLease(l *FenceLease) error
	GetLease(id string) (*FenceLease,
		error)
	GetActiveLeaseForSub(subID string) (*FenceLease, error)
	UpdateLease(l *FenceLease) error
	ListLeases(subID string) ([]*FenceLease,
		error)
}

// MemLeaseStore 内存租约仓库。
type MemLeaseStore struct {
	mu   sync.Mutex
	data map[string]*FenceLease
}

// NewMemLeaseStore 构造内存租约仓库。
  func NewMemLeaseStore() *MemLeaseStore {
	  return &MemLeaseStore{data: make(map[string]*FenceLease)}
   }

  func (s *MemLeaseStore) SaveLease(l *FenceLease) error {
	   if l == nil || strings.TrimSpace(l.ID) == "" {
		  return ErrInput
	   }
	   s.mu.Lock()
	  cp := *l
	  s.data[l.ID] = &cp
	  s.mu.Unlock()
	  return nil
   }

  func (s *MemLeaseStore) GetLease(id string) (*FenceLease, error) {
	  id = strings.TrimSpace(id)
	   s.mu.Lock()
	   l, ok := s.data[id]
	   s.mu.Unlock()
	  if !ok {
		   return nil, ErrNotFound
	  }
	   cp := *l
	   return &cp, nil
   }

  func (s *MemLeaseStore) GetActiveLeaseForSub(subID string) (*FenceLease, error) {
	   subID = strings.TrimSpace(subID)
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   for _, l := range s.data {
		   if l.SubID == subID && LeaseIsWriteBlocking(l.State) {
			   cp := *l
			   return &cp, nil
		   }
	   }
	  return nil, ErrNotFound
  }

   func (s *MemLeaseStore) UpdateLease(l *FenceLease) error {
	  if l == nil || strings.TrimSpace(l.ID) == "" {
		   return ErrInput
	  }
	   s.mu.Lock()
	  _, ok := s.data[l.ID]
	   if !ok {
		  s.mu.Unlock()
		   return ErrNotFound
	   }
	  cp := *l
	  s.data[l.ID] = &cp
	  s.mu.Unlock()
	  return nil
  }

  func (s *MemLeaseStore) ListLeases(subID string) ([]*FenceLease, error) {
	   subID = strings.TrimSpace(subID)
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   var out []*FenceLease
	   for _, l := range s.data {
		   if subID == "" || l.SubID == subID {
			  cp := *l
			  out = append(out, &cp)
		  }
	   }
	   return out, nil
   }

// ──────────────────────────────────────────────
// 内存 BypassStore
// ──────────────────────────────────────────────
// MemBypassStore 内存旁路案件仓库。
type MemBypassStore struct {
	mu sync.Mutex
	data map[string]*BypassCase
}

// NewMemBypassStore 构造内存旁路仓库。
  func NewMemBypassStore() *MemBypassStore {
	  return &MemBypassStore{data: make(map[string]*BypassCase)}
   }

   func (s *MemBypassStore) SaveBypassCase(bc *BypassCase) error {
	   if bc == nil || strings.TrimSpace(bc.ID) == "" {
		  return ErrInput
	   }
	  s.mu.Lock()
	  cp := *bc
	   s.data[bc.ID] = &cp
	  s.mu.Unlock()
	  return nil
  }

   func (s *MemBypassStore) GetBypassCase(id string) (*BypassCase, error) {
	   id = strings.TrimSpace(id)
	   s.mu.Lock()
	  bc, ok := s.data[id]
	   s.mu.Unlock()
	  if !ok {
		  return nil, ErrNotFound
	  }
	  cp := *bc
	  return &cp, nil
   }

  func (s *MemBypassStore) ListBypassCases(subID string, state BypassState) ([]*BypassCase, error) {
	   subID = strings.TrimSpace(subID)
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  var out []*BypassCase
	  for _, bc := range s.data {
		   if (subID == "" || bc.SubID == subID) && bc.State == state {
			   cp := *bc
			  out = append(out, &cp)
		   }
	  }
	   return out, nil
   }

   func (s *MemBypassStore) UpdateBypassCase(bc *BypassCase) error {
	  if bc == nil || strings.TrimSpace(bc.ID) == "" {
		   return ErrInput
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   _, ok := s.data[bc.ID]
	  if !ok {
		   return ErrNotFound
	  }
	  cp := *bc
	   s.data[bc.ID] = &cp
	   return nil
   }

// ──────────────────────────────────────────────
// 内存 ConserveStore
// ──────────────────────────────────────────────
// MemConserveStore 内存守恒证明仓库。
type MemConserveStore struct {
	mu      sync.Mutex
	data    map[string]*ConserveProof   // key=id
	byLease map[string][]*ConserveProof // key=leaseID
}

// NewMemConserveStore 构造内存守恒证明仓库。

  func NewMemConserveStore() *MemConserveStore {
	  return &MemConserveStore{
		   data:    make(map[string]*ConserveProof),
		  byLease: make(map[string][]*ConserveProof),
	  }
  }

   func (s *MemConserveStore) SaveProof(p *ConserveProof) error {
	  if p == nil || strings.TrimSpace(p.ID) == "" {
		   return ErrInput
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  cp := *p
	  s.data[p.ID] = &cp
	  // 更新 byLease 索引（若已存在则替换）。
	  list := s.byLease[p.LeaseID]
	  replaced := false
	   for i, existing := range list {
		  if existing.ID == p.ID {
			  list[i] = &cp
			   replaced = true
			  break
		  }
	  }
	   if !replaced {
		  s.byLease[p.LeaseID] = append(list, &cp)
	   }
	   return nil
   }

  func (s *MemConserveStore) GetProof(id string) (*ConserveProof, error) {
	   id = strings.TrimSpace(id)
	   s.mu.Lock()
	   p, ok := s.data[id]
	   s.mu.Unlock()
	   if !ok {
		  return nil, ErrNotFound
	  }
	  cp := *p
	   return &cp, nil
  }

  func (s *MemConserveStore) LatestProof(leaseID string) (*ConserveProof, error) {
	  leaseID = strings.TrimSpace(leaseID)
	   s.mu.Lock()
	   list := s.byLease[leaseID]
	  s.mu.Unlock()
	   if len(list) == 0 {
		   return nil, ErrNotFound
	  }
	  latest := list[0]
	   for _, p := range list[1:] {
		  if p.SampledAt.After(latest.SampledAt) {
			  latest = p
		   }
	   }
	   cp := *latest
	   return &cp, nil
   }

  func (s *MemConserveStore) ListProofs(leaseID string) ([]*ConserveProof, error) {
	   leaseID = strings.TrimSpace(leaseID)
	  s.mu.Lock()
	   list := s.byLease[leaseID]
	  s.mu.Unlock()
	  out := make([]*ConserveProof, len(list))
	  for i, p := range list {
		   cp := *p
		   out[i] = &cp
	  }
	  return out, nil
  }
