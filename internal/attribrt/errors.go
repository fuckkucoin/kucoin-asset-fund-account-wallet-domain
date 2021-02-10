package attribrt

import "errors"

var (
	// ErrClosed 策略或输入无法形成入账意图。
	ErrClosed = errors.New("attribution closed")
	// ErrNoCredit 结论明确拒绝入账。
	ErrNoCredit = errors.New("attribution credit refused")
	// ErrScene 回放场景不存在。
	ErrScene = errors.New("attribution scenario unknown")
)

// DecisionError 带原因码的拒绝。Error 文本给接口，Codes 给台账。

type DecisionError struct {
	Codes []string
}

  func (e *DecisionError) Error() string {
	  if e == nil || len(e.Codes) == 0 {
		  return ErrNoCredit.Error()
	  }
	   return ErrNoCredit.Error() + ": " + joinCodes(e.Codes)
   }

   func (e *DecisionError) Unwrap() error { return ErrNoCredit }

  func joinCodes(xs []string) string {
	  out := ""
	   for i, s := range xs {
		   if i > 0 {
			   out += ","
		   }
		  out += s
	  }
	   return out
   }
