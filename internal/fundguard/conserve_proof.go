package fundguard

import (
	"context"
	"account-assets.kucoin.net/funding/internal/guardlease"
)

// ProofService 封装 guardlease 证明相关操作（方便替换）。

type ProofService interface {
	RecordProof(ctx context.Context, req guardlease.ConserveProofRequest) (guardlease.ConserveProof, error)
	IsBlocked(userID, asset string) bool
	Halted() bool
}

// BindProofStore 注入 ProofService（与 LeaseService 可以是同一个 guardlease.Service 实例）。
  func (s *Service) BindProofStore(ps ProofService) *Service {
	   if s != nil {
		  s.proofer = ps
	  }
	   return s
  }

// ConserveWithProof 完整守恒证明流程：
// 1. 核验世代租约（若 leaser 已注入）；
// 2. 执行 Conserve ；
// 3. 无论成败均落地 ConserveProof（若 proofer 已注入）；
// 4. 守恒失败时触发挡写升级，返回 ErrConserve。
  func (s *Service) ConserveWithProof(ctx context.Context, before, after Bundle, leaseID, snapHash string) error {
	   if s == nil {
		   return ErrUnavailable
	  }
	   if s.Halted() {
		   return ErrHalt
	  }
	   if s.proofer != nil {
		  if s.proofer.IsBlocked(before.UserID, before.Asset) {
			  return ErrHalt
		   }
	   }
	  // 核验租约（fail-closed）
	  if s.leaser != nil && leaseID != "" {
		   if _, err := s.leaser.BindLease(ctx, guardlease.BindLeaseRequest{
			  LeaseID: leaseID,
			   UserID: before.UserID,
			   Asset: before.Asset,
			  SnapshotHash: snapHash,
		   }); err != nil {
			  return mapLeaseErr(err)
		   }
	   }
	  // 执行守恒
	  conserveErr := s.Conserve(before, after)
	   // 落地证明
	   if s.proofer != nil {
		   proofReq := guardlease.ConserveProofRequest{
			  LeaseID: leaseID,
			   UserID:      before.UserID,
			   Asset:       before.Asset,
			  BeforeTotal: before.Total,
			  AfterTotal: after.Total,
			  SnapHash:   snapHash,
		  }
		  if _, perr := s.proofer.RecordProof(ctx, proofReq); perr != nil {
			   // 证明记录失败（含守恒失败升级）：先返回守恒错误，其次返回证明错误
			  if conserveErr != nil {
				  return conserveErr
			   }
			  return perr
		  }
	  }
	   return conserveErr
   }
