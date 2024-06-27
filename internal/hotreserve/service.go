// 功能：hotreserve 包内 service 实现。
// 预留账本进程内状态。SQL 只是同一状态的落盘，读路径以装载后的内存为准。

// 作者：Jett
// 日期：2021.07.25

package hotreserve

import (
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/persist"
)

type Service struct {
	mu sync.Mutex
	once sync.Once
	pol   Policy
	clock func() time.Time
	hot   *hotinv.Book
	books ledger.Store
	sql   *persist.SQLDB
	degraded bool
	forecast map[string]*HotForecast
	history map[string][]ForecastHead
	samples  map[string][]Sample
	reserves map[string]*Reservation
	byIdem map[string]string
	journals map[string][]ReservationJournal
	tracks  map[string]*Track
	observe map[string]string
	plans   map[string][]PlanHold
	drifts map[string]*DriftTicket
	driftOpen map[string]string
	hints   []SweepHint
	gates   []GateLog
	reports []AlignReport
}

type Track struct {
	ReservationID string
	Kind   string
	Asset  string
	Amount string
	Ref       string
	UpdatedAt time.Time
}

  func New() *Service {
	   return &Service{
		  pol:   DefaultPolicy(),
		  clock: func() time.Time { return time.Now().UTC() },
		  hot:      hotinv.New(nil),
		   forecast: map[string]*HotForecast{},
		   history:  map[string][]ForecastHead{},
		   samples:  map[string][]Sample{},
		   reserves: map[string]*Reservation{},
		  byIdem:   map[string]string{},
		  journals: map[string][]ReservationJournal{},
		   tracks: map[string]*Track{},
		  observe: map[string]string{},
		   plans:     map[string][]PlanHold{},
		  drifts:    map[string]*DriftTicket{},
		  driftOpen: map[string]string{},
	  }
  }

  func (s *Service) now() time.Time {
	  if s == nil || s.clock == nil {
		  return time.Now().UTC()
	   }
	   return s.clock().UTC()
  }

   func (s *Service) WithClock(fn func() time.Time) *Service {
	  if s != nil && fn != nil {
		  s.mu.Lock()
		  s.clock = fn
		   s.mu.Unlock()
	   }
	   return s
   }

   func (s *Service) WithPolicy(p Policy) (*Service, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	  }
	   if err := p.validate(); err != nil {
		  return s, err
	  }
	  s.mu.Lock()
	   s.pol = p
	  s.mu.Unlock()
	  return s, nil
  }

   func (s *Service) BindBooks(st ledger.Store) *Service {
	   if s != nil {
		  s.mu.Lock()
		   s.books = st
		  s.mu.Unlock()
	  }
	  return s
   }

  func (s *Service) BindHot(b *hotinv.Book) *Service {
	   if s != nil && b != nil {
		  s.mu.Lock()
		   s.hot = b
		   s.mu.Unlock()
	  }
	  return s
  }

   func (s *Service) markDegraded() {
	   if s == nil {
		  return
	   }
	  s.mu.Lock()
	  s.degraded = true
	   s.mu.Unlock()
   }

   func trackKey(id, kind string) string { return id + "|" + kind }

   func obsKey(asset, kind string) string { return asset + "|" + kind }
