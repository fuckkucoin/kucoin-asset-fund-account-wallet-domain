package utabook

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

type persistSQL = persist.SQLDB

   func (b *Book) BindSQL(db *persist.SQLDB) *Book {
	  if b != nil {
		  b.sql = db
		  b.loadSQL()
	  }
	  return b
  }

   func (b *Book) persistMode(userID, mode string) error {
	  if b == nil || b.sql == nil || b.sql.DB == nil {
		   return nil
	   }
	  return sqlrepo.NewFundingSubjects(b.sql).PutMode(context.Background(), sqlrepo.UTAMode{UserID: userID, Mode: mode})
   }

   func (b *Book) persistLev(userID string, in Leverage) error {
	  if b == nil || b.sql == nil || b.sql.DB == nil {
		  return nil
	   }
	   return sqlrepo.NewFundingSubjects(b.sql).PutLeverage(context.Background(), sqlrepo.UTALeverage{
		  UserID: userID, Symbol: in.Symbol, Currency: in.Currency,
		   MarginMode: in.MarginMode,
		  TradeType:  in.TradeType, Leverage: in.Leverage,
	   })
   }

func (b *Book) interestFromSQL(userID, currency string) ([]InterestRow, bool) {
	if b == nil || b.sql == nil || b.sql.DB == nil {
		return nil, false
	}
	rows, err := sqlrepo.NewFundingSubjects(b.sql).ListInterest(context.Background())
	if err != nil {
		b.noteHydrate(err)
		return nil, false
	}
	out := make([]InterestRow, 0)
	for _, row := range rows {
		if row.UserID != userID {
			continue
		}
		if currency != "" && row.Currency != currency {
			continue
		}
		out = append(out, InterestRow{Currency: row.Currency, Amount: row.Amount, CreatedAt: row.CreatedAt})
	}
	return out, true
}

   func (b *Book) persistInterest(userID string, row InterestRow) error {
	  if b == nil || b.sql == nil || b.sql.DB == nil {
		  return nil
	   }
	   id := userID + "|" + row.Currency + "|" + strconv.FormatInt(row.CreatedAt, 10)
	  return sqlrepo.NewFundingSubjects(b.sql).PutInterest(context.Background(), sqlrepo.UTAInterest{
		   ID: id, UserID: userID, Currency: row.Currency, Amount: row.Amount, CreatedAt: row.CreatedAt,
	   })
  }

   func (b *Book) loadSQL() {
	   if b == nil || b.sql == nil || b.sql.DB == nil {
		   return
	   }
	  repo := sqlrepo.NewFundingSubjects(b.sql)
	   if modes, err := repo.ListModes(context.Background()); err != nil {
		  b.noteHydrate(err)
	  } else {
		   b.mu.Lock()
		  for _, m := range modes {
			  b.mode[strings.TrimSpace(m.UserID)] = m.Mode
		   }
		  b.mu.Unlock()
	  }
	  if levs, err := repo.ListLeverage(context.Background()); err != nil {
		  b.noteHydrate(err)
	  } else {
		   b.mu.Lock()
		  for _, l := range levs {
			  key := l.UserID + "|" + l.Symbol + "|" + l.Currency + "|" + l.MarginMode + "|" + l.TradeType
			   b.lev[key] = Leverage{Symbol: l.Symbol, Currency: l.Currency, Leverage: l.Leverage, MarginMode: l.MarginMode, TradeType: l.TradeType}
		  }
		   b.mu.Unlock()
	  }
	   if hist, err := repo.ListInterest(context.Background()); err != nil {
		  b.noteHydrate(err)
	  } else {
		   b.mu.Lock()
		   for _, row := range hist {
			   b.hist[row.UserID] = append(b.hist[row.UserID], InterestRow{Currency: row.Currency, Amount: row.Amount, CreatedAt: row.CreatedAt})
		   }
		   b.mu.Unlock()
	   }
	   if tiers, err := repo.ListRateTiers(context.Background()); err == nil {
		  for _, t := range tiers {
			  b.RateCatalog().Put(RateTier{
				  Currency: t.Currency, Tier: t.Tier, RateHourly: t.RateHourly, RateDaily: t.RateDaily,
				  BorrowLimit: t.BorrowLimit, InterestFreeLimit: t.InterestFreeLimit, HoldLimit: t.HoldLimit,
			  })
		  }
	   } else {
		  b.noteHydrate(err)
	   }
	  if rows, err := repo.ListLiabilities(context.Background(), ""); err != nil {
		  b.noteHydrate(err)
	  } else {
		  b.mu.Lock()
		   for _, r := range rows {
			  row := LiabilityRow{
				  Currency: r.Currency, MarginMode: r.MarginMode, Symbol: r.Symbol,
				  Principal: r.Principal, Interest: r.Interest, AccruedInterest: r.AccruedInterest,
				   BorrowEnabled: r.BorrowEnabled,
				  UpdatedAt:     r.UpdatedAt,
			   }
			  row.TotalLiability = sumDecimal(sumDecimal(row.Principal,
				   row.Interest), row.AccruedInterest)
			  b.liab[r.UserID] = append(b.liab[r.UserID], row)
		  }
		   b.mu.Unlock()
	  }
	   if cols, err := repo.ListCollateral(context.Background()); err != nil {
		  b.noteHydrate(err)
	  } else {
		  b.mu.Lock()
		  for _, c := range cols {
			   b.collateral[c.Currency] = CollateralRule{
				   Currency: c.Currency, Haircut: c.Haircut, CollateralEnabled: c.CollateralEnabled,
				  MaxLTV: c.MaxLTV, UpdatedAt: c.UpdatedAt,
			  }
		  }
		  b.mu.Unlock()
	   }
	  if risks, err := repo.ListRisk(context.Background()); err != nil {
		  b.noteHydrate(err)
	  } else {
		  b.mu.Lock()
		   for _, r := range risks {
			  b.risk[r.UserID] = RiskView{
				  UserID: r.UserID, MarginLevel: r.MarginLevel, TotalEquity: r.TotalEquity,
				   TotalLiability: r.TotalLiability, TotalCollateral: r.TotalCollateral,
				  RiskGrade:      r.RiskGrade,
				   CrossDebtRatio: r.CrossDebtRatio, UpdatedAt: r.UpdatedAt,
			   }
		   }
		   b.mu.Unlock()
	  }
	   if cross, err := repo.ListCrossMargin(context.Background(), ""); err != nil {
		  b.noteHydrate(err)
	  } else {
		  b.mu.Lock()
		  for _, r := range cross {
			   b.cross[r.UserID] = append(b.cross[r.UserID], CrossAsset{
				  Currency: r.Currency, Total: r.Total, Available: r.Available, Hold: r.Hold,
				   Liability: r.Liability,
				   MaxBorrow: r.MaxBorrow, BorrowEnabled: r.BorrowEnabled,
				  TransferEnabled: r.TransferEnabled, UpdatedAt: r.UpdatedAt,
			  })
		  }
		   b.mu.Unlock()
	   }
  }

  func (b *Book) sqlReady() bool {
	  return b != nil && b.sql != nil && b.sql.DB != nil
  }

  func (b *Book) RequireMode(userID string) (string, error) {
	  userID = strings.TrimSpace(userID)
	  if b.sqlReady() {
		  m, err := sqlrepo.NewFundingSubjects(b.sql).GetMode(context.Background(), userID)
		  if errors.Is(err, sqlrepo.ErrNotFound) {
			  return "UNIFIED", nil
		  }
		  if err != nil {
			  return "", err
		  }
		  if strings.TrimSpace(m.Mode) == "" {
			  return "UNIFIED", nil
		  }
		  b.mu.Lock()
		  b.mode[userID] = m.Mode
		  b.mu.Unlock()
		  return m.Mode, nil
	  }
	  if b == nil {
		  return "UNIFIED", nil
	  }
	  b.mu.Lock()
	  defer b.mu.Unlock()
	  if m := b.mode[userID]; m != "" {
		  return m, nil
	  }
	  return "UNIFIED", nil
  }

  func (b *Book) RequireLeverage(userID, symbol, currency, tradeType, marginMode string) ([]Leverage, error) {
	  userID = strings.TrimSpace(userID)
	  symbol = strings.ToUpper(strings.TrimSpace(symbol))
	  currency = strings.ToUpper(strings.TrimSpace(currency))
	  tradeType = strings.ToUpper(strings.TrimSpace(tradeType))
	  marginMode = strings.ToUpper(strings.TrimSpace(marginMode))
	  if b.sqlReady() {
		  rows, err := sqlrepo.NewFundingSubjects(b.sql).ListUserLeverage(context.Background(), userID)
		  if err != nil {
			  return nil, err
		  }
		  b.mu.Lock()
		  for k := range b.lev {
			  if strings.HasPrefix(k, userID+"|") {
				  delete(b.lev, k)
			  }
		  }
		  for _, l := range rows {
			  key := l.UserID + "|" + l.Symbol + "|" + l.Currency + "|" + l.MarginMode + "|" + l.TradeType
			  b.lev[key] = Leverage{Symbol: l.Symbol, Currency: l.Currency, Leverage: l.Leverage, MarginMode: l.MarginMode, TradeType: l.TradeType}
		  }
		  b.mu.Unlock()
	  }
	  if b == nil {
		  return []Leverage{}, nil
	  }
	  return b.Leverage(userID, symbol, currency, tradeType, marginMode), nil
  }

  func (b *Book) refreshRates() error {
	  if !b.sqlReady() {
		  return nil
	  }
	  tiers, err := sqlrepo.NewFundingSubjects(b.sql).ListRateTiers(context.Background())
	  if err != nil {
		  return err
	  }
	  cat := NewRateCatalog()
	  for _, t := range tiers {
		  cat.Put(RateTier{
			  Currency: t.Currency, Tier: t.Tier, RateHourly: t.RateHourly, RateDaily: t.RateDaily,
			  BorrowLimit: t.BorrowLimit, InterestFreeLimit: t.InterestFreeLimit, HoldLimit: t.HoldLimit,
		  })
	  }
	  b.mu.Lock()
	  b.rateCatalog = cat
	  b.mu.Unlock()
	  return nil
  }

  func (b *Book) RequireInterestLimits(currency string) (InterestLimit, error) {
	  if b == nil {
		  return zeroInterestLimit(), nil
	  }
	  if err := b.refreshRates(); err != nil {
		  return InterestLimit{}, err
	  }
	  if b.rateCatalog == nil {
		  return zeroInterestLimit(), nil
	  }
	  return b.rateCatalog.InterestLimits(currency), nil
  }

  func (b *Book) noteHydrate(err error) {
	  if b == nil || err == nil {
		  return
	  }
	  b.mu.Lock()
	  if b.hydrateErr == "" {
		  b.hydrateErr = err.Error()
	  } else {
		  b.hydrateErr = b.hydrateErr + "; " + err.Error()
	  }
	  b.mu.Unlock()
  }

   func (b *Book) persistLiability(userID string, row LiabilityRow) error {
	  if b == nil || b.sql == nil || b.sql.DB == nil {
		   return nil
	  }
	  return sqlrepo.NewFundingSubjects(b.sql).PutLiability(context.Background(), sqlrepo.UTALiabilityRow{
		  UserID: userID, Currency: row.Currency, MarginMode: row.MarginMode, Symbol: row.Symbol,
		   Principal: row.Principal, Interest: row.Interest, AccruedInterest: row.AccruedInterest,
		  BorrowEnabled: row.BorrowEnabled, UpdatedAt: row.UpdatedAt,
	  })
  }

   func (b *Book) persistCollateral(rule CollateralRule) error {
	   if b == nil || b.sql == nil || b.sql.DB == nil {
		  return nil
	  }
	  return sqlrepo.NewFundingSubjects(b.sql).PutCollateral(context.Background(), sqlrepo.UTACollateralRow{
		   Currency: rule.Currency, Haircut: rule.Haircut, CollateralEnabled: rule.CollateralEnabled,
		   MaxLTV: rule.MaxLTV,
		   UpdatedAt: rule.UpdatedAt,
	  })
   }

   func (b *Book) persistRepay(userID string, row RepayRow) error {
	  if b == nil || b.sql == nil || b.sql.DB == nil {
		   return nil
	   }
	   return sqlrepo.NewFundingSubjects(b.sql).PutRepay(context.Background(), sqlrepo.UTARepayRow{
		   ID:     row.ID,
		  UserID: userID, Currency: row.Currency, MarginMode: row.MarginMode, Symbol: row.Symbol,
		  RepayType: row.RepayType, Principal: row.Principal, Interest: row.Interest, CreatedAt: row.CreatedAt,
	  })
   }

  func (b *Book) persistRisk(view RiskView) error {
	   if b == nil || b.sql == nil || b.sql.DB == nil {
		   return nil
	  }
	   return sqlrepo.NewFundingSubjects(b.sql).PutRisk(context.Background(), sqlrepo.UTARiskRow{
		   UserID: view.UserID,
		   MarginLevel: view.MarginLevel, TotalEquity: view.TotalEquity,
		  TotalLiability: view.TotalLiability, TotalCollateral: view.TotalCollateral,
		   RiskGrade: view.RiskGrade, CrossDebtRatio: view.CrossDebtRatio, UpdatedAt: view.UpdatedAt,
	   })
   }

   func (b *Book) persistRateTier(t RateTier) error {
	   if b == nil || b.sql == nil || b.sql.DB == nil {
		   return nil
	  }
	  return sqlrepo.NewFundingSubjects(b.sql).PutRateTier(context.Background(), sqlrepo.UTARateTier{
		  Currency: t.Currency, Tier: t.Tier, RateHourly: t.RateHourly, RateDaily: t.RateDaily,
		   BorrowLimit: t.BorrowLimit, InterestFreeLimit: t.InterestFreeLimit, HoldLimit: t.HoldLimit,
		   UpdatedAt: time.Now().UTC().UnixMilli(),
	   })
  }

  func (b *Book) persistCross(userID string, row CrossAsset) error {
	   if b == nil || b.sql == nil || b.sql.DB == nil {
		   return nil
	  }
	  return sqlrepo.NewFundingSubjects(b.sql).PutCrossMargin(context.Background(), sqlrepo.UTACrossMarginRow{
		  UserID: userID, Currency: row.Currency, Total: row.Total, Available: row.Available, Hold: row.Hold,
		  Liability: row.Liability,
		  MaxBorrow: row.MaxBorrow, BorrowEnabled: row.BorrowEnabled,
		   TransferEnabled: row.TransferEnabled, UpdatedAt: row.UpdatedAt,
	   })
  }
