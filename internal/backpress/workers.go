package backpress

import (
	"context"
	"sync"
	"account-assets.kucoin.net/funding/internal/conc"
	"account-assets.kucoin.net/funding/internal/persist"
)

type WorkerBatch struct {
	Requested int `json:"requested"`
	Accepted int `json:"accepted"`
	Failed  int `json:"failed"`
	Dropped int `json:"dropped"`
}

// PumpInto 把本队列拷进 conc.Pipeline。满则记 Dropped，不在锁里跑阶段。

  func (s *Service) PumpInto(ctx context.Context, pipe *conc.Pipeline, limit int) WorkerBatch {
	  out := WorkerBatch{}
	   if s == nil || pipe == nil {
		   return out
	  }
	   jobs := s.Drain(limit)
	   out.Requested = len(jobs)
	   for _, j := range jobs {
		  if _, err := pipe.Offer(j.Kind + "|" + j.Payload); err != nil {
			   if err == conc.ErrFull {
				  out.Dropped++
				   continue
			  }
			   out.Failed++
			   continue
		   }
		   out.Accepted++
	   }
	  if out.Requested > 0 {
		  _ = pipe.Run(ctx, out.Accepted)
		  s.Finish(out.Requested, out.Failed+out.Dropped)
	  }
	   return out
   }

  func (s *Service) OfferMany(kind string, payloads []string) (accepted, dropped int) {
	  for _, p := range payloads {
		  if _, err := s.Offer(kind, p); err != nil {
			  if err == ErrFull || err == ErrHalt || err == ErrClosed {
				   dropped++
				  continue
			   }
			  dropped++
			   continue
		   }
		   accepted++
	   }
	  return accepted, dropped
   }

  func RegionProfile(region string) persist.PoolSettings {
	   return conc.RegionPoolOf(region, "").Pool
   }

   func FanIn(ctx context.Context, src []*Service, pipe *conc.Pipeline, limit int) WorkerBatch {
	   var mu sync.Mutex
	  var wg sync.WaitGroup
	  sum := WorkerBatch{}
	   if ctx == nil {
		   ctx = context.Background()
	  }
	   for _, s := range src {
		  s := s
		  wg.Add(1)
		   go func() {
			  defer wg.Done()
			   got := s.PumpInto(ctx, pipe, limit)
			   mu.Lock()
			  sum.Requested += got.Requested
			   sum.Accepted += got.Accepted
			   sum.Failed += got.Failed
			  sum.Dropped += got.Dropped
			   mu.Unlock()
		  }()
	  }
	   wg.Wait()
	   return sum
   }
