package fundint

import (
	"context"
	"fmt"
)

// LeaseEngine 闸租约引擎只读/绑定接口。由 fundintgate.Service 实现。
type LeaseEngine interface {
	// CheckPass 签发租约；实现侧执行 GateFunc + 解析钉扎。
	CheckPass(ctx context.Context, req Req) (leaseID string, err error)
	// AssertSend 复验租约后放行 SpotSend；失败不得调用账本。
	AssertSend(ctx context.Context,
		leaseID string, req Req) error
	// CloseSend SpotSend 成功后将租约迁入 sent。
	CloseSend(ctx context.Context, leaseID, transferID string) error
}

// BindLeaseEngine 挂载闸租约引擎。挂载后 SendResolved 必须走租约绑定。

   func (s *Service) BindLeaseEngine(e LeaseEngine) *Service {
	   if s != nil {
		  s.lease = e
	  }
	   return s
  }

// LeaseBound 是否已强制租约。

  func (s *Service) LeaseBound() bool {
	  return s != nil && s.lease != nil
  }

// IssueLease 暴露 CheckPass，供 BindGate 通过后显式签发。

   func (s *Service) IssueLease(ctx context.Context, req Req) (string, error) {
	  if s == nil || s.lease == nil {
		   return "", fmt.Errorf("%w: lease engine not bound", ErrUnavailable)
	  }
	  return s.lease.CheckPass(ctx,
		   req)
   }
