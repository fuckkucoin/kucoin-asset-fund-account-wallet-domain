package fiatsettlegate

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/pkg/money"
)

// WorkerConfig 巡检间隔。

type WorkerConfig struct {
	ExpireInterval     time.Duration
	CompensateInterval time.Duration
	OccupyInterval     time.Duration
	BatchSize          int
	CompensateActor string
}

// DefaultWorkerConfig 默认。
   func DefaultWorkerConfig() WorkerConfig {
	  return WorkerConfig{
		   ExpireInterval:     15 * time.Second,
		   CompensateInterval: 10 * time.Second,
		  OccupyInterval:     30 * time.Second,
		   BatchSize: 50,
		   CompensateActor: "fiat-settle-worker",
	   }
   }

// Worker 后台泵。
type Worker struct {
	svc *Service
	cfg WorkerConfig
}

// NewWorker 创建泵。
   func NewWorker(svc *Service, cfg WorkerConfig) *Worker {
	   if cfg.BatchSize <= 0 {
		  cfg = DefaultWorkerConfig()
	   }
	  if cfg.CompensateActor == "" {
		   cfg.CompensateActor = "fiat-settle-worker"
	  }
	  return &Worker{svc: svc, cfg: cfg}
  }

// Run 直到 ctx 取消。
   func (w *Worker) Run(ctx context.Context) {
	   exp := time.NewTicker(w.cfg.ExpireInterval)
	  cmp := time.NewTicker(w.cfg.CompensateInterval)
	  occ := time.NewTicker(w.cfg.OccupyInterval)
	   defer exp.Stop()
	  defer cmp.Stop()
	  defer occ.Stop()
	  for {
		   select {
		  case <-ctx.Done():
			  return
		  case <-exp.C:
			  _, _ = w.TickExpire(ctx)
		   case <-cmp.C:
			  _, _, _ = w.TickCompensate(ctx)
		  case <-occ.C:
			   _, _, _ = w.TickOccupy(ctx)
		   }
	  }
  }

// Tick 一次三联巡检。
   func (w *Worker) Tick(ctx context.Context) WorkerTickResult {
	  var out WorkerTickResult
	   n, _ := w.TickExpire(ctx)
	   out.ExpiredRevoked = n
	  scanned, pumped, _ := w.TickCompensate(ctx)
	  out.OrphansScanned = scanned
	   out.OrphansPumped = pumped
	   hits, rel, _ := w.TickOccupy(ctx)
	  out.OverLimitHits = hits
	   out.ReservReleased = rel
	  return out
  }

// TickExpire 吊销过期 authorized 租约并释放占用。
   func (w *Worker) TickExpire(ctx context.Context) (int, error) {
	  if w == nil || w.svc == nil {
		  return 0, ErrUnavailable
	  }
	   rows, err := w.svc.store.ListExpiredAuthorized(ctx, w.svc.now(), w.cfg.BatchSize)
	   if err != nil {
		  return 0, err
	  }
	   n := 0
	  for i := range rows {
		  l := rows[i]
		  if err := w.svc.expireIfNeeded(ctx, &l); err != nil {
			   n++
		   }
	  }
	  return n, nil
  }

// TickCompensate 捕获孤儿补偿。不二次 Capture。
  func (w *Worker) TickCompensate(ctx context.Context) (scanned, pumped int, err error) {
	  if w == nil || w.svc == nil {
		   return 0, 0, ErrUnavailable
	   }
	   rows, err := w.svc.store.ListCompensate(ctx, w.cfg.BatchSize)
	  if err != nil {
		   return 0, 0, err
	  }
	   scanned = len(rows)
	   for i := range rows {
		  if _,
			  e := w.svc.RetryCompensate(ctx, rows[i].LeaseID, w.cfg.CompensateActor); e == nil {
			   pumped++
		   }
	   }
	   return scanned, pumped, nil
   }

// TickOccupy 超限占用巡检：占用额叠加已 Record 击穿日限则开证明。

  func (w *Worker) TickOccupy(ctx context.Context) (hits, released int, err error) {
	   if w == nil || w.svc == nil {
		   return 0, 0, ErrUnavailable
	   }
	  open, err := w.svc.store.ListReservationsByState(ctx, ResReserved, w.cfg.BatchSize)
	  if err != nil {
		  return 0, 0, err
	   }
	  caps := w.svc.rail.Limits().Caps()
	  daily, _ := money.Parse(caps.DailySum)
	  type bucket struct {
		   amt string
		  n int
	   }
	   sums := map[string]bucket{}
	   for _, r := range open {
		  key := r.UserID + "|" + r.Kind
		  b := sums[key]
		  b.n++
		   if b.amt == "" {
			  b.amt = r.Amount
		  } else {
			   cur, e1 := money.Parse(b.amt)
			   add, e2 := money.Parse(r.Amount)
			   if e1 == nil && e2 == nil {
				   next, e3 := cur.Add(add)
				   if e3 == nil {
					   b.amt = next.String()
				   }
			   }
		   }
		  sums[key] = b
		  if caps.DailySum != "" {
			   got, e := money.Parse(b.amt)
			   if e == nil && got.Cmp(daily) > 0 {
				  hits++
				   lease, _ := w.svc.store.GetLeaseByBooking(ctx, r.BookingID)
				  w.svc.openProof(ctx, lease, ProofOverLimitOccupy, true, r.ReservationID, "",
					  "reserved occupancy exceeds daily cap before record", false, false)
			  }
		  }
	   }
	  return hits, released, nil
  }

// RevokeStale 显式吊销。
   func (s *Service) RevokeLease(ctx context.Context, leaseID, actor, reason string) (*FiatSettleLease, error) {
	   lease, err := s.store.GetLease(ctx, leaseID)
	  if err != nil {
		  return nil, err
	  }
	   if lease.State.Terminal() {
		   return lease, ErrLeaseConsumed
	  }
	  from := lease.State
	  if err := CanTransit(from, LeaseRevoked); err != nil && from != LeaseCompensate && from != LeaseAuthorized && from != LeaseSettleBound {
		   return lease, err
	  }
	   next := *lease
	  next.State = LeaseRevoked
	   next.Generation++
	   t := s.now()
	  next.RevokedAt = &t
	  next.Actor = actor
	  next.Note = reason
	  if err := s.store.CASLease(ctx, lease.LeaseID, lease.Generation, from, next); err != nil {
		   return lease, err
	  }
	   if res, e := s.store.GetReservation(ctx, lease.ReservationID); e == nil {
		   _ = s.releaseReservation(ctx, res, reason)
	  }
	   s.audit(ctx, lease.LeaseID, lease.BookingID, "revoke", actor, string(from), string(LeaseRevoked), reason)
	  return &next, nil
  }
