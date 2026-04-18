package daycut

import (
	"sort"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/utabook"
)

type BatchStatus string

const (
	StatusOpen   BatchStatus = "open"
	StatusPosted BatchStatus = "posted"
	StatusFailed BatchStatus = "failed"
)

type BatchRec struct {
	Key BatchKey
	Status   BatchStatus
	Periods  []utabook.PeriodKey
	EntryIDs []string
	AccruedTotal string
	Attempt   int
	UpdatedAt int64
}

type Store interface {
	Save(rec BatchRec) error
	Get(key BatchKey) (BatchRec,
		bool, error)
	List(loanID string) ([]BatchRec, error)
}

type MemStore struct {
	mu   sync.Mutex
	rows map[string]BatchRec
	Down bool
}

  func NewMemStore() *MemStore {
	   return &MemStore{rows: map[string]BatchRec{}}
   }

  func (s *MemStore) Save(rec BatchRec) error {
	  if s == nil || s.Down {
		  return ErrStore
	   }
	   if !rec.Key.Valid() {
		   return ErrBatchKey
	   }
	  if rec.UpdatedAt == 0 {
		   rec.UpdatedAt = time.Now().UTC().UnixMilli()
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  if prev, ok := s.rows[rec.Key.String()]; ok && prev.Status == StatusPosted && rec.Status == StatusOpen {
		  return ErrRollback
	  }
	   s.rows[rec.Key.String()] = rec
	   return nil
   }

   func (s *MemStore) Get(key BatchKey) (BatchRec, bool, error) {
	  if s == nil || s.Down {
		   return BatchRec{}, false, ErrStore
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   rec, ok := s.rows[key.String()]
	  return rec, ok, nil
  }

  func (s *MemStore) List(loanID string) ([]BatchRec, error) {
	   if s == nil || s.Down {
		   return nil, ErrStore
	  }
	   loanID = strings.TrimSpace(loanID)
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  out := make([]BatchRec, 0)
	   for _, rec := range s.rows {
		   if loanID != "" && rec.Key.LoanID != loanID {
			  continue
		   }
		   out = append(out, rec)
	  }
	  sort.Slice(out, func(i, j int) bool {
		  if out[i].Key.CutDate == out[j].Key.CutDate {
			  return out[i].Key.LoanID < out[j].Key.LoanID
		  }
		  return out[i].Key.CutDate < out[j].Key.CutDate
	   })
	   return out, nil
  }
