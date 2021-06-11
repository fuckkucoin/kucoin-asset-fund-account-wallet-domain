package fundmat

import (
	"strings"
	"time"
	"github.com/shopspring/decimal"
)

// Mutate 把一条事件应用到行上。行的用户、资产、科目必须与事件一致。
// 结果为负时返回 ErrNegativeColumn，调用方不得把该事件标成已应用。

  func Mutate(row MatRow, ev FundingEvent) (MatRow, error) {
	   if err := ev.Validate(); err != nil {
		  return row, err
	   }
	   if NormUser(row.UserID) != ev.UserID || NormAsset(row.Asset) != ev.Asset || NormSubject(row.Subject) != ev.Subject {
		  return row, ErrBadEvent
	  }
	  row = fillZero(row)
	   var err error
	  switch ev.Op {
	  case OpFreeze:
		   row.Available, err = subPos(row.Available, ev.Amount)
		  if err != nil {
			   return MatRow{}, err
		  }
		   row.Frozen, err = addPos(row.Frozen, ev.Amount)
	  case OpUnfreeze:
		  row.Frozen,
			   err = subPos(row.Frozen, ev.Amount)
		  if err != nil {
			  return MatRow{}, err
		   }
		  row.Available,
			  err = addPos(row.Available, ev.Amount)
	   case OpCreditAvailable:
		  row.Available, err = addPos(row.Available, ev.Amount)
	  case OpDebitAvailable:
		   row.Available,
			  err = subPos(row.Available, ev.Amount)
	   case OpDebitFrozen:
		   row.Frozen, err = subPos(row.Frozen, ev.Amount)
	   case OpInFlightOpen:
		  err = bumpBucket(&row, ev.Bucket, ev.Amount, true)
	   case OpInFlightClose:
		  err = bumpBucket(&row, ev.Bucket, ev.Amount, false)
	   case OpOccupy:
		  err = bumpOcc(&row, ev.Bucket, ev.Amount, true)
	   case OpRelease:
		   err = bumpOcc(&row, ev.Bucket, ev.Amount, false)
	   case OpSetAvailable:
		   row.Available,
			  err = setNonNeg(ev.Amount)
	   case OpSetFrozen:
		  row.Frozen, err = setNonNeg(ev.Amount)
	  default:
		   return row, ErrBadEvent
	   }
	   if err != nil {
		   return MatRow{}, err
	  }
	  if err := row.recompute(); err != nil {
		   return MatRow{}, err
	   }
	   return row, nil
   }

   func fillZero(row MatRow) MatRow {
	   z := func(p *string) {
		  if strings.TrimSpace(*p) == "" {
			  *p = "0"
		   }
	  }
	   z(&row.Available)
	  z(&row.Frozen)
	  z(&row.InFlight)
	  z(&row.InFlightDeposit)
	  z(&row.InFlightWithdraw)
	   z(&row.InFlightTransfer)
	   z(&row.Occupancy)
	  z(&row.OccupancyRisk)
	  z(&row.OccupancyReserve)
	  return row
   }

  func (r *MatRow) recompute() error {
	  sum, err := add3(r.InFlightDeposit, r.InFlightWithdraw, r.InFlightTransfer)
	  if err != nil {
		  return err
	  }
	  r.InFlight = sum
	  occ, err := add2(r.OccupancyRisk, r.OccupancyReserve)
	  if err != nil {
		   return err
	   }
	   r.Occupancy = occ
	   return nil
   }

  func bumpBucket(row *MatRow, b Bucket, amt string, open bool) error {
	   var dst *string
	   switch b {
	  case BucketDeposit:
		  dst = &row.InFlightDeposit
	   case BucketWithdraw:
		  dst = &row.InFlightWithdraw
	   case BucketTransfer, "":
		  dst = &row.InFlightTransfer
		   if b == "" {
			   row.InFlightTransfer = row.InFlightTransfer
		  }
	  default:
		   return ErrBadEvent
	   }
	  var err error
	  if open {
		   *dst, err = addPos(*dst, amt)
	  } else {
		  *dst, err = subPos(*dst, amt)
	   }
	  return err
  }

  func bumpOcc(row *MatRow, b Bucket, amt string, open bool) error {
	   var dst *string
	  switch b {
	  case BucketRisk, "":
		   dst = &row.OccupancyRisk
	   case BucketReserve:
		  dst = &row.OccupancyReserve
	  default:
		   return ErrBadEvent
	  }
	   var err error
	   if open {
		  *dst, err = addPos(*dst, amt)
	  } else {
		   *dst, err = subPos(*dst, amt)
	   }
	   return err
  }

// Validate 检查事件能否进入日志。序号 0 表示由仓储分配。
   func (ev *FundingEvent) Validate() error {
	   if ev == nil {
		  return ErrBadEvent
	   }
	   ev.ID = strings.TrimSpace(ev.ID)
	   ev.UserID = NormUser(ev.UserID)
	  ev.Asset = NormAsset(ev.Asset)
	  ev.Subject = NormSubject(ev.Subject)
	  ev.Amount = strings.TrimSpace(ev.Amount)
	   ev.CausalID = strings.TrimSpace(ev.CausalID)
	   ev.SourceEventID = strings.TrimSpace(ev.SourceEventID)
	  if ev.Stream == "" {
		   ev.Stream = StreamOf(ev.Op)
	  }
	   if ev.ID == "" || ev.UserID == "" || ev.Asset == "" || ev.Subject == "" {
		  return ErrBadEvent
	  }
	  if !ev.Stream.Valid() || !ev.Op.Valid() || !ev.Bucket.Valid() {
		   return ErrBadEvent
	  }
	  if _, err := parsePositive(ev.Amount); err != nil {
		   return err
	   }
	   switch ev.Op {
	  case OpInFlightOpen, OpInFlightClose:
		  if ev.Bucket == BucketRisk || ev.Bucket == BucketReserve {
			   return ErrBadEvent
		   }
	   case OpOccupy, OpRelease:
		  if ev.Bucket == BucketDeposit || ev.Bucket == BucketWithdraw || ev.Bucket == BucketTransfer {
			  return ErrBadEvent
		   }
	   }
	   if ev.OccurredAt.IsZero() {
		   ev.OccurredAt = time.Now().UTC()
	   } else {
		  ev.OccurredAt = ev.OccurredAt.UTC()
	  }
	   return nil
   }

  func parsePositive(s string) (decimal.Decimal, error) {
	  s = strings.TrimSpace(s)
	  if s == "" {
		   return decimal.Zero, ErrBadEvent
	  }
	   for _, r := range s {
		   if r == 'e' || r == 'E' {
			  return decimal.Zero, ErrBadEvent
		   }
	   }
	   d, err := decimal.NewFromString(s)
	   if err != nil || !d.IsPositive() {
		   return decimal.Zero, ErrBadEvent
	  }
	  if d.Exponent() < -18 {
		   return decimal.Zero, ErrBadEvent
	  }
	   return d, nil
  }

  func parseBook(s string) (decimal.Decimal, error) {
	  s = strings.TrimSpace(s)
	   if s == "" {
		  return decimal.Zero, nil
	   }
	  d, err := decimal.NewFromString(s)
	  if err != nil || d.IsNegative() {
		  return decimal.Zero, ErrNegativeColumn
	   }
	  return d, nil
  }

  func addPos(cur, amt string) (string, error) {
	  c, err := parseBook(cur)
	   if err != nil {
		   return "", err
	  }
	   a, err := parsePositive(amt)
	  if err != nil {
		  return "", err
	  }
	  return c.Add(a).String(), nil
   }

   func subPos(cur, amt string) (string, error) {
	  c, err := parseBook(cur)
	  if err != nil {
		  return "", err
	  }
	  a, err := parsePositive(amt)
	  if err != nil {
		   return "", err
	   }
	  out := c.Sub(a)
	   if out.IsNegative() {
		  return "", ErrNegativeColumn
	  }
	   return out.String(), nil
  }

  func setNonNeg(amt string) (string, error) {
	  d, err := parsePositive(amt)
	  if err != nil {
		   // 绝对写入允许 0，重建会把空仓写成 0。
		   s := strings.TrimSpace(amt)
		  if s == "0" || s == "0.0" || s == "0.00" {
			   return "0", nil
		   }
		   d, err = decimal.NewFromString(s)
		  if err != nil || d.IsNegative() {
			  return "", ErrBadEvent
		  }
		  if !d.IsZero() && d.Exponent() < -18 {
			   return "", ErrBadEvent
		   }
	   }
	  return d.String(), nil
   }

  func add2(a, b string) (string, error) {
	   da, err := parseBook(a)
	  if err != nil {
		  return "", err
	   }
	  db, err := parseBook(b)
	  if err != nil {
		   return "", err
	  }
	   return da.Add(db).String(), nil
  }

  func add3(a, b, c string) (string, error) {
	   ab, err := add2(a, b)
	   if err != nil {
		  return "", err
	   }
	  return add2(ab, c)
   }

// SameAmount 十进制相等。容差为 0 时仍允许 1 与 1.0 视为相同。
  func SameAmount(a, b string) (bool, error) {
	   da, err := parseBook(a)
	  if err != nil {
		   return false, err
	   }
	   db, err := parseBook(b)
	   if err != nil {
		  return false, err
	  }
	  return da.Equal(db), nil
   }

// WithinTol 绝对差不超过容差。容差空串或 0 表示必须相等。

  func WithinTol(a, b, tol string) (bool, error) {
	   da, err := parseBook(a)
	  if err != nil {
		   return false, err
	   }
	  db, err := parseBook(b)
	   if err != nil {
		  return false, err
	  }
	   diff := da.Sub(db).Abs()
	  tol = strings.TrimSpace(tol)
	  if tol == "" || tol == "0" {
		   return diff.IsZero(), nil
	   }
	  td, err := decimal.NewFromString(tol)
	  if err != nil || td.IsNegative() {
		   return false, ErrBadEvent
	  }
	  return diff.LessThanOrEqual(td), nil
  }
