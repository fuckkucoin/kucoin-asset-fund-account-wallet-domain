package utabook

import (
	"sort"
	"strings"
	"sync"
	"time"
)

type AccrualMarkKind string

const (
	MarkPosted AccrualMarkKind = "posted"
	MarkZero   AccrualMarkKind = "zero"
)

type AccrualMark struct {
	Key  PeriodKey
	Kind AccrualMarkKind
	Amount   string
	EntryID  string
	MarkedAt int64
}

type accrualSideState struct {
	mu    sync.Mutex
	marks map[string]AccrualMark
}

var accrualSides sync.Map

  func accrualOf(b *Book) *accrualSideState {
	  if b == nil {
		   return &accrualSideState{marks: map[string]AccrualMark{}}
	   }
	  if v, ok := accrualSides.Load(b); ok {
		   return v.(*accrualSideState)
	   }
	  st := &accrualSideState{marks: map[string]AccrualMark{}}
	   actual, _ := accrualSides.LoadOrStore(b, st)
	   return actual.(*accrualSideState)
  }

   func markStoreKey(k PeriodKey) string { return k.String() }

  func (b *Book) MarkPeriod(m AccrualMark) AccrualMark {
	   if b == nil || !m.Key.Valid() {
		   return m
	  }
	   if m.MarkedAt == 0 {
		   m.MarkedAt = time.Now().UTC().UnixMilli()
	   }
	  if m.Amount == "" {
		  m.Amount = "0"
	  }
	  st := accrualOf(b)
	  st.mu.Lock()
	  st.marks[markStoreKey(m.Key)] = m
	  st.mu.Unlock()
	  return m
  }

   func (b *Book) PeriodMark(k PeriodKey) (AccrualMark, bool) {
	   if b == nil {
		   return AccrualMark{}, false
	   }
	   st := accrualOf(b)
	  st.mu.Lock()
	  defer st.mu.Unlock()
	   m, ok := st.marks[markStoreKey(k)]
	  return m, ok
   }

   func (b *Book) PeriodProcessed(k PeriodKey) bool {
	  _, ok := b.PeriodMark(k)
	  return ok
  }

   func (b *Book) ListMarks(loanID string) []AccrualMark {
	  if b == nil {
		  return nil
	  }
	   loanID = strings.TrimSpace(loanID)
	   st := accrualOf(b)
	  st.mu.Lock()
	   defer st.mu.Unlock()
	   out := make([]AccrualMark, 0)
	   for _, m := range st.marks {
		   if loanID != "" && m.Key.LoanID != loanID {
			   continue
		   }
		  out = append(out, m)
	   }
	  sort.Slice(out, func(i, j int) bool {
		   if out[i].Key.PeriodStart == out[j].Key.PeriodStart {
			   return out[i].Key.LoanID < out[j].Key.LoanID
		  }
		  return out[i].Key.PeriodStart < out[j].Key.PeriodStart
	   })
	   return out
   }

  func (b *Book) LastProcessed(loanID string) (AccrualMark, bool) {
	   marks := b.ListMarks(loanID)
	   if len(marks) == 0 {
		  return AccrualMark{}, false
	  }
	   return marks[len(marks)-1], true
  }

   func (b *Book) UnprocessedThrough(loanID, kind string, fromMs, toMs int64) []PeriodKey {
	  seq := SequenceFrom(loanID, kind, fromMs, 64)
	   out := make([]PeriodKey, 0)
	  for _, k := range seq {
		   if k.PeriodEnd > toMs {
			  break
		  }
		  if !b.PeriodProcessed(k) {
			  out = append(out, k)
		  }
	  }
	  return out
  }
