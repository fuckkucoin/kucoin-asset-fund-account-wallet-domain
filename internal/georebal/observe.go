package georebal

import (
	"context"
	"strings"
	"time"
)

// Surface 只读观察面，供 legmovefence 复验 Plan/Leg/区域 Halted。

type Surface interface {
	Snapshot(ctx context.Context, planID, legID string) (LegSnapshot, error)
	RegionHalted(ctx context.Context, regionID string) (bool, int64, error)
	SuccessfulMoves(legID string) int
	Balance(region, asset string) int64
}

// BookSurface Book 的只读适配。
type BookSurface struct {
	Book *Book
}

  func (s BookSurface) Snapshot(ctx context.Context, planID, legID string) (LegSnapshot, error) {
	   if s.Book == nil {
		   return LegSnapshot{}, ErrUnavailable
	  }
	  return s.Book.Snapshot(ctx, planID, legID)
   }

  func (s BookSurface) RegionHalted(ctx context.Context, regionID string) (bool, int64, error) {
	  if s.Book == nil {
		  return false, 0, ErrUnavailable
	   }
	  r, err := s.Book.GetRegion(ctx, regionID)
	  if err != nil {
		   return false, 0, err
	   }
	   return r.Status == RegionHalted,
		  r.HaltGen, nil
   }

   func (s BookSurface) SuccessfulMoves(legID string) int {
	   if s.Book == nil {
		  return 0
	  }
	  return s.Book.SuccessfulMoves(legID)
  }

   func (s BookSurface) Balance(region, asset string) int64 {
	  if s.Book == nil {
		   return 0
	   }
	   return s.Book.Balance(region, asset)
  }

// SweepObserve 归集意图观察点。不调用 sweeprun AgreeGate，只记录挂钩摘要。

type SweepObserve struct {
	Seen []SweepNote
}

// SweepNote Enqueue 观察摘要。
type SweepNote struct {
	At time.Time
	PlanID string
	LegID   string
	FenceID string
	Kind    string
}

   func (o *SweepObserve) NoteEnqueue(planID, legID, fenceID string) {
	   if o == nil {
		   return
	  }
	  o.Seen = append(o.Seen, SweepNote{
		  At: time.Now().UTC(), PlanID: planID, LegID: legID, FenceID: fenceID, Kind: "enqueue",
	  })
   }

   func (o *SweepObserve) Count(kind string) int {
	  if o == nil {
		   return 0
	  }
	  n := 0
	  for _, s := range o.Seen {
		   if s.Kind == kind {
			   n++
		  }
	  }
	   return n
   }

// SeedDemo 构造可执行计划（两区 USDT）。
   func SeedDemo(book *Book, planID, legID, idem string, amount int64) {
	  if book == nil {
		   return
	   }
	  now := time.Now().UTC()
	   book.PutRegion(Region{
		  RegionID: "sg-1", Name: "Singapore", Status: RegionLive, HaltGen: 1,
		   Balances: map[string]int64{"USDT": 1_000_000}, UpdatedAt: now,
	  })
	   book.PutRegion(Region{
		  RegionID: "tyo-1", Name: "Tokyo", Status: RegionLive, HaltGen: 1,
		   Balances: map[string]int64{"USDT": 100_000}, UpdatedAt: now,
	  })
	   book.PutPlan(Plan{PlanID: planID, Asset: "USDT", State: FencePlanOpen, CreatedAt: now})
	  if strings.TrimSpace(idem) == "" {
		  idem = "idem-" + legID
	   }
	  book.PutLeg(Leg{
		  LegID: legID, PlanID: planID, SrcRegion: "sg-1", DstRegion: "tyo-1",
		  Asset: "USDT", Amount: amount, State: FenceLegPending, IdempotencyKey: idem, CreatedAt: now,
	  })
   }
