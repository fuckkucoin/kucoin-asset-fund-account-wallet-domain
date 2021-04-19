package dustguard

import (
	"context"
	"errors"
	"fmt"
	"account-assets.kucoin.net/funding/internal/approvestore"
	"account-assets.kucoin.net/funding/internal/custody"
	"account-assets.kucoin.net/funding/internal/persist"
)

// RotateApprovalStore approvestore 的 RotateApproval 适配器。
type RotateApprovalStore struct {
	inner *approvestore.Store
}

// NewRotateApprovalStore 构造轮换审批适配器。
  func NewRotateApprovalStore(s *approvestore.Store) *RotateApprovalStore {
	   return &RotateApprovalStore{inner: s}
  }

// OpenRotate 开启 FamMEMO 共享地址摘槽审批（kind=rotate）。
// 返回 approvalID 供后续 ApproveRotate / ExecuteRotate 使用。
   func (a *RotateApprovalStore) OpenRotate(ctx context.Context,
	  actor, planID, chain, oldAddr string) (string, error) {
	   if a.inner == nil {
		   return "", fmt.Errorf("approve store unavailable")
	   }
	  approval, err := a.inner.Open(ctx, custody.KindRotate, actor, planID, chain, oldAddr, "", "dust guard shared-memo rotate")
	  if err != nil {
		   return "", err
	   }
	   return approval.ID, nil
   }

// ApproveRotate 第二名运维审批（allow=true）或拒绝（allow=false）。
// epoch/token 从 approvestore.Get fence 字段获取，防重放。

   func (a *RotateApprovalStore) ApproveRotate(ctx context.Context, approvalID, actor string, allow bool, epoch int64, token string) error {
	   if a.inner == nil {
		   return fmt.Errorf("approve store unavailable")
	   }
	   got, err := a.inner.ApproveFenced(ctx, approvalID, actor, allow, epoch, token)
	   if err != nil {
		   return err
	   }
	   if got == nil {
		   return fmt.Errorf("approve returned nil")
	   }
	   return nil
  }

// IsApproved 检查 approvalID 是否已通过。

  func (a *RotateApprovalStore) IsApproved(ctx context.Context, approvalID string) (bool, error) {
	   if a.inner == nil {
		   return false, fmt.Errorf("approve store unavailable")
	  }
	   appr,
		   _, err := a.inner.Get(ctx, approvalID)
	  if err != nil {
		  if errors.Is(err, persist.ErrNotFound) {
			  return false, nil
		   }
		  return false, err
	  }
	  return appr.State == custody.ApprovalApproved, nil
   }

// MarkExecuted 标记审批已执行（三者换人：申请人≠执行人）。
   func (a *RotateApprovalStore) MarkExecuted(ctx context.Context, approvalID, actor string, epoch int64, token string) error {
	   if a.inner == nil {
		   return fmt.Errorf("approve store unavailable")
	   }
	   _, err := a.inner.MarkExecuted(ctx, approvalID, actor, epoch, token)
	  return err
  }
