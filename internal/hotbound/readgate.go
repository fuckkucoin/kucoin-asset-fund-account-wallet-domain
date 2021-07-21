package hotbound

import (
	"context"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"account-assets.kucoin.net/funding/internal/validate"
)

// ReadGate 只读。返回前不写归集记录，调用方可用 List 核对条数未变。

  func (s *Service) ReadGate(ctx context.Context, asset string, band hotinv.GateBand) (hotinv.GateVerdict, error) {
	  if s == nil || s.hot == nil {
		   return hotinv.GateVerdict{Code: hotinv.GateDenyUnreadable, Reason: "boundary unavailable"}, ErrUnavailable
	   }
	  as, err := validate.Asset(asset)
	  if err != nil {
		   return hotinv.GateVerdict{Code: hotinv.GateDenyUnknown, Reason: "asset"}, ErrAsset
	  }
	  if s.store == nil {
		  return hotinv.GateVerdict{Asset: as, Code: hotinv.GateDenyUnreadable, Reason: "store unavailable"}, ErrStore
	   }
	  return s.hot.ReadGate(ctx, s.store, as, band)
  }
