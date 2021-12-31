package xfermirror

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrNotFound = errors.New("xfermirror: record not found")
	ErrConflict = errors.New("xfermirror: conflicting record")
	ErrStaleFence = errors.New("xfermirror: version conflict (stale)")
)

// MirrorStore 镜像持久化接口。

type MirrorStore interface {
	// Put 首次写入镜像（hop 不存在则 ErrConflict）。
	Put(ctx context.Context, m MirrorView) error
	// Update 乐观锁更新（version 须与当前记录一致，否则 ErrStaleFence）。
	Update(ctx context.Context,
		m MirrorView) error
	// GetByHopID 通过 hopID 查找镜像。
	GetByHopID(ctx context.Context, hopID string) (MirrorView, error)
	// Get 通过 ID 查找镜像。
	Get(ctx context.Context, id string) (MirrorView, error)
	// List 分页列出镜像。
	List(ctx context.Context, q MirrorQuery) ([]MirrorView, error)
	// PutEvent 写入状态变更审计事件。
	PutEvent(ctx context.Context,
		e MirrorEvent) error
	// ListEvents 查询某镜像的所有事件，按时间升序。
	ListEvents(ctx context.Context, mirrorID string) ([]MirrorEvent, error)
}

// MemMirrorStore 内存实现。
type MemMirrorStore struct {
	mu      sync.RWMutex
	byID    map[string]MirrorView
	byHopID map[string]string // hopID → ID
	events  []MirrorEvent
}

// NewMemMirrorStore 构造内存仓储。
   func NewMemMirrorStore() *MemMirrorStore {
	  return &MemMirrorStore{
		  byID:    map[string]MirrorView{},
		   byHopID: map[string]string{},
	   }
   }

   func (s *MemMirrorStore) Put(_ context.Context, m MirrorView) error {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  if _, ok := s.byHopID[m.HopID]; ok {
		   return ErrConflict
	  }
	   s.byID[m.ID] = m
	  s.byHopID[m.HopID] = m.ID
	   return nil
  }

  func (s *MemMirrorStore) Update(_ context.Context, m MirrorView) error {
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   cur, ok := s.byID[m.ID]
	   if !ok {
		  return ErrNotFound
	  }
	   if cur.Version != m.Version-1 {
		  return ErrStaleFence
	  }
	   m.UpdatedAt = time.Now().UTC()
	  s.byID[m.ID] = m
	  return nil
  }

   func (s *MemMirrorStore) GetByHopID(_ context.Context, hopID string) (MirrorView, error) {
	  s.mu.RLock()
	  defer s.mu.RUnlock()
	  id, ok := s.byHopID[hopID]
	  if !ok {
		  return MirrorView{}, ErrNotFound
	   }
	  m, ok2 := s.byID[id]
	  if !ok2 {
		  return MirrorView{}, ErrNotFound
	  }
	   return m, nil
  }

  func (s *MemMirrorStore) Get(_ context.Context, id string) (MirrorView, error) {
	  s.mu.RLock()
	   defer s.mu.RUnlock()
	  m, ok := s.byID[id]
	  if !ok {
		  return MirrorView{}, ErrNotFound
	   }
	  return m, nil
   }

  func (s *MemMirrorStore) List(_ context.Context, q MirrorQuery) ([]MirrorView, error) {
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	   if q.Limit <= 0 || q.Limit > 500 {
		   q.Limit = 100
	   }
	  asset := strings.ToUpper(strings.TrimSpace(q.Asset))
	  var out []MirrorView
	   for _, m := range s.byID {
		  if q.State != "" && m.State != q.State {
			  continue
		  }
		   if asset != "" && !strings.EqualFold(m.Asset, asset) {
			  continue
		  }
		   if q.UserID != "" && m.InitiatorUserID != q.UserID && m.CounterpartyUserID != q.UserID {
			  continue
		   }
		   out = append(out, m)
	  }
	  sort.Slice(out, func(i, j int) bool {
		  return out[i].CreatedAt.Before(out[j].CreatedAt)
	  })
	  if q.Offset >= len(out) {
		   return nil, nil
	  }
	   out = out[q.Offset:]
	   if len(out) > q.Limit {
		  out = out[:q.Limit]
	   }
	   return out, nil
  }

  func (s *MemMirrorStore) PutEvent(_ context.Context, e MirrorEvent) error {
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   s.events = append(s.events, e)
	  return nil
  }

  func (s *MemMirrorStore) ListEvents(_ context.Context, mirrorID string) ([]MirrorEvent, error) {
	  s.mu.RLock()
	   defer s.mu.RUnlock()
	  var out []MirrorEvent
	   for _, e := range s.events {
		  if e.MirrorID == mirrorID {
			   out = append(out, e)
		  }
	   }
	  sort.Slice(out, func(i, j int) bool {
		  return out[i].At.Before(out[j].At)
	  })
	   return out, nil
  }

// IsNotFound 判断是否未找到错误。

  func IsNotFound(err error) bool {
	  return errors.Is(err, ErrNotFound) || strings.Contains(err.Error(), "not found")
   }
