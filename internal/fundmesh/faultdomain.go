// 功能：资金 mesh 故障域。
// 重点：配置封闭或远端写失败时整域拒写；观察面仍可 ping。
// 风险点：写域与观察域不得共用计数器。

// 作者：Cason
// 日期：2021.06.14

package fundmesh

import (
	"sync"
	"sync/atomic"
	"time"
)

type FaultDomain struct {
	mu        sync.RWMutex
	writeOpen bool
	reason string
	since  time.Time
	writes atomic.Int64
	observes atomic.Int64
}

  func NewFaultDomain() *FaultDomain {
	  return &FaultDomain{writeOpen: true, since: time.Now().UTC()}
  }

  func (d *FaultDomain) CloseWrite(reason string) {
	   if d == nil {
		  return
	   }
	   d.mu.Lock()
	   defer d.mu.Unlock()
	  d.writeOpen = false
	   d.reason = reason
	  d.since = time.Now().UTC()
  }

   func (d *FaultDomain) OpenWrite() {
	  if d == nil {
		   return
	  }
	   d.mu.Lock()
	   defer d.mu.Unlock()
	   d.writeOpen = true
	  d.reason = ""
	   d.since = time.Now().UTC()
  }

   func (d *FaultDomain) WriteOpen() bool {
	   if d == nil {
		   return false
	   }
	   d.mu.RLock()
	  defer d.mu.RUnlock()
	  return d.writeOpen
   }

  func (d *FaultDomain) Reason() string {
	   if d == nil {
		  return ErrFaultDomainClosed.Error()
	  }
	   d.mu.RLock()
	   defer d.mu.RUnlock()
	   return d.reason
   }

  func (d *FaultDomain) AllowCriticalWrite() error {
	  if d == nil || !d.WriteOpen() {
		  return ErrFaultDomainClosed
	  }
	  return nil
  }

   func (d *FaultDomain) NoteWrite() {
	   if d != nil {
		  d.writes.Add(1)
	   }
   }

  func (d *FaultDomain) NoteObserve() {
	  if d != nil {
		   d.observes.Add(1)
	   }
  }

  func (d *FaultDomain) WriteCount() int64 {
	   if d == nil {
		   return 0
	  }
	   return d.writes.Load()
  }

   func (d *FaultDomain) ObserveCount() int64 {
	  if d == nil {
		  return 0
	   }
	  return d.observes.Load()
  }
