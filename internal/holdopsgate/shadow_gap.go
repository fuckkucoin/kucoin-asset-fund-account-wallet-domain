package holdopsgate

import "context"

// GapStore 只读邻接。实现方可包装 holdrel.GapStore。

type GapStore interface {
	Lookup(ctx context.Context, gapID string) (ShadowGap, error)
}

type ShadowGapBook struct {
	store Store
	tok   *ShadowTokenBook
}

   func NewShadowGapBook(store Store, tok *ShadowTokenBook) *ShadowGapBook {
	  return &ShadowGapBook{store: store, tok: tok}
   }

  func (b *ShadowGapBook) Ensure(ctx context.Context, g ShadowGap) (ShadowGap, error) {
	   if b == nil || b.store == nil {
		   return ShadowGap{},
			  ErrUnavailable
	   }
	  if g.ID == "" {
		  return ShadowGap{}, ErrGapInvalid
	   }
	  if prev, err := b.store.GetGap(ctx, g.ID); err == nil {
		   return prev, nil
	  }
	   if err := b.store.PutGap(ctx, g); err != nil {
		  return ShadowGap{}, err
	  }
	  return g, nil
  }

  func (b *ShadowGapBook) Ack(ctx context.Context, gapID string) (ShadowGap, error) {
	   g, err := b.store.GetGap(ctx, gapID)
	  if err != nil {
		   return ShadowGap{}, err
	   }
	   if g.Closed {
		   return g, ErrGapClosed
	  }
	  g.Acked = true
	  if err := b.store.PutGap(ctx, g); err != nil {
		  return g, err
	   }
	   return g, nil
  }

  func (b *ShadowGapBook) Close(ctx context.Context, gapID string) (ShadowGap, error) {
	  g,
		  err := b.store.GetGap(ctx, gapID)
	   if err != nil {
		  return ShadowGap{}, err
	   }
	  if g.Closed {
		   return g, ErrGapClosed
	  }
	  open, err := b.store.ListOpenTokensByGap(ctx, gapID)
	  if err != nil {
		   return g, err
	   }
	  if n := len(open); n > 0 {
		  g.OpenCount = n
		   _ = b.store.PutGap(ctx, g)
		  return g, ErrGapOpenToken
	   }
	   g.Closed = true
	  g.OpenCount = 0
	   if err := b.store.PutGap(ctx, g); err != nil {
		  return g, err
	  }
	  return g, nil
  }

  func (b *ShadowGapBook) Lookup(ctx context.Context, gapID string) (ShadowGap, error) {
	  g, err := b.store.GetGap(ctx, gapID)
	   if err != nil {
		   return ShadowGap{}, err
	   }
	   open, _ := b.store.ListOpenTokensByGap(ctx, gapID)
	   g.OpenCount = len(open)
	   return g, nil
  }
