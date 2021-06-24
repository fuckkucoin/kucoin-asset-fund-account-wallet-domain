package fxmarkleasegate

import (
	"context"
	"errors"
	"sync"
)

// PostBinder 资金过账绑定。无租约不得成功。

type PostBinder interface {
	Issue(ctx context.Context, from, to, purpose string) (IssueMarkLeaseResp, error)
	Assert(ctx context.Context,
		leaseID, from, to, purpose, postRef string, consume bool) (AssertBeforePostResp, error)
}

type EngineBinder struct {
	Eng *Engine
}

   func (b EngineBinder) Issue(ctx context.Context, from, to, purpose string) (IssueMarkLeaseResp, error) {
	  if b.Eng == nil {
		   return IssueMarkLeaseResp{},
			   ErrNilEngine
	  }
	   return b.Eng.IssueMarkLease(ctx, IssueMarkLeaseReq{From: from, To: to, Purpose: purpose})
   }

  func (b EngineBinder) Assert(ctx context.Context, leaseID, from, to, purpose, postRef string, consume bool) (AssertBeforePostResp, error) {
	   if b.Eng == nil {
		  return AssertBeforePostResp{}, ErrNilEngine
	  }
	   return b.Eng.AssertBeforePost(ctx, AssertBeforePostReq{
		  LeaseID: leaseID, From: from, To: to, Purpose: purpose, PostRef: postRef, Consume: consume,
	  })
   }

var (
	globalMu sync.RWMutex
	globalBinder PostBinder
)

  func BindGlobal(b PostBinder) {
	   globalMu.Lock()
	  globalBinder = b
	  globalMu.Unlock()
   }

  func Global() PostBinder {
	  globalMu.RLock()
	  defer globalMu.RUnlock()
	   return globalBinder
  }

// MustAssert 落账入口。未绑定全局引擎且无租约则失败关闭。
   func MustAssert(ctx context.Context, leaseID, from, to, purpose, postRef string) error {
	  b := Global()
	  if b == nil {
		   if stringsEmpty(leaseID) {
			  return ErrMissingLease
		   }
		  return ErrNilEngine
	   }
	   resp,
		  err := b.Assert(ctx, leaseID, from, to, purpose, postRef, true)
	  if err != nil {
		   return err
	  }
	   if !resp.Posted {
		   return ErrLeaseNotBound
	  }
	   return nil
   }

   func stringsEmpty(s string) bool { return len(s) == 0 }

   func IsFailClosed(err error) bool {
	   return errors.Is(err, ErrMissingLease) ||
		   errors.Is(err, ErrLeaseExpired) ||
		  errors.Is(err,
			   ErrLeaseRevoked) ||
		  errors.Is(err, ErrPutInvalidated) ||
		   errors.Is(err, ErrLiveOverThreshold) ||
		   errors.Is(err, ErrRateMissing) ||
		  errors.Is(err, ErrMissingRate) ||
		  errors.Is(err,
			  ErrFingerprintDrift)
  }
