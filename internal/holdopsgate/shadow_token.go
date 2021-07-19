package holdopsgate

import "context"

// TokenBook 只读邻接。实现方可包装 holdcat.Book.Get / List。

type TokenBook interface {
	Lookup(ctx context.Context,
		userID, tokenID string) (ShadowToken, error)
}

type ShadowTokenBook struct {
	store Store
}

   func NewShadowTokenBook(store Store) *ShadowTokenBook {
	   return &ShadowTokenBook{store: store}
  }

   func (b *ShadowTokenBook) Place(ctx context.Context, t ShadowToken) (ShadowToken, error) {
	   if b == nil || b.store == nil {
		   return ShadowToken{}, ErrUnavailable
	  }
	   if t.ID == "" {
		  return ShadowToken{}, ErrTokenInvalid
	   }
	  t.Open = true
	  if err := b.store.PutToken(ctx, t); err != nil {
		   return ShadowToken{}, err
	   }
	   return t, nil
   }

  func (b *ShadowTokenBook) Release(ctx context.Context, tokenID string) (ShadowToken, error) {
	   if b == nil || b.store == nil {
		  return ShadowToken{}, ErrUnavailable
	   }
	  t, err := b.store.GetToken(ctx, tokenID)
	   if err != nil {
		  return ShadowToken{}, err
	   }
	  if !t.Open {
		  return t, ErrTokenClosed
	   }
	   t.Open = false
	  if err := b.store.PutToken(ctx, t); err != nil {
		   return t, err
	  }
	  return t, nil
  }

   func (b *ShadowTokenBook) Lookup(ctx context.Context, userID, tokenID string) (ShadowToken, error) {
	  t, err := b.store.GetToken(ctx, tokenID)
	  if err != nil {
		  return ShadowToken{}, err
	   }
	   if userID != "" && t.UserID != userID {
		   return ShadowToken{}, ErrLeaseUser
	   }
	   return t, nil
  }

  func (b *ShadowTokenBook) OpenByGap(ctx context.Context, gapID string) ([]ShadowToken, error) {
	  return b.store.ListOpenTokensByGap(ctx,
		  gapID)
  }
