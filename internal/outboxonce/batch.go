package outboxonce

import (
	"context"
	"strings"
	"sync"
	"account-assets.kucoin.net/funding/internal/persist"
)

// BatchDeliver 批量投递结果汇总。
type BatchDeliver struct {
	Requested int `json:"requested"`
	Delivered int `json:"delivered"`
	Replay  int `json:"replay"`
	Failed  int `json:"failed"`
	Fence   int `json:"fence"`
	HaltGen int `json:"halt_gen"`
	Results []DeliverResult `json:"results,omitempty"`
}

// DeliverPending 把未投递键按各自围栏投一遍。锁外投递，避免占业务锁。
// 停机世代绑定由 Deliver 内部自动处理；若 Halt 被切换，受影响的键返回 ErrHalt 或 ErrHaltGenCAS。
   func (s *Service) DeliverPending(ctx context.Context, limit int) BatchDeliver {
	  out := BatchDeliver{}
	   if s == nil {
		   return out
	  }
	   if limit <= 0 {
		  limit = 32
	   }
	  pending := s.Pending()
	  if len(pending) > limit {
		   pending = pending[:limit]
	  }
	  out.Requested = len(pending)
	   out.HaltGen = int(s.hgm.CurrentGen())
	   for _, ev := range pending {
		   got, err := s.Deliver(ctx, ev.Key, ev.Epoch, ev.Token)
		  if err != nil {
			   switch err {
			   case ErrFence:
				   out.Fence++
			   default:
				   out.Failed++
			   }
			   continue
		   }
		   if got.Replay {
			  out.Replay++
		   } else {
			   out.Delivered++
		   }
		   out.Results = append(out.Results, got)
	  }
	   return out
  }

// EnqueueMany 同批入箱。单键冲突不回滚已成功键，调用方按结果对账。
  func (s *Service) EnqueueMany(ctx context.Context, rows []Event) ([]Event, []error) {
	   got := make([]Event, 0, len(rows))
	  errs := make([]error, 0, len(rows))
	  for _, r := range rows {
		   ev,
			  err := s.Enqueue(ctx, r.Key, r.Kind, r.Payload)
		  if err != nil {
			  errs = append(errs, err)
			   continue
		   }
		  got = append(got, ev)
	  }
	   return got, errs
  }

type raceSlot struct {
	ev  Event
	err error
}

// RaceEnqueue 同键并发入箱。只允许一个赢家，其余必须 Conflict。
  func RaceEnqueue(ctx context.Context, s *Service, n int, key, kind, payload string) (ok, conflict int) {
	  if s == nil || n <= 0 {
		   return 0, 0
	   }
	  var wg sync.WaitGroup
	   ch := make(chan raceSlot, n)
	   for i := 0; i < n; i++ {
		  wg.Add(1)
		  go func() {
			   defer wg.Done()
			   ev, err := s.Enqueue(ctx, key, kind, payload)
			  ch <- raceSlot{ev: ev, err: err}
		  }()
	   }
	  wg.Wait()
	   close(ch)
	  for sl := range ch {
		   if sl.err == nil {
			   ok++
			   continue
		   }
		  if sl.err == ErrConflict || sl.err == persist.ErrConflict {
			   conflict++
		  }
	  }
	  return ok, conflict
  }

// SamePayload 比较两个事件是否 key/kind/payload 相同。
  func SamePayload(a, b Event) bool {
	   return strings.TrimSpace(a.Key) == strings.TrimSpace(b.Key) &&
		  a.Kind == b.Kind && a.Payload == b.Payload
   }

// RaceDeliver 同键并发 Deliver，用于竞态。
// 返回成功数、被吊销数、fence 失败数。
  func RaceDeliver(ctx context.Context, s *Service, n int, key string, epoch int64, token string) (ok, revoked, fence int) {
	  if s == nil || n <= 0 {
		   return 0, 0, 0
	  }
	  type slot struct {
		   ok  bool
		   err error
	  }
	   ch := make(chan slot, n)
	  var wg sync.WaitGroup
	  for i := 0; i < n; i++ {
		  wg.Add(1)
		   go func() {
			   defer wg.Done()
			  res, err := s.Deliver(ctx, key, epoch, token)
			   ch <- slot{ok: err == nil && res.Delivered && !res.Replay,
				  err: err}
		  }()
	  }
	   wg.Wait()
	  close(ch)
	   for sl := range ch {
		  if sl.ok {
			   ok++
			  continue
		   }
		  switch sl.err {
		  case ErrHalt, ErrHaltGenCAS, ErrHaltGenRevoked, ErrStaleFenceRevive:
			   revoked++
		   case ErrFence:
			   fence++
		  }
	   }
	  return ok, revoked, fence
   }
