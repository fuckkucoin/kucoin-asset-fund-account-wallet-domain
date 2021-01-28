package regioinv

import (
	"context"
	"time"
)

// TransferRegions 在两个区的同一层之间划转可用。
// 调用方传入已解析的 Transfer（ID 字段）；守恒失败返回 ErrConserve。
  func (svc *Service) TransferRegions(ctx context.Context, t Transfer, now time.Time) (RegionInv, RegionInv, error) {
	  if now.IsZero() {
		   now = svc.now()
	   }
	   return svc.st.Transfer(ctx, t)
   }
