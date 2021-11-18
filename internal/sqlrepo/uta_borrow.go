package sqlrepo

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

type UTARateTier struct {
	Currency string
	Tier       int
	RateHourly string
	RateDaily  string
	BorrowLimit string
	InterestFreeLimit string
	HoldLimit string
	UpdatedAt int64
}

type UTALiabilityRow struct {
	UserID string
	Currency   string
	MarginMode string
	Symbol     string
	Principal  string
	Interest string
	AccruedInterest string
	BorrowEnabled   bool
	UpdatedAt       int64
}

type UTACollateralRow struct {
	Currency string
	Haircut string
	CollateralEnabled bool
	MaxLTV string
	UpdatedAt int64
}

type UTARepayRow struct {
	ID         string
	UserID     string
	Currency   string
	MarginMode string
	Symbol string
	RepayType string
	Principal string
	Interest  string
	CreatedAt int64
}

type UTARiskRow struct {
	UserID      string
	MarginLevel string
	TotalEquity string
	TotalLiability  string
	TotalCollateral string
	RiskGrade string
	CrossDebtRatio string
	UpdatedAt      int64
}

type UTACrossMarginRow struct {
	UserID   string
	Currency string
	Total    string
	Available string
	Hold      string
	Liability string
	MaxBorrow string
	BorrowEnabled bool
	TransferEnabled bool
	UpdatedAt       int64
}

type UTARepayFilter struct {
	UserID     string
	Currency   string
	MarginMode string
	Symbol   string
	AfterMs  int64
	BeforeMs int64
	Limit    int
	Offset   int
}

   func (r *FundingSubjects) PutRateTier(ctx context.Context, t UTARateTier) error {
	   if !sqlOn(r.db) {
		  return ErrUnavailable
	   }
	  t.Currency = strings.ToUpper(strings.TrimSpace(t.Currency))
	  if t.Currency == "" {
		  return ErrConflict
	  }
	   q := `INSERT INTO aa_uta_rate_ladder (currency, tier, rate_hourly, rate_daily, borrow_limit, interest_free_limit, hold_limit, updated_at)
VALUES (?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE rate_hourly=VALUES(rate_hourly), rate_daily=VALUES(rate_daily), borrow_limit=VALUES(borrow_limit),
interest_free_limit=VALUES(interest_free_limit), hold_limit=VALUES(hold_limit), updated_at=VALUES(updated_at)`
	   if r.db.Dialect == persist.Postgres {
		  q = `INSERT INTO aa_uta_rate_ladder (currency, tier, rate_hourly, rate_daily, borrow_limit, interest_free_limit, hold_limit, updated_at)
VALUES (?,?,?,?,?,?,?,?)
ON CONFLICT (currency, tier) DO UPDATE SET rate_hourly=EXCLUDED.rate_hourly, rate_daily=EXCLUDED.rate_daily,
borrow_limit=EXCLUDED.borrow_limit, interest_free_limit=EXCLUDED.interest_free_limit, hold_limit=EXCLUDED.hold_limit, updated_at=EXCLUDED.updated_at`
	   }
	   _, err := r.db.Conn(ctx).ExecContext(ctx, q, t.Currency, t.Tier, t.RateHourly, t.RateDaily, t.BorrowLimit, t.InterestFreeLimit, t.HoldLimit, t.UpdatedAt)
	  return err
   }

  func (r *FundingSubjects) ListRateTiers(ctx context.Context) ([]UTARateTier, error) {
	  if !sqlOn(r.db) {
		   return nil, ErrUnavailable
	  }
	   rows, err := r.db.Conn(ctx).QueryContext(ctx, `SELECT currency, tier, rate_hourly, rate_daily, borrow_limit, interest_free_limit, hold_limit, updated_at FROM aa_uta_rate_ladder ORDER BY currency, tier`)
	   if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	  out := make([]UTARateTier, 0)
	  for rows.Next() {
		  var t UTARateTier
		   if err := rows.Scan(&t.Currency, &t.Tier, &t.RateHourly, &t.RateDaily, &t.BorrowLimit, &t.InterestFreeLimit, &t.HoldLimit, &t.UpdatedAt); err != nil {
			   return nil, err
		   }
		  out = append(out, t)
	   }
	   return out, rows.Err()
   }

   func (r *FundingSubjects) PutLiability(ctx context.Context, row UTALiabilityRow) error {
	  if !sqlOn(r.db) {
		   return ErrUnavailable
	   }
	  row.UserID = strings.TrimSpace(row.UserID)
	  row.Currency = strings.ToUpper(strings.TrimSpace(row.Currency))
	  row.MarginMode = strings.ToUpper(strings.TrimSpace(row.MarginMode))
	   row.Symbol = strings.ToUpper(strings.TrimSpace(row.Symbol))
	  if row.UserID == "" || row.Currency == "" {
		   return ErrConflict
	  }
	  if row.MarginMode == "" {
		   row.MarginMode = "CROSS"
	   }
	   be := 0
	   if row.BorrowEnabled {
		   be = 1
	   }
	   q := `INSERT INTO aa_uta_liability (user_id, currency, margin_mode, symbol, principal, interest, accrued_interest, borrow_enabled, updated_at)
VALUES (?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE principal=VALUES(principal), interest=VALUES(interest), accrued_interest=VALUES(accrued_interest),
borrow_enabled=VALUES(borrow_enabled), updated_at=VALUES(updated_at)`
	   if r.db.Dialect == persist.Postgres {
		   q = `INSERT INTO aa_uta_liability (user_id, currency, margin_mode, symbol, principal, interest, accrued_interest, borrow_enabled, updated_at)
VALUES (?,?,?,?,?,?,?,?,?)
ON CONFLICT (user_id, currency, margin_mode, symbol) DO UPDATE SET principal=EXCLUDED.principal, interest=EXCLUDED.interest,
accrued_interest=EXCLUDED.accrued_interest, borrow_enabled=EXCLUDED.borrow_enabled, updated_at=EXCLUDED.updated_at`
	  }
	  _, err := r.db.Conn(ctx).ExecContext(ctx, q, row.UserID, row.Currency, row.MarginMode, row.Symbol, row.Principal, row.Interest, row.AccruedInterest, be, row.UpdatedAt)
	   return err
  }

   func (r *FundingSubjects) ListLiabilities(ctx context.Context, userID string) ([]UTALiabilityRow, error) {
	  if !sqlOn(r.db) {
		  return nil, ErrUnavailable
	   }
	   userID = strings.TrimSpace(userID)
	   q := `SELECT user_id, currency, margin_mode, symbol, principal, interest, accrued_interest, borrow_enabled, updated_at FROM aa_uta_liability`
	  args := []any{}
	   if userID != "" {
		  q += ` WHERE user_id=?`
		   args = append(args, userID)
	   }
	  rows, err := r.db.Conn(ctx).QueryContext(ctx, q, args...)
	  if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	   out := make([]UTALiabilityRow, 0)
	   for rows.Next() {
		   var row UTALiabilityRow
		  var be int
		   if err := rows.Scan(&row.UserID, &row.Currency, &row.MarginMode, &row.Symbol, &row.Principal, &row.Interest, &row.AccruedInterest, &be, &row.UpdatedAt); err != nil {
			   return nil, err
		  }
		  row.BorrowEnabled = be != 0
		  out = append(out, row)
	  }
	  return out, rows.Err()
   }

   func (r *FundingSubjects) PutCollateral(ctx context.Context, row UTACollateralRow) error {
	  if !sqlOn(r.db) {
		  return ErrUnavailable
	  }
	   row.Currency = strings.ToUpper(strings.TrimSpace(row.Currency))
	   if row.Currency == "" {
		   return ErrConflict
	   }
	   ce := 0
	   if row.CollateralEnabled {
		   ce = 1
	  }
	   q := `INSERT INTO aa_uta_collateral (currency, haircut, collateral_enabled, max_ltv, updated_at)
VALUES (?,?,?,?,?)
ON DUPLICATE KEY UPDATE haircut=VALUES(haircut), collateral_enabled=VALUES(collateral_enabled), max_ltv=VALUES(max_ltv), updated_at=VALUES(updated_at)`
	  if r.db.Dialect == persist.Postgres {
		  q = `INSERT INTO aa_uta_collateral (currency, haircut, collateral_enabled, max_ltv, updated_at) 
VALUES (?,?,?,?,?)
ON CONFLICT (currency) DO UPDATE SET haircut=EXCLUDED.haircut, collateral_enabled=EXCLUDED.collateral_enabled, max_ltv=EXCLUDED.max_ltv, updated_at=EXCLUDED.updated_at`
	   }
	   _, err := r.db.Conn(ctx).ExecContext(ctx, q, row.Currency, row.Haircut, ce, row.MaxLTV, row.UpdatedAt)
	  return err
   }

   func (r *FundingSubjects) ListCollateral(ctx context.Context) ([]UTACollateralRow, error) {
	   if !sqlOn(r.db) {
		  return nil, ErrUnavailable
	  }
	   rows, err := r.db.Conn(ctx).QueryContext(ctx, `SELECT currency, haircut, collateral_enabled, max_ltv, updated_at FROM aa_uta_collateral`)
	  if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	   out := make([]UTACollateralRow, 0)
	  for rows.Next() {
		   var row UTACollateralRow
		   var ce int
		  if err := rows.Scan(&row.Currency, &row.Haircut, &ce, &row.MaxLTV, &row.UpdatedAt); err != nil {
			  return nil, err
		   }
		   row.CollateralEnabled = ce != 0
		   out = append(out, row)
	   }
	   return out, rows.Err()
   }

  func (r *FundingSubjects) PutRepay(ctx context.Context, row UTARepayRow) error {
	   if !sqlOn(r.db) {
		  return ErrUnavailable
	  }
	  row.ID = strings.TrimSpace(row.ID)
	  row.UserID = strings.TrimSpace(row.UserID)
	  row.Currency = strings.ToUpper(strings.TrimSpace(row.Currency))
	   if row.ID == "" || row.UserID == "" || row.Currency == "" {
		  return ErrConflict
	  }
	   if row.MarginMode == "" {
		  row.MarginMode = "CROSS"
	   }
	  q := `INSERT INTO aa_uta_repay_history (id, user_id, currency, margin_mode, symbol, repay_type, principal, interest, created_at) 
VALUES (?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE principal=VALUES(principal), interest=VALUES(interest)`
	   if r.db.Dialect == persist.Postgres {
		  q = `INSERT INTO aa_uta_repay_history (id, user_id, currency, margin_mode, symbol, repay_type, principal, interest, created_at)
VALUES (?,?,?,?,?,?,?,?,?)
ON CONFLICT (id) DO UPDATE SET principal=EXCLUDED.principal, interest=EXCLUDED.interest`
	  }
	   _,
		  err := r.db.Conn(ctx).ExecContext(ctx, q, row.ID, row.UserID, row.Currency, row.MarginMode, row.Symbol, row.RepayType, row.Principal, row.Interest, row.CreatedAt)
	  return err
  }

   func (r *FundingSubjects) ListRepay(ctx context.Context, f UTARepayFilter) ([]UTARepayRow, error) {
	  if !sqlOn(r.db) {
		   return nil, ErrUnavailable
	   }
	   f.UserID = strings.TrimSpace(f.UserID)
	  f.Currency = strings.ToUpper(strings.TrimSpace(f.Currency))
	   f.MarginMode = strings.ToUpper(strings.TrimSpace(f.MarginMode))
	  f.Symbol = strings.ToUpper(strings.TrimSpace(f.Symbol))
	  if f.Limit <= 0 || f.Limit > 500 {
		   f.Limit = 50
	  }
	  q := `SELECT id, user_id, currency, margin_mode, symbol, repay_type, principal, interest, created_at FROM aa_uta_repay_history WHERE user_id=?`
	   args := []any{f.UserID}
	  if f.Currency != "" {
		  q += ` AND currency=?`
		   args = append(args, f.Currency)
	  }
	  if f.MarginMode != "" {
		  q += ` AND margin_mode=?`
		  args = append(args, f.MarginMode)
	  }
	   if f.Symbol != "" {
		   q += ` AND symbol=?`
		  args = append(args, f.Symbol)
	  }
	   if f.AfterMs > 0 {
		  q += ` AND created_at>=?`
		  args = append(args, f.AfterMs)
	  }
	   if f.BeforeMs > 0 {
		   q += ` AND created_at<=?`
		   args = append(args, f.BeforeMs)
	   }
	   q += ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	  args = append(args, f.Limit, f.Offset)
	  rows, err := r.db.Conn(ctx).QueryContext(ctx, q, args...)
	  if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	  out := make([]UTARepayRow, 0)
	  for rows.Next() {
		  var row UTARepayRow
		   if err := rows.Scan(&row.ID, &row.UserID, &row.Currency, &row.MarginMode, &row.Symbol, &row.RepayType, &row.Principal, &row.Interest, &row.CreatedAt); err != nil {
			  return nil, err
		  }
		  out = append(out, row)
	  }
	  return out, rows.Err()
  }

  func (r *FundingSubjects) PutRisk(ctx context.Context, row UTARiskRow) error {
	  if !sqlOn(r.db) {
		  return ErrUnavailable
	   }
	   row.UserID = strings.TrimSpace(row.UserID)
	   if row.UserID == "" {
		   return ErrConflict
	   }
	   q := `INSERT INTO aa_uta_risk_snapshot (user_id, margin_level, total_equity, total_liability, total_collateral, risk_grade, cross_debt_ratio, updated_at)
VALUES (?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE margin_level=VALUES(margin_level), total_equity=VALUES(total_equity), total_liability=VALUES(total_liability),
total_collateral=VALUES(total_collateral), risk_grade=VALUES(risk_grade), cross_debt_ratio=VALUES(cross_debt_ratio), updated_at=VALUES(updated_at)`
	  if r.db.Dialect == persist.Postgres {
		  q = `INSERT INTO aa_uta_risk_snapshot (user_id, margin_level, total_equity, total_liability, total_collateral, risk_grade, cross_debt_ratio, updated_at)
VALUES (?,?,?,?,?,?,?,?)
ON CONFLICT (user_id) DO UPDATE SET margin_level=EXCLUDED.margin_level, total_equity=EXCLUDED.total_equity,
total_liability=EXCLUDED.total_liability, total_collateral=EXCLUDED.total_collateral, risk_grade=EXCLUDED.risk_grade,
cross_debt_ratio=EXCLUDED.cross_debt_ratio, updated_at=EXCLUDED.updated_at`
	   }
	  _, err := r.db.Conn(ctx).ExecContext(ctx, q, row.UserID, row.MarginLevel, row.TotalEquity, row.TotalLiability, row.TotalCollateral, row.RiskGrade, row.CrossDebtRatio, row.UpdatedAt)
	   return err
   }

  func (r *FundingSubjects) GetRisk(ctx context.Context, userID string) (UTARiskRow, error) {
	  if !sqlOn(r.db) {
		   return UTARiskRow{}, ErrUnavailable
	   }
	   userID = strings.TrimSpace(userID)
	   var row UTARiskRow
	  err := r.db.Conn(ctx).QueryRowContext(ctx, `SELECT user_id, margin_level, total_equity, total_liability, total_collateral, risk_grade, cross_debt_ratio, updated_at 
FROM aa_uta_risk_snapshot WHERE user_id=?`, userID).Scan(&row.UserID, &row.MarginLevel, &row.TotalEquity, &row.TotalLiability, &row.TotalCollateral, &row.RiskGrade, &row.CrossDebtRatio, &row.UpdatedAt)
	   if err != nil {
		   return UTARiskRow{}, err
	  }
	  return row, nil
  }

  func (r *FundingSubjects) ListRisk(ctx context.Context) ([]UTARiskRow, error) {
	   if !sqlOn(r.db) {
		   return nil, ErrUnavailable
	   }
	   rows, err := r.db.Conn(ctx).QueryContext(ctx, `SELECT user_id, margin_level, total_equity, total_liability, total_collateral, risk_grade, cross_debt_ratio, updated_at FROM aa_uta_risk_snapshot`)
	   if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	  out := make([]UTARiskRow, 0)
	  for rows.Next() {
		   var row UTARiskRow
		   if err := rows.Scan(&row.UserID, &row.MarginLevel, &row.TotalEquity, &row.TotalLiability, &row.TotalCollateral, &row.RiskGrade, &row.CrossDebtRatio, &row.UpdatedAt); err != nil {
			   return nil, err
		   }
		   out = append(out, row)
	   }
	  return out, rows.Err()
   }

  func (r *FundingSubjects) PutCrossMargin(ctx context.Context, row UTACrossMarginRow) error {
	  if !sqlOn(r.db) {
		   return ErrUnavailable
	  }
	  row.UserID = strings.TrimSpace(row.UserID)
	  row.Currency = strings.ToUpper(strings.TrimSpace(row.Currency))
	  if row.UserID == "" || row.Currency == "" {
		  return ErrConflict
	   }
	   be, te := 0, 0
	   if row.BorrowEnabled {
		   be = 1
	   }
	   if row.TransferEnabled {
		   te = 1
	   }
	   q := `INSERT INTO aa_uta_cross_margin (user_id, currency, total, available, hold, liability, max_borrow, borrow_enabled, transfer_enabled, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE total=VALUES(total), available=VALUES(available), hold=VALUES(hold), liability=VALUES(liability),
max_borrow=VALUES(max_borrow), borrow_enabled=VALUES(borrow_enabled), transfer_enabled=VALUES(transfer_enabled), updated_at=VALUES(updated_at)`
	   if r.db.Dialect == persist.Postgres {
		  q = `INSERT INTO aa_uta_cross_margin (user_id, currency, total, available, hold, liability, max_borrow, borrow_enabled, transfer_enabled, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (user_id, currency) DO UPDATE SET total=EXCLUDED.total, available=EXCLUDED.available, hold=EXCLUDED.hold,
liability=EXCLUDED.liability, max_borrow=EXCLUDED.max_borrow, borrow_enabled=EXCLUDED.borrow_enabled, transfer_enabled=EXCLUDED.transfer_enabled, updated_at=EXCLUDED.updated_at`
	  }
	   _,
		   err := r.db.Conn(ctx).ExecContext(ctx, q, row.UserID, row.Currency, row.Total, row.Available, row.Hold, row.Liability, row.MaxBorrow, be, te, row.UpdatedAt)
	   return err
  }

  func (r *FundingSubjects) ListCrossMargin(ctx context.Context, userID string) ([]UTACrossMarginRow, error) {
	  if !sqlOn(r.db) {
		  return nil, ErrUnavailable
	   }
	  userID = strings.TrimSpace(userID)
	  q := `SELECT user_id, currency, total, available, hold, liability, max_borrow, borrow_enabled, transfer_enabled, updated_at FROM aa_uta_cross_margin`
	  args := []any{}
	   if userID != "" {
		  q += ` WHERE user_id=?`
		   args = append(args, userID)
	  }
	   rows, err := r.db.Conn(ctx).QueryContext(ctx, q, args...)
	   if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	  out := make([]UTACrossMarginRow, 0)
	  for rows.Next() {
		   var row UTACrossMarginRow
		  var be, te int
		   if err := rows.Scan(&row.UserID, &row.Currency, &row.Total, &row.Available, &row.Hold, &row.Liability, &row.MaxBorrow, &be, &te, &row.UpdatedAt); err != nil {
			  return nil, err
		  }
		  row.BorrowEnabled = be != 0
		  row.TransferEnabled = te != 0
		   out = append(out, row)
	  }
	  return out, rows.Err()
  }

   func (r *FundingSubjects) CountRepay(ctx context.Context, f UTARepayFilter) (int, error) {
	   if !sqlOn(r.db) {
		   return 0, ErrUnavailable
	   }
	   f.UserID = strings.TrimSpace(f.UserID)
	  q := `SELECT COUNT(1) FROM aa_uta_repay_history WHERE user_id=?`
	   args := []any{f.UserID}
	  if f.Currency != "" {
		   q += ` AND currency=?`
		  args = append(args, strings.ToUpper(strings.TrimSpace(f.Currency)))
	   }
	   if f.MarginMode != "" {
		  q += ` AND margin_mode=?`
		  args = append(args, strings.ToUpper(strings.TrimSpace(f.MarginMode)))
	  }
	   if f.Symbol != "" {
		   q += ` AND symbol=?`
		   args = append(args, strings.ToUpper(strings.TrimSpace(f.Symbol)))
	   }
	  var n int
	   err := r.db.Conn(ctx).QueryRowContext(ctx, q, args...).Scan(&n)
	   return n, err
  }
