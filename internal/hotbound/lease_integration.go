package hotbound

import (
	"context"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/hotbreach"
)

// LeaseAwarePreSubmit 在 PreSubmit 只读检查通过后签发 PreLease。
// 调用时序：
// 1. 调用 s.PreSubmit（只读热库存检查）。
// 2. 检查通过则调用 hotbreach.Service.IssueLease。
// 3. 返回签发的 PreLease 供出金流程携带。
// 若 breachSvc 为 nil，退化为原有 PreSubmit 语义（不签发租约）。
  func (s *Service) LeaseAwarePreSubmit(
	   ctx context.Context,
	   asset string,
	   issuedBy string,
	  breachSvc *hotbreach.Service,
   ) (hotbreach.PreLease, error) {
	  if s == nil {
		   return hotbreach.PreLease{}, ErrUnavailable
	  }
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	  if asset == "" {
		   return hotbreach.PreLease{}, ErrAsset
	   }
	   // 1. 只读预检（原有语义）
	   if err := s.PreSubmit(ctx, asset); err != nil {
		   return hotbreach.PreLease{}, err
	   }
	   // 2. 未装配 breachSvc → 退化
	   if breachSvc == nil {
		  return hotbreach.PreLease{}, nil
	  }
	   // 3. 签发租约
	  req := hotbreach.IssueLeaseReq{
		   Asset: asset,
		  IssuedBy: issuedBy,
	   }
	   lease, err := breachSvc.IssueLease(ctx, req)
	   if err != nil {
		   return hotbreach.PreLease{}, fmt.Errorf("hotbound: issue lease after presubmit: %w", err)
	  }
	   return lease, nil
  }

// GuardedDebit 出金/归集预扣前调用 DrainGuard 执行挡写检查。
// 参数 leaseID 为空表示调用方未走租约路径（可能被挡写）。
// 参数 party 为本次操作方标识（用于击穿记录）。
// 若 breachSvc 为 nil，跳过挡写检查（降级为原有语义）。
  func GuardedDebit(
	  ctx context.Context,
	  asset string,
	   leaseID string,
	   party string,
	  breachSvc *hotbreach.Service,
   ) error {
	  if breachSvc == nil {
		  return nil
	   }
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	  if asset == "" {
		   return ErrAsset
	   }
	   req := hotbreach.GuardReq{
		   Asset:   asset,
		  LeaseID: leaseID,
		   Party: party,
	   }
	   return breachSvc.Guard(ctx, req)
   }
