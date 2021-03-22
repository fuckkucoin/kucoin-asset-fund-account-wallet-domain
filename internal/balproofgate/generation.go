package balproofgate

import (
	"context"
	"strings"
	"time"
	"github.com/google/uuid"
)

// GenerationEngine 管理 ProofGeneration。

type GenerationEngine struct {
	store Store
	clock func() time.Time
}

   func newGenerationEngine(store Store, clock func() time.Time) *GenerationEngine {
	  if clock == nil {
		   clock = func() time.Time { return time.Now().UTC() }
	}
	return &GenerationEngine{store: store, clock: clock}
}

// Current 读取当前世代，无记录视为 0。
  func (e *GenerationEngine) Current(ctx context.Context, userID, book, asset string) (int64, error) {
	   if e == nil || e.store == nil {
		   return 0, ErrUnavailable
	   }
	   userID, book, asset, err := normKey(userID, book, asset)
	   if err != nil {
		   return 0, err
	  }
	  return e.store.CurrentGeneration(ctx, userID, book, asset)
   }

// Bump 原子抬世代。expected 必须等于当前值。

  func (e *GenerationEngine) Bump(ctx context.Context, userID, book, asset string, expected int64) (int64, error) {
	   if e == nil || e.store == nil {
		   return 0, ErrUnavailable
	   }
	  userID, book, asset, err := normKey(userID, book, asset)
	   if err != nil {
		   return 0, err
	   }
	  next, err := e.store.BumpGeneration(ctx, userID, book, asset, expected, e.clock())
	  if err != nil {
		   return 0, err
	  }
	   return next, nil
  }

// Get 完整记录；不存在则返回世代 0 的骨架（尚未落库）。
  func (e *GenerationEngine) Get(ctx context.Context, userID, book, asset string) (*ProofGeneration, error) {
	  if e == nil || e.store == nil {
		  return nil, ErrUnavailable
	   }
	  userID, book, asset, err := normKey(userID, book, asset)
	  if err != nil {
		   return nil, err
	  }
	  g, err := e.store.GetGeneration(ctx, userID, book, asset)
	   if err != nil {
		   return nil, err
	  }
	  if g == nil {
		  return &ProofGeneration{
			   ID: uuid.NewString(), UserID: userID, Book: book, Asset: asset,
			   Generation: 0, UpdatedAt: e.clock(),
		  }, nil
	   }
	  cp := g.Copy()
	  return &cp, nil
   }

   func normKey(userID, book, asset string) (string, string, string, error) {
	  userID = strings.TrimSpace(userID)
	   book = strings.TrimSpace(book)
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	  if userID == "" || userID == "system" {
		  return "", "", "", ErrUserInvalid
	   }
	  if book == "" {
		   return "", "", "", ErrBookInvalid
	   }
	   if asset == "" {
		  return "", "", "", ErrAssetInvalid
	  }
	  return userID, book, asset, nil
   }
