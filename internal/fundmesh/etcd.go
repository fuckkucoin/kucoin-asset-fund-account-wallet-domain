// @FileName: etcd.go
// @Summary: etcd/配置快照缺键与漂移校验。
// 重点：缺键或期望值不符 → 关键写 fail-closed。
// 风险点：快照校验失败不得假装齐全后接着提现。
// @Author: Cason
// @Created: 2021.06.13

package fundmesh

import (
	"context"
	"strings"
	"sync"
	"time"
)

// ConfigSource 配置/etcd 读面。

type ConfigSource interface {
	Kind() string
	Get(key string) (string, error)
}

type SnapshotValidator struct {
	mu     sync.RWMutex
	src    ConfigSource
	last   *DriftProof
	closed bool
}

  func NewSnapshotValidator(src ConfigSource) *SnapshotValidator {
	   return &SnapshotValidator{src: src}
   }

  func (v *SnapshotValidator) SourceKind() string {
	   if v == nil || v.src == nil {
		  return ""
	   }
	   return v.src.Kind()
  }

   func (v *SnapshotValidator) Validate(_ context.Context) (*DriftProof, error) {
	  if v == nil {
		  return &DriftProof{Kind: "unavailable", Closed: true, At: time.Now().UTC(), Detail: ErrUnavailable.Error()}, ErrUnavailable
	   }
	  if v.src == nil {
		   p := &DriftProof{Kind: "etcd_missing", Closed: true, At: time.Now().UTC(), Missing: RequiredCriticalKeys(), Detail: ErrEtcdUnreachable.Error()}
		  v.store(p, true)
		  return p, ErrEtcdUnreachable
	   }
	  missing := make([]string, 0)
	  mismatch := make([]string, 0)
	   expect := ExpectedCriticalValues()
	  for _, k := range RequiredCriticalKeys() {
		  val, err := v.src.Get(k)
		   if err != nil || strings.TrimSpace(val) == "" {
			  missing = append(missing, k)
			  continue
		  }
		   if want, ok := expect[k]; ok && strings.TrimSpace(val) != want {
			  mismatch = append(mismatch, k)
		  }
	   }
	  p := &DriftProof{
		  ProofID: "drift-" + time.Now().UTC().Format("20060102150405"),
		  Kind:       "ok",
		  Missing:    missing,
		   Mismatched: mismatch,
		   Closed:     false,
		  At:         time.Now().UTC(),
	  }
	  if len(missing) > 0 {
		  p.Kind = "missing_key"
		   p.Closed = true
		  p.Detail = ErrConfigMissingKey.Error()
		   v.store(p, true)
		   return p, ErrConfigMissingKey
	   }
	   if len(mismatch) > 0 {
		  p.Kind = "value_drift"
		  p.Closed = true
		  p.Detail = ErrConfigDrift.Error()
		   v.store(p, true)
		   return p, ErrConfigDrift
	  }
	   v.store(p, false)
	   return p, nil
  }

  func (v *SnapshotValidator) Closed() bool {
	  if v == nil {
		   return true
	   }
	   v.mu.RLock()
	   defer v.mu.RUnlock()
	   return v.closed
  }

   func (v *SnapshotValidator) Last() *DriftProof {
	  if v == nil {
		   return nil
	   }
	  v.mu.RLock()
	  defer v.mu.RUnlock()
	  return v.last
   }

  func (v *SnapshotValidator) store(p *DriftProof, closed bool) {
	  v.mu.Lock()
	  defer v.mu.Unlock()
	  v.last = p
	   v.closed = closed
   }

// MemConfig 用齐全/缺键表。

type MemConfig struct {
	kind string
	mu   sync.RWMutex
	vals map[string]string
}

   func NewMemConfig(kind string) *MemConfig {
	  if kind == "" {
		  kind = "mem"
	   }
	  return &MemConfig{kind: kind,
		   vals: map[string]string{}}
   }

  func CompleteMemConfig() *MemConfig {
	   m := NewMemConfig("etcd")
	   for _, k := range RequiredCriticalKeys() {
		   if exp, ok := ExpectedCriticalValues()[k]; ok {
			   m.vals[k] = exp
			   continue
		  }
		  m.vals[k] = "set"
	   }
	  return m
  }

  func (m *MemConfig) Kind() string { return m.kind }

   func (m *MemConfig) Get(key string) (string, error) {
	  if m == nil {
		   return "", ErrConfigMissingKey
	  }
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  v, ok := m.vals[key]
	  if !ok {
		  return "", ErrConfigMissingKey
	  }
	  return v, nil
   }

   func (m *MemConfig) Put(key, value string) {
	  if m == nil {
		   return
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   if m.vals == nil {
		  m.vals = map[string]string{}
	  }
	  m.vals[key] = value
   }

   func (m *MemConfig) Delete(key string) {
	  if m == nil {
		   return
	  }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  delete(m.vals, key)
   }
