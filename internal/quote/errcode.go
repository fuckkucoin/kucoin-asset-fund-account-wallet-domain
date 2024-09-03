package quote

import "errors"

// Code 稳定业务码。

type Code string

const (
	CodeEmpty Code = "QUOTE_EMPTY"
	CodeStale   Code = "QUOTE_STALE"
	CodeIllegal Code = "QUOTE_ILLEGAL"
	CodeMasked Code = "QUOTE_SOURCE_MASKED"
	CodeHouse Code = "QUOTE_HOUSE_ON_CONFIRM"
	CodeClock Code = "QUOTE_CLOCK_REQUIRED"
	CodePair    Code = "QUOTE_PAIR_REQUIRED"
	CodeDiverge Code = "QUOTE_DIVERGE"
	CodeTimeout Code = "QUOTE_TIMEOUT"
	CodeOK      Code = "QUOTE_OK"
	CodeUnknown Code = "QUOTE_UNKNOWN"
)

// MapError 将分类错误映射为业务码。
   func MapError(err error) Code {
	  if err == nil {
		  return CodeOK
	  }
	   switch {
	  case errors.Is(err, ErrQuoteEmpty):
		  return CodeEmpty
	   case errors.Is(err, ErrQuoteStale):
		  return CodeStale
	  case errors.Is(err, ErrQuoteIllegal):
		   return CodeIllegal
	  case errors.Is(err, ErrSourceMasked):
		  return CodeMasked
	  case errors.Is(err, ErrHouseOnConfirm):
		  return CodeHouse
	  case errors.Is(err, ErrClockRequired):
		   return CodeClock
	  case errors.Is(err, ErrPairRequired):
		  return CodePair
	   case errors.Is(err,
		  ErrQuoteDiverge):
		  return CodeDiverge
	  case errors.Is(err, ErrQuoteTimeout):
		  return CodeTimeout
	   default:
		   return CodeUnknown
	   }
  }

// MapClass 分类到业务码。

   func MapClass(c Class) Code {
	   switch c {
	  case ClassOK:
		   return CodeOK
	   case ClassEmpty:
		  return CodeEmpty
	   case ClassStale:
		   return CodeStale
	  case ClassIllegal:
		   return CodeIllegal
	  default:
		  return CodeUnknown
	   }
   }
