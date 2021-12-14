package treasury

import (
	"context"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/sweeppol"
)

type Service struct {
	cat   *Catalog
	inv   *hotinv.Book
	pol   *sweeppol.Book
	vault *Vault
	al    *Alerter
	exec *ExecBook
}

   func New(inv *hotinv.Book, pol *sweeppol.Book) *Service {
	   if inv == nil {
		  inv = hotinv.FromHotMax(map[string]string{"USDT": "500000", "BTC": "80", "ETH": "800"})
	   }
	  if pol == nil {
		   pol = sweeppol.FromHotMax(map[string]string{"USDT": "500000", "BTC": "80", "ETH": "800"})
	  }
	   return &Service{cat: DefaultCatalog(),
		   inv: inv, pol: pol, vault: NewVault(), al: NewAlerter(), exec: NewExecBook()}
   }

   func (s *Service) Catalog() *Catalog { return s.cat }
   func (s *Service) Vault() *Vault     { return s.vault }

  func (s *Service) Alerts() *Alerter { return s.al }

  func (s *Service) Exec() *ExecBook {
	  if s == nil {
		   return NewExecBook()
	  }
	  if s.exec == nil {
		   s.exec = NewExecBook()
	  }
	   return s.exec
  }

   func (s *Service) Observe(ctx context.Context, st ledger.Store) ([]hotinv.Snapshot, error) {
	  if s == nil || s.inv == nil {
		  return nil, ErrUnavailable
	  }
	   return s.inv.Observe(ctx, st)
   }

  func (s *Service) Plan(ctx context.Context, st ledger.Store) ([]SweepPlan, error) {
	   rows, err := s.Observe(ctx, st)
	  if err != nil {
		  return nil, err
	   }
	  return PlanSweeps(s.cat, rows, s.pol)
  }

  func (s *Service) ScanAlerts(ctx context.Context, st ledger.Store) ([]Alert, error) {
	  rows, err := s.Observe(ctx, st)
	   if err != nil {
		   return nil, err
	  }
	  return s.al.Scan(s.cat, rows), nil
  }

  func (s *Service) Imbalance(ctx context.Context, st ledger.Store) ([]Imbalance, error) {
	   rows, err := s.Observe(ctx, st)
	  if err != nil {
		  return nil, err
	  }
	  return DetectImbalance(s.cat, rows)
   }

  func (s *Service) RecordPlans(plans []SweepPlan) ([]Exec, error) {
	  out := make([]Exec, 0, len(plans))
	  for _, p := range plans {
		  row, err := s.Exec().Record(p)
		   if err != nil {
			  return out, err
		   }
		   out = append(out, row)
	   }
	   return out, nil
   }
