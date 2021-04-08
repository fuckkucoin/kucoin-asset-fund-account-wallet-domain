package deppartial

import (
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/fsm"
)

// Service 入金观察批次部分失败与重复贷记封堵服务。
type Service struct {
	mu sync.Mutex
	store *Store
	clock func() time.Time
	leaseTTL time.Duration
	batchFSM *fsm.Machine[BatchResidueState,
		string]
	residueFSM *fsm.Machine[ResidueState, string]
	leaseFSM   *fsm.Machine[LeaseStatus, string]
	proofFSM *fsm.Machine[ProofStatus, string]
}

type Option func(*Service)

   func WithClock(fn func() time.Time) Option {
	  return func(s *Service) { s.clock = fn }
}

   func WithLeaseTTL(d time.Duration) Option {
	  return func(s *Service) { s.leaseTTL = d }
  }

  func New(opts ...Option) *Service {
	  svc := &Service{
		  store: NewStore(),
		   clock:      func() time.Time { return time.Now().UTC() },
		  leaseTTL:   30 * time.Second,
		   batchFSM:   BatchResidueFSM(),
		  residueFSM: ResidueFSM(),
		   leaseFSM:   LeaseFSM(),
		  proofFSM: ProofFSM(),
	   }
	   for _, o := range opts {
		   o(svc)
	   }
	  return svc
  }

  func (svc *Service) now() time.Time {
	   if svc == nil || svc.clock == nil {
		  return time.Now().UTC()
	   }
	   return svc.clock()
   }

   func requireActor(s string) error {
	   if strings.TrimSpace(s) == "" {
		  return ErrActor
	  }
	  return nil
   }

// RegisterBatch 注册批次到残余管理器。
  func (svc *Service) RegisterBatch(batchID string, totalItems int) error {
	  if svc == nil {
		   return ErrUnavailable
	  }
	   if strings.TrimSpace(batchID) == "" {
		  return ErrBatchNotFound
	   }
	  if totalItems <= 0 {
		  return ErrBatchNotFound
	  }
	   svc.store.RegisterBatch(batchID, totalItems)
	   return nil
   }

// GetBatchState 获取批次残余状态。

  func (svc *Service) GetBatchState(batchID string) (BatchResidueState, error) {
	  if svc == nil {
		   return "", ErrUnavailable
	  }
	   s, ok := svc.store.GetBatchState(batchID)
	   if !ok {
		  return "", ErrBatchNotFound
	   }
	   return s, nil
  }

// GetView 获取批次残余聚合视图。

  func (svc *Service) GetView(batchID string) (BatchResidueView, error) {
	  if svc == nil {
		  return BatchResidueView{}, ErrUnavailable
	   }
	   if _, ok := svc.store.GetBatchState(batchID); !ok {
		  return BatchResidueView{}, ErrBatchNotFound
	  }
	  return svc.store.BuildView(batchID), nil
   }

// ListResidues 列出批次所有残余行。
  func (svc *Service) ListResidues(batchID string) ([]ObserveResidue, error) {
	  if svc == nil {
		   return nil, ErrUnavailable
	   }
	   if _, ok := svc.store.GetBatchState(batchID); !ok {
		   return nil, ErrBatchNotFound
	   }
	  return svc.store.ListResiduesByBatch(batchID), nil
   }

// GetResidue 查询单条残余行。
   func (svc *Service) GetResidue(residueID string) (ObserveResidue, error) {
	  if svc == nil {
		  return ObserveResidue{}, ErrUnavailable
	   }
	  r, ok := svc.store.GetResidue(residueID)
	   if !ok {
		   return ObserveResidue{},
			  ErrResidueNotOpen
	  }
	  return *r, nil
   }

// AllBatchIDs 返回所有注册批次 ID。
   func (svc *Service) AllBatchIDs() []string {
	   if svc == nil {
		  return nil
	   }
	   return svc.store.AllBatchIDs()
  }

// BatchesInState 返回指定状态的批次列表。

   func (svc *Service) BatchesInState(s BatchResidueState) []string {
	  if svc == nil {
		   return nil
	  }
	   return svc.store.BatchesInState(s)
   }
