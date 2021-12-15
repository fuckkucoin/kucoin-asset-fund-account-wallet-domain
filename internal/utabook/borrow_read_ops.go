package utabook

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

type LoadReport struct {
	RateTiers int
	Liabilities int
	Collateral int
	RiskRows     int
	CrossRows    int
	HydrateError string
}

   func (b *Book) ReloadBorrowRead(ctx context.Context) LoadReport {
	  rep := LoadReport{}
	   if b == nil || b.sql == nil || b.sql.DB == nil {
		   return rep
	  }
	   repo := sqlrepo.NewFundingSubjects(b.sql)
	  if tiers, err := repo.ListRateTiers(ctx); err == nil {
		   b.RateCatalog().ReplaceAll(nil)
		   for _, t := range tiers {
			  b.RateCatalog().Put(RateTier{
				   Currency: t.Currency,
				  Tier:     t.Tier, RateHourly: t.RateHourly, RateDaily: t.RateDaily,
				   BorrowLimit: t.BorrowLimit, InterestFreeLimit: t.InterestFreeLimit, HoldLimit: t.HoldLimit,
			  })
		   }
		   rep.RateTiers = len(tiers)
	  } else {
		   rep.HydrateError = err.Error()
	  }
	  if rows, err := repo.ListLiabilities(ctx, ""); err == nil {
		   b.mu.Lock()
		   b.liab = map[string][]LiabilityRow{}
		  b.mu.Unlock()
		  for _, r := range rows {
			   if _, err := b.UpsertLiability(r.UserID, LiabilityRow{
				   Currency: r.Currency, MarginMode: r.MarginMode, Symbol: r.Symbol,
				  Principal: r.Principal,
				   Interest:  r.Interest, AccruedInterest: r.AccruedInterest,
				  BorrowEnabled: r.BorrowEnabled, UpdatedAt: r.UpdatedAt,
			  }); err != nil && rep.HydrateError == "" {
				  rep.HydrateError = err.Error()
			  }
		  }
		   rep.Liabilities = len(rows)
	  }
	  if cols, err := repo.ListCollateral(ctx); err == nil {
		  for _, c := range cols {
			   if _, err := b.PutCollateral(CollateralRule{
				   Currency: c.Currency,
				   Haircut:  c.Haircut, CollateralEnabled: c.CollateralEnabled,
				  MaxLTV: c.MaxLTV, UpdatedAt: c.UpdatedAt,
			   }); err != nil && rep.HydrateError == "" {
				  rep.HydrateError = err.Error()
			  }
		   }
		  rep.Collateral = len(cols)
	  }
	  if risks, err := repo.ListRisk(ctx); err == nil {
		   for _, r := range risks {
			  if _, err := b.UpsertRisk(r.UserID, RiskView{
				   UserID: r.UserID,
				  MarginLevel: r.MarginLevel, TotalEquity: r.TotalEquity,
				   TotalLiability: r.TotalLiability, TotalCollateral: r.TotalCollateral,
				   RiskGrade: r.RiskGrade, CrossDebtRatio: r.CrossDebtRatio, UpdatedAt: r.UpdatedAt,
			  }); err != nil && rep.HydrateError == "" {
				  rep.HydrateError = err.Error()
			  }
		  }
		  rep.RiskRows = len(risks)
	  }
	   if cross, err := repo.ListCrossMargin(ctx, ""); err == nil {
		  for _, r := range cross {
			   if _, err := b.UpsertCross(r.UserID, CrossAsset{
				  Currency: r.Currency, Total: r.Total, Available: r.Available, Hold: r.Hold,
				  Liability: r.Liability, MaxBorrow: r.MaxBorrow, BorrowEnabled: r.BorrowEnabled,
				  TransferEnabled: r.TransferEnabled, UpdatedAt: r.UpdatedAt,
			  }); err != nil && rep.HydrateError == "" {
				  rep.HydrateError = err.Error()
			  }
		  }
		   rep.CrossRows = len(cross)
	   }
	  return rep
  }

  func (b *Book) BorrowReadHealth() map[string]any {
	   rep := LoadReport{HydrateError: b.Health()}
	   if b != nil {
		   rep.RateTiers = len(b.ListRateTiers(""))
		   b.mu.Lock()
		   for _, rows := range b.liab {
			  rep.Liabilities += len(rows)
		   }
		  rep.Collateral = len(b.collateral)
		   rep.RiskRows = len(b.risk)
		  for _, rows := range b.cross {
			   rep.CrossRows += len(rows)
		   }
		   b.mu.Unlock()
	  }
	  return map[string]any{
		  "rate_tiers": rep.RateTiers, "liability_rows": rep.Liabilities,
		  "collateral_rules": rep.Collateral, "risk_snapshots": rep.RiskRows,
		   "cross_rows": rep.CrossRows, "hydrate_err": rep.HydrateError,
	   }
  }

   func (b *Book) AssertReadOnlyCurrency(currency string) error {
	   currency = strings.ToUpper(strings.TrimSpace(currency))
	  if currency == "" {
		  return errReadCurrency
	   }
	  if !b.CollateralRule(currency).CollateralEnabled && b.InterestLimits(currency).BorrowLimitTotal == "0" {
		  // 仍允许读，仅标记未开放抵押
		   return nil
	  }
	  return nil
  }

type readErr string

   func (e readErr) Error() string { return string(e) }

var errReadCurrency = readErr("uta currency required")
