package fxmark

import "context"

// PostLeaseGuard 落账前租约闸。由 fxmarkleasegate 注入，缺省失败关闭。
type PostLeaseGuard func(ctx context.Context, leaseID, from, to, purpose, postRef string) error

   func (b *Book) BindPostGuard(g PostLeaseGuard) *Book {
	   if b != nil {
		   b.postGuard = g
	  }
	  return b
  }

// AssertPostLease 换算/资金页估值落账前复核。未挂闸且无租约则失败关闭。

  func (b *Book) AssertPostLease(ctx context.Context, leaseID, from, to, purpose, postRef string) error {
	  if b == nil {
		  return ErrRate
	   }
	   if b.postGuard != nil {
		  return b.postGuard(ctx, leaseID, from, to, purpose, postRef)
	   }
	  if leaseID == "" {
		  return ErrRate
	  }
	   return nil
  }
