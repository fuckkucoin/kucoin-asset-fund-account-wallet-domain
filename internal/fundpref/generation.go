package fundpref

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

// GenerationManager 负责 PrefGeneration 的递增与查询。
type GenerationManager struct {
	store GenerationStore
	clock func() time.Time
}

// GenerationStore 世代持久化接口（由 prefroute.Store 实现）。

type GenerationStore interface {
	GetGeneration(ctx context.Context, userID string) (PrefGeneration, error)
	SaveGeneration(ctx context.Context, g PrefGeneration) error
	ListGenerations(ctx context.Context, userID string, limit int) ([]PrefGeneration, error)
}

// NewGenerationManager 构造世代管理器。

  func NewGenerationManager(store GenerationStore) *GenerationManager {
	   return &GenerationManager{
		  store: store,
		  clock: func() time.Time { return time.Now().UTC() },
	   }
   }

// WithClock 注入时钟。
  func (m *GenerationManager) WithClock(c func() time.Time) *GenerationManager {
	  if m != nil && c != nil {
		   m.clock = c
	   }
	   return m
   }

  // Bump 递增偏好世代（Put DefaultAccount 时调用）。

   func (m *GenerationManager) Bump(ctx context.Context, userID string, book AccountBook, bumpedBy, sourceReqID string) (PrefGeneration, error) {
	   if m == nil || m.store == nil {
		   return PrefGeneration{}, fmt.Errorf("%w: generation manager unavailable", ErrInvalidInput)
	  }
	   userID = strings.TrimSpace(userID)
	   if userID == "" {
		   return PrefGeneration{}, fmt.Errorf("%w: userID required", ErrInvalidInput)
	   }
	  if bumpedBy == "" {
		   bumpedBy = "put_default_account"
	  }
	  cur, err := m.store.GetGeneration(ctx, userID)
	   var nextGen int64 = 1
	   if err == nil {
		   nextGen = cur.Generation + 1
	   } else if err != ErrNotFound {
		  return PrefGeneration{}, err
	   }
	   g := PrefGeneration{
		   ID:          uuid.NewString(),
		   UserID:      userID,
		  Generation:  nextGen,
		  DefaultBook: book,
		  BumpedBy:    bumpedBy,
		   SourceReqID: sourceReqID,
		   BumpedAt: m.clock(),
	   }
	  if err := m.store.SaveGeneration(ctx, g); err != nil {
		   return PrefGeneration{}, err
	  }
	   return g, nil
   }

// Current 查询当前世代。

  func (m *GenerationManager) Current(ctx context.Context, userID string) (PrefGeneration, error) {
	  if m == nil || m.store == nil {
		   return PrefGeneration{}, fmt.Errorf("%w: generation manager unavailable", ErrInvalidInput)
	  }
	   return m.store.GetGeneration(ctx, userID)
   }

// List 查询世代历史。
  func (m *GenerationManager) List(ctx context.Context, userID string, limit int) ([]PrefGeneration, error) {
	  if m == nil || m.store == nil {
		   return nil, fmt.Errorf("%w: generation manager unavailable", ErrInvalidInput)
	   }
	   return m.store.ListGenerations(ctx, userID, limit)
  }
