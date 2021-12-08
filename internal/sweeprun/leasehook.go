package sweeprun

import (
	"context"
	"strings"
)

// AgreeLeaseAsserter 由 agreeleasegate 实现。sweeprun 只调用，不持有租约仓。
type AgreeLeaseAsserter interface {
	PlanAssert(ctx context.Context, leaseID, chain, asset, planRef string) error
	ExecAssert(ctx context.Context, leaseID, chain, asset, execRef string) error
}

// BindAgreeAsserter 挂同意租约闸。重复绑定覆盖。
   func (s *Service) BindAgreeAsserter(a AgreeLeaseAsserter) *Service {
	   if s != nil {
		  s.agree = a
	  }
	  return s
  }

// BindAgreeBook 挂读闸簿记。

  func (s *Service) BindAgreeBook(g *AgreeBook) *Service {
	  if s != nil {
		   s.gate = g
	  }
	   return s
  }

// AgreeBookView 只读暴露。
  func (s *Service) AgreeBookView() *AgreeBook {
	  if s == nil {
		  return nil
	   }
	  return s.gate
   }

// EnqueueReq 扩展字段在 types.go 的 AgreeLeaseID。

  func (s *Service) requirePlanLease(ctx context.Context, req EnqueueReq, planRef string) error {
	  if s == nil || s.agree == nil {
		   return nil
	   }
	  id := strings.TrimSpace(req.AgreeLeaseID)
	   if id == "" {
		   return ErrAgreeLease
	  }
	   return s.agree.PlanAssert(ctx, id, req.Chain, req.Asset, planRef)
   }

  func (s *Service) requireExecLease(ctx context.Context, job *Job) error {
	  if s == nil || s.agree == nil {
		   return nil
	  }
	   id := strings.TrimSpace(job.AgreeLeaseID)
	   if id == "" {
		  return ErrAgreeLease
	   }
	  return s.agree.ExecAssert(ctx,
		  id, job.Chain, job.Asset, job.ID)
  }
