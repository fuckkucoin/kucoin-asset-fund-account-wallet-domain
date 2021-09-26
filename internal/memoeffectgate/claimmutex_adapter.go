package memoeffectgate

import (
	"context"
	"strings"
	"sync"
)

// MemClaimMutex 输出占用内存实现，语义一致 claimmutex OutputKey/Acquire。
type MemClaimMutex struct {
	mu sync.Mutex
	rows map[string]Occupancy
}

  func NewMemClaimMutex() *MemClaimMutex {
	   return &MemClaimMutex{rows: map[string]Occupancy{}}
   }

  func (m *MemClaimMutex) OutputKey(chain, txHash string, outputIndex int) string {
	   k,
		  err := CanonicalOutputKey(chain, txHash, outputIndex)
	   if err != nil {
		  return ""
	  }
	   return k
  }

   func (m *MemClaimMutex) Acquire(_ context.Context, key, holder, ticketID string) (bool, Occupancy, error) {
	  if m == nil {
		   return false, Occupancy{}, ErrMutexUnavailable
	  }
	  key = strings.TrimSpace(key)
	  if key == "" {
		   return false, Occupancy{}, ErrOutputKeyInvalid
	  }
	   holder = strings.TrimSpace(holder)
	   if holder == "" {
		  holder = HolderMemoApprove
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  if cur, ok := m.rows[key]; ok && cur.Taken {
		  if cur.Holder == holder && (ticketID == "" || cur.TicketID == ticketID) {
			   return true, cur, nil
		  }
		   return false,
			  cur, ErrOutputOccupied
	  }
	   occ := Occupancy{
		  OutputKey: key,
		   Holder:    holder,
		   TicketID:    ticketID,
		   Taken:       true,
		   AutoClaimed: holder == HolderAutoClaim,
	   }
	   m.rows[key] = occ
	   return true, occ, nil
   }

   func (m *MemClaimMutex) Lookup(_ context.Context, key string) (Occupancy, bool, error) {
	  if m == nil {
		   return Occupancy{}, false, ErrMutexUnavailable
	  }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  cur, ok := m.rows[strings.TrimSpace(key)]
	   return cur, ok, nil
  }

// SeedAutoClaim 夹具：同一输出已被自动认领贷记占用。
  func (m *MemClaimMutex) SeedAutoClaim(chain,
	  txHash string, outputIndex int, ticketHint string) (Occupancy, error) {
	  k, err := CanonicalOutputKey(chain, txHash, outputIndex)
	   if err != nil {
		  return Occupancy{}, err
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   occ := Occupancy{
		  OutputKey: k, Holder: HolderAutoClaim, TicketID: ticketHint,
		  Taken: true, AutoClaimed: true,
	  }
	  m.rows[k] = occ
	   return occ, nil
   }

  func (m *MemClaimMutex) Release(key string) {
	   if m == nil {
		   return
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  delete(m.rows, strings.TrimSpace(key))
  }

   func (m *MemClaimMutex) Count() int {
	  if m == nil {
		   return 0
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   return len(m.rows)
   }
