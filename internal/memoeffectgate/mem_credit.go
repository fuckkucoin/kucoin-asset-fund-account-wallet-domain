package memoeffectgate

import (
	"context"
	"strings"
	"sync"
	"github.com/google/uuid"
)

type MemCreditPort struct {
	mu sync.Mutex
	bindErr error
	observeErr error
	applyErr error
	applyOnce int
	applyFails int
	observeAmt   string
	applyCount   int
	observeCount int
	reverses int
	applied map[string]string
}

  func NewMemCreditPort() *MemCreditPort {
	   return &MemCreditPort{observeAmt: "1.25", applied: map[string]string{}}
   }

  func (m *MemCreditPort) FailBind(err error) *MemCreditPort {
	  m.bindErr = err
	  return m
   }

  func (m *MemCreditPort) FailObserve(err error) *MemCreditPort {
	   m.observeErr = err
	  return m
  }

  func (m *MemCreditPort) FailApplyTimes(n int, err error) *MemCreditPort {
	   m.applyFails = n
	  m.applyErr = err
	   return m
  }

   func (m *MemCreditPort) SetAmount(a string) *MemCreditPort {
	  m.observeAmt = a
	  return m
   }

   func (m *MemCreditPort) BindMemo(_, _, _ string) error {
	   if m == nil {
		   return ErrCreditUnavailable
	   }
	   return m.bindErr
   }

  func (m *MemCreditPort) ObserveCredit(_ context.Context, _, _, _, _, _, _ string) (CreditSight, error) {
	   if m == nil {
		   return CreditSight{}, ErrCreditUnavailable
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  m.observeCount++
	  if m.observeErr != nil {
		   return CreditSight{}, m.observeErr
	  }
	   return CreditSight{
		  TransferID: uuid.NewString(),
		  Amount:     m.observeAmt,
		   State:      "credited",
		  Credited: true,
	  }, nil
  }

  func (m *MemCreditPort) ApplyInbound(_ context.Context, _, _, _, amount, idem string) error {
	   if m == nil {
		   return ErrCreditUnavailable
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  if prev, ok := m.applied[idem]; ok {
		   _ = prev
		  m.applyCount++
		   return nil
	   }
	   if m.applyFails > 0 && m.applyOnce < m.applyFails {
		   m.applyOnce++
		  return m.applyErr
	  }
	   m.applied[idem] = amount
	  m.applyCount++
	   return nil
  }

  func (m *MemCreditPort) ReverseInbound(_ context.Context,
	   _, _, _, idem string) error {
	  if m == nil {
		  return ErrCreditUnavailable
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   m.reverses++
	  delete(m.applied, strings.TrimPrefix(idem, "memo-reverse:"))
	   return nil
   }

   func (m *MemCreditPort) ApplyCount() int {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  return m.applyCount
  }

  func (m *MemCreditPort) ObserveCount() int {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  return m.observeCount
  }

  func (m *MemCreditPort) ReverseCount() int {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   return m.reverses
  }

type MemReversePort struct {
	mu   sync.Mutex
	n    int
	fail error
}

  func NewMemReversePort() *MemReversePort { return &MemReversePort{} }

   func (m *MemReversePort) Quarantine(_ context.Context, _, _, _, _ string) error {
	  if m == nil {
		  return ErrOpsReverseDenied
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  if m.fail != nil {
		   return m.fail
	  }
	   m.n++
	   return nil
  }

   func (m *MemReversePort) Count() int {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   return m.n
  }
