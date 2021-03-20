package clearing

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
)

// ErrExceptLinkExists 同一异常与差异单已存在关联。

var ErrExceptLinkExists = fmt.Errorf("except-link already exists for this exception and diff")

// ErrExceptLinkNotFound 关联记录不存在。
var ErrExceptLinkNotFound = fmt.Errorf("except-link not found")

// ExceptLink 清算异常与 recon 差异单只读关联记录。

type ExceptLink struct {
	ID string `json:"id"`
	ExceptionID string `json:"exception_id"` // clearing.Exception.ID
	ReconDiffID string `json:"recon_diff_id"`  // recon 差异单号（只读引用，不调分类器）
	Note        string `json:"note,omitempty"` // 关联说明
	LinkedBy string    `json:"linked_by"`
	LinkedAt time.Time `json:"linked_at"`
}

// ExceptLinkStore 异常关联存储，单进程内存实现。
type ExceptLinkStore struct {
	mu sync.Mutex
	links   map[string]*ExceptLink // key: ID
	byExcep map[string][]string    // key: exception_id → []link_id
	seen    map[string]struct{}    // key: exception_id + ":" + recon_diff_id（去重）
	clock   func() time.Time
}

// NewExceptLinkStore 初始化关联存储。

   func NewExceptLinkStore() *ExceptLinkStore {
	   return &ExceptLinkStore{
		  links:   map[string]*ExceptLink{},
		   byExcep: map[string][]string{},
		   seen:    map[string]struct{}{},
		   clock:   func() time.Time { return time.Now().UTC() },
	  }
  }

// WithClock 注入时钟（用）。
  func (s *ExceptLinkStore) WithClock(c func() time.Time) *ExceptLinkStore {
	   if s != nil && c != nil {
		  s.clock = c
	   }
	  return s
  }

   // Link 建立清算异常与 recon 差异单的只读关联。
  func (s *ExceptLinkStore) Link(exceptionID, reconDiffID, note, actor string) (ExceptLink, error) {
	   if s == nil {
		  return ExceptLink{},
			  ErrUnavailable
	   }
	  act, err := actorOK(actor)
	  if err != nil {
		   return ExceptLink{}, err
	  }
	   exceptionID = strings.TrimSpace(exceptionID)
	   reconDiffID = strings.TrimSpace(reconDiffID)
	   if exceptionID == "" || reconDiffID == "" {
		  return ExceptLink{}, ErrNotFound
	  }
	   dedupeKey := exceptionID + ":" + reconDiffID
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  if _, exists := s.seen[dedupeKey]; exists {
		   return ExceptLink{}, ErrExceptLinkExists
	  }
	   lnk := ExceptLink{
		   ID:          uuid.NewString(),
		  ExceptionID: exceptionID,
		   ReconDiffID: reconDiffID,
		   Note:     strings.TrimSpace(note),
		   LinkedBy: act,
		  LinkedAt: s.clock(),
	  }
	  cp := lnk
	  s.links[lnk.ID] = &cp
	   s.byExcep[exceptionID] = append(s.byExcep[exceptionID], lnk.ID)
	  s.seen[dedupeKey] = struct{}{}
	   return cp, nil
  }

// Get 按 ID 查关联记录。

  func (s *ExceptLinkStore) Get(id string) (ExceptLink, error) {
	   if s == nil {
		   return ExceptLink{}, ErrUnavailable
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   lnk, ok := s.links[strings.TrimSpace(id)]
	   if !ok {
		  return ExceptLink{}, ErrExceptLinkNotFound
	   }
	  return *lnk, nil
  }

// ByException 查询指定异常 ID 的所有关联差异单。
   func (s *ExceptLinkStore) ByException(exceptionID string) []ExceptLink {
	  if s == nil {
		   return nil
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  ids := s.byExcep[strings.TrimSpace(exceptionID)]
	  out := make([]ExceptLink, 0, len(ids))
	  for _, id := range ids {
		  if lnk := s.links[id]; lnk != nil {
			   out = append(out, *lnk)
		   }
	   }
	  return out
  }

// Unlink 移除关联（谨慎操作，需记录日志）。

  func (s *ExceptLinkStore) Unlink(id, actor string) error {
	   if s == nil {
		   return ErrUnavailable
	   }
	  if _, err := actorOK(actor); err != nil {
		  return err
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  lnk, ok := s.links[strings.TrimSpace(id)]
	  if !ok {
		  return ErrExceptLinkNotFound
	  }
	   dedupeKey := lnk.ExceptionID + ":" + lnk.ReconDiffID
	   delete(s.seen, dedupeKey)
	  // 从 byExcep 列表中移除
	   list := s.byExcep[lnk.ExceptionID]
	   filtered := list[:0]
	  for _, lid := range list {
		   if lid != id {
			   filtered = append(filtered, lid)
		  }
	  }
	   s.byExcep[lnk.ExceptionID] = filtered
	  delete(s.links, id)
	   return nil
   }
