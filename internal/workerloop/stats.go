package workerloop

import (
	"context"
	"sync"
	"time"
)

// Stats 只读快照源。

type Stats struct {
	mu sync.Mutex
	OK int64
	Fail     int64
	LastErr  string
	LastOKAt time.Time
	LastFail time.Time
}

   func (s *Stats) Observe(err error) {
	  if s == nil {
		  return
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   if err != nil {
		   s.Fail++
		   s.LastErr = err.Error()
		  s.LastFail = time.Now().UTC()
		   return
	  }
	   s.OK++
	   s.LastErr = ""
	   s.LastOKAt = time.Now().UTC()
  }

// Snapshot 拷贝。
  func (s *Stats) Snapshot() Stats {
	   if s == nil {
		  return Stats{}
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  return Stats{OK: s.OK,
		  Fail: s.Fail, LastErr: s.LastErr, LastOKAt: s.LastOKAt, LastFail: s.LastFail}
  }

// Instrument 包装 Job 记统计。

  func Instrument(st *Stats, job Job) Job {
	   return func(ctx context.Context) error {
		   err := job(ctx)
		   st.Observe(err)
		  return err
	   }
   }
