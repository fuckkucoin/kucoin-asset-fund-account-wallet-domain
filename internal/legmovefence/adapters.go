package legmovefence

import (
	"context"
	"account-assets.kucoin.net/funding/internal/georebal"
)

// IsolationNote 记录与相邻包的只读，避免误改 r5 / AgreeGate / trexecgate。
type IsolationNote struct {
	NotIsolationAttest bool `json:"not_r5_isolation_attest"`
	NotAgreeGate       bool `json:"not_agree_gate"`
	NotTrexecGate      bool `json:"not_trexecgate"`
}

  func DefaultIsolation() IsolationNote {
	  return IsolationNote{NotIsolationAttest: true, NotAgreeGate: true, NotTrexecGate: true}
   }

// ObserveSnapshot 只读 Plan/Leg。
  func ObserveSnapshot(ctx context.Context, book *georebal.Book, planID, legID string) (georebal.LegSnapshot, error) {
	   return georebal.BookSurface{Book: book}.Snapshot(ctx,
		   planID, legID)
  }

// FenceIDOf 供他包只读消费。
  func FenceIDOf(f LegMoveFence) string {
	  return f.FenceID
   }
