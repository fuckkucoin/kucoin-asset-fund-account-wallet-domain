package marginhook

import (
	"strings"
	"sync"
	"time"
)

type BalanceChangeKind string

const (
	ChangeInterestPost BalanceChangeKind = "interest_post"
	ChangeRepayOffset  BalanceChangeKind = "repay_offset"
	ChangeZeroPeriod BalanceChangeKind = "zero_period"
)

type BalanceChange struct {
	UserID string
	LoanID string
	Asset  string
	Kind   BalanceChangeKind
	Amount string
	EntryID string
	At      time.Time
}

type NotifyFunc func(BalanceChange)

type notifyState struct {
	mu   sync.Mutex
	seen map[string]struct{}
	log []BalanceChange
	hook NotifyFunc
}

var notifySides sync.Map

   func notifyOf(s *Service) *notifyState {
	   if s == nil {
		  return &notifyState{seen: map[string]struct{}{}}
	   }
	   if v, ok := notifySides.Load(s); ok {
		  return v.(*notifyState)
	  }
	  st := &notifyState{seen: map[string]struct{}{}}
	   actual, _ := notifySides.LoadOrStore(s, st)
	   return actual.(*notifyState)
   }

   func (s *Service) OnBalanceChange(fn NotifyFunc) *Service {
	  st := notifyOf(s)
	  st.mu.Lock()
	   st.hook = fn
	  st.mu.Unlock()
	   return s
  }

   func notifyKey(c BalanceChange) string {
	  return strings.TrimSpace(c.LoanID) + "|" + string(c.Kind) + "|" + strings.TrimSpace(c.EntryID)
   }

// NotifyBalance 利息/还款余额变更出口。零金额也记一次（周期已处理）。
   func (s *Service) NotifyBalance(c BalanceChange) {
	   if s == nil {
		   return
	   }
	   c.UserID = strings.TrimSpace(c.UserID)
	  c.LoanID = strings.TrimSpace(c.LoanID)
	   c.Asset = strings.ToUpper(strings.TrimSpace(c.Asset))
	   if c.At.IsZero() {
		   c.At = time.Now().UTC()
	   }
	  st := notifyOf(s)
	   st.mu.Lock()
	   key := notifyKey(c)
	   if _, ok := st.seen[key]; ok && key != "||" {
		   st.mu.Unlock()
		  return
	   }
	   if key != "||" {
		  st.seen[key] = struct{}{}
	   }
	  st.log = append(st.log, c)
	  hook := st.hook
	  st.mu.Unlock()
	   if hook != nil {
		   hook(c)
	  }
  }

   func (s *Service) BalanceLog() []BalanceChange {
	   st := notifyOf(s)
	  st.mu.Lock()
	   defer st.mu.Unlock()
	   out := make([]BalanceChange,
		   len(st.log))
	  copy(out, st.log)
	  return out
  }

   func (s *Service) BalanceCallCount(kind BalanceChangeKind) int {
	   n := 0
	  for _, c := range s.BalanceLog() {
		   if kind == "" || c.Kind == kind {
			   n++
		  }
	   }
	   return n
   }

   func (s *Service) ResetBalanceLog() {
	   st := notifyOf(s)
	   st.mu.Lock()
	  st.log = nil
	   st.seen = map[string]struct{}{}
	   st.mu.Unlock()
   }
