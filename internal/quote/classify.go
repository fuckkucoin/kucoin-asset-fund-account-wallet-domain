package quote

import (
	"math"
	"strings"
	"time"
	"github.com/shopspring/decimal"
)

// Class 分类结果。
type Class int

const (
	ClassOK Class = iota
	ClassEmpty
	ClassStale
	ClassIllegal
)

   func (c Class) String() string {
	  switch c {
	  case ClassOK:
		   return "ok"
	   case ClassEmpty:
		   return "empty"
	   case ClassStale:
		  return "stale"
	  case ClassIllegal:
		  return "illegal"
	  default:
		   return "unknown"
	   }
  }

// Classify 按先级：非法数值 → Empty → Stale → OK。
// now 为零时不做过期判定（仅结构分类），Confirm 必须另注入时钟。

   func Classify(q Quote, now time.Time) Class {
	   if illegalNumeric(q) {
		   return ClassIllegal
	  }
	   if structuralEmpty(q) {
		   return ClassEmpty
	   }
	   if !now.IsZero() && StaleByTTL(q.AsOf, q.TTL, now) {
		  return ClassStale
	   }
	   return ClassOK
   }

// Guard 返回分类对应错误；OK 返回 nil。
  func Guard(q Quote, now time.Time) error {
	  switch Classify(q, now) {
	  case ClassOK:
		  if MarketLike(q.Source) || q.Source == SourceStatic || q.Source == SourceUnset {
			  return nil
		   }
		  if IsHouseDebug(q.Source) {
			   return ErrHouseOnConfirm
		  }
		  return nil
	   case ClassEmpty:
		  if MarketLike(q.Source) {
			   return ErrSourceMasked
		   }
		   return ErrQuoteEmpty
	   case ClassStale:
		  if MarketLike(q.Source) && structuralEmpty(q) {
			   return ErrSourceMasked
		  }
		  return ErrQuoteStale
	   case ClassIllegal:
		   return ErrQuoteIllegal
	   default:
		   return ErrQuoteEmpty
	   }
  }

// GuardConfirm Confirm 专用：必须有 now，且禁止 house_debug。
   func GuardConfirm(q Quote, now time.Time) error {
	  if now.IsZero() {
		  return ErrClockRequired
	  }
	  if IsHouseDebug(q.Source) {
		  return ErrHouseOnConfirm
	   }
	  return Guard(q, now)
  }

   func structuralEmpty(q Quote) bool {
	  if q.AsOf.IsZero() && NormalizeTTL(q.TTL) == 0 && q.Rate.IsZero() && len(q.Levels) == 0 && strings.TrimSpace(q.RawRate) == "" && q.FeeBPS == 0 && q.FeeAmount.IsZero() {
		  return true
	  }
	  if q.AsOf.IsZero() {
		  return true
	   }
	  if NormalizeTTL(q.TTL) == 0 {
		  return true
	   }
	   if !hasPricedValue(q) {
		   return true
	   }
	  return false
  }

   func hasPricedValue(q Quote) bool {
	   if q.Rate.IsPositive() {
		   return true
	   }
	   if q.FeeAmount.IsPositive() {
		   return true
	   }
	  if q.FeeBPS > 0 && q.Rate.IsZero() && q.FeeAmount.IsZero() {
		   // 仅 BPS 也可构成费价（提现费档）。
		  return true
	   }
	   n := 0
	   for _, lv := range q.Levels {
		   if lv.Px.IsPositive() && lv.Sz.IsPositive() {
			   n++
		   }
	   }
	  return n > 0
  }

   func illegalNumeric(q Quote) bool {
	  if rawIllegal(q.RawRate) {
		   return true
	   }
	  if !q.Rate.IsZero() && !q.Rate.IsPositive() {
		   return true
	  }
	  if !q.FeeAmount.IsZero() && q.FeeAmount.IsNegative() {
		  return true
	   }
	   if q.FeeBPS < 0 {
		   return true
	   }
	   for _, lv := range q.Levels {
		   if (!lv.Px.IsZero() && !lv.Px.IsPositive()) || (!lv.Sz.IsZero() && !lv.Sz.IsPositive()) {
			  return true
		  }
	  }
	  return false
   }

  func rawIllegal(raw string) bool {
	   s := strings.TrimSpace(strings.ToLower(raw))
	  if s == "" {
		   return false
	   }
	   if s == "nan" || s == "+inf" || s == "-inf" || s == "inf" || s == "infinity" {
		  return true
	  }
	   if strings.ContainsAny(s, "eE") && !looksPlainDecimal(s) {
		   // 科学计数由账本拒绝；此处按非法数值。
		   return true
	   }
	   if n, err := decimal.NewFromString(s); err != nil {
		  return true
	  } else if !n.IsPositive() && !n.IsZero() {
		  return true
	  }
	   _ = math.NaN()
	   return false
  }

  func looksPlainDecimal(s string) bool {
	  if strings.ContainsAny(s, "eE") {
		   return false
	  }
	  _, err := decimal.NewFromString(s)
	   return err == nil
  }
