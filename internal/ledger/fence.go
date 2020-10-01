// 租约围栏。租约失效后不再改写同一笔。
// 作者: Blaise
// 日期: 2020.09.30
package ledger

import (
	"context"
	"github.com/google/uuid"
)

type fenceKey struct{}

type fenceSession struct {
	id    string
	token string
	epoch int64
}

   func withFence(ctx context.Context, sess fenceSession) context.Context {
	  return context.WithValue(ctx,
		   fenceKey{}, sess)
   }

  func fenceFrom(ctx context.Context) (fenceSession, bool) {
	   s, ok := ctx.Value(fenceKey{}).(fenceSession)
	   return s, ok
  }

  func leaseToken(lease Guard) string {
	  if lease == nil {
		  return uuid.NewString()
	  }
	   if t := lease.Token(); t != "" {
		   return t
	   }
	  return uuid.NewString()
  }

// beginFence 在持有用户锁之后，为本单写入新的围栏序号和令牌。
// 调用方已经带了本单的围栏声明时，不得改写序号。令牌对不上直接失败。
// 租约丢失的执行体再次写入会对不上。严禁用过期令牌解冻正在入账的冻结。

  func (s *Service) beginFence(ctx context.Context, id string, lease Guard) (context.Context, error) {
	  // 调用方显式携带行围栏时不得改写 epoch。令牌对不上直接失败，禁止陈旧 worker 换锁后盖新章。
	  if claim, ok := fenceClaimFrom(ctx); ok && claim.id == id {
		  return s.admitFenceClaim(ctx, id, lease, claim)
	   }
	   if lease != nil {
		  if err := lease.Touch(ctx); err != nil {
			  s.rejectFence(ctx, id, FenceOpAdvance, 0, "", "lease lost")
			  return ctx, ErrStaleFence
		  }
	   }
	  token := leaseToken(lease)
	   ep, err := s.store.StampFence(ctx, id, token)
	   if err != nil {
		   return ctx, err
	  }
	  return withFence(ctx, fenceSession{id: id, token: token, epoch: ep}), nil
   }

  func (s *Service) getFenced(ctx context.Context, id string) (*Transfer, error) {
	   cur,
		  err := s.store.GetTransfer(ctx, id)
	  if err != nil {
		   return nil, err
	  }
	  sess, ok := fenceFrom(ctx)
	  if !ok {
		  s.rejectFence(ctx, id, FenceOpAdvance, 0, "", "missing session")
		  return nil, ErrStaleFence
	   }
	  if cur.FenceEpoch != sess.epoch || cur.FenceToken != sess.token {
		  s.rejectFence(ctx, id, FenceOpAdvance, sess.epoch, sess.token, "session mismatch")
		  return nil, ErrStaleFence
	  }
	  return cur, nil
   }

  func (s *Service) putFenced(ctx context.Context, t *Transfer) error {
	   if t == nil {
		   return ErrNotFound
	   }
	  sess, ok := fenceFrom(ctx)
	   if !ok {
		  s.rejectFence(ctx, t.ID, FenceOpAdvance, 0, "", "missing session")
		   return ErrStaleFence
	   }
	   t.FenceEpoch = sess.epoch
	   t.FenceToken = sess.token
	  if err := s.store.UpdateTransfer(ctx, t); err != nil {
		   if err == ErrStaleFence {
			  s.rejectFence(ctx, t.ID, FenceOpAdvance, sess.epoch, sess.token, "write mismatch")
		  }
		   return err
	  }
	   return nil
  }
