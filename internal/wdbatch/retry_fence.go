package wdbatch

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/batchresiduefence"
)

   func (s *Service) RetryWithFence(ctx context.Context, req batchresiduefence.RetryWithFenceReq) (batchresiduefence.RetryWithFenceResp, error) {
	  if s == nil {
		  return batchresiduefence.RetryWithFenceResp{},
			   ErrUnavailable
	   }
	  eng := s.ensureFence()
	  if eng == nil {
		  return batchresiduefence.RetryWithFenceResp{}, ErrUnavailable
	  }
	  return eng.RetryWithFence(ctx, req)
   }

   func (s *Service) SubmitAssert(ctx context.Context, req batchresiduefence.SubmitAssertReq) (batchresiduefence.SubmitAssertResp, error) {
	  if s == nil {
		   return batchresiduefence.SubmitAssertResp{}, ErrUnavailable
	  }
	   eng := s.ensureFence()
	  if eng == nil {
		  return batchresiduefence.SubmitAssertResp{},
			   ErrUnavailable
	  }
	  return eng.SubmitAssert(ctx, req)
  }

   func (s *Service) IssueResidueFence(ctx context.Context, req batchresiduefence.IssueFenceReq) (batchresiduefence.ResidueFence, error) {
	   if s == nil {
		  return batchresiduefence.ResidueFence{}, ErrUnavailable
	  }
	   return s.ensureFence().IssueFence(ctx, req)
  }

  func (s *Service) ExportDupFreezeProofs(ctx context.Context, batchID string) (batchresiduefence.ProofExport, error) {
	  if s == nil {
		   return batchresiduefence.ProofExport{}, ErrUnavailable
	  }
	  return s.ensureFence().ExportDupProofs(ctx, strings.TrimSpace(batchID))
  }

  func (s *Service) PatrolResidue(ctx context.Context, limit int) (batchresiduefence.PatrolReport, error) {
	   if s == nil {
		  return batchresiduefence.PatrolReport{}, ErrUnavailable
	  }
	   return s.ensureFence().PatrolResidue(ctx, limit)
  }
