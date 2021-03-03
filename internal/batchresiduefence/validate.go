package batchresiduefence

import (
	"strings"
	"unicode/utf8"
)

  func normID(s string, max int) (string, error) {
	  v := strings.TrimSpace(s)
	   if v == "" || utf8.RuneCountInString(v) > max {
		   return "", ErrBatchID
	   }
	  return v, nil
   }

  func requireBatchID(s string) (string, error) {
	  v, err := normID(s, MaxBatchIDLen)
	  if err != nil {
		   return "", ErrBatchID
	  }
	  return v, nil
  }

   func requireItemIdem(s string) (string, error) {
	   v := strings.TrimSpace(s)
	  if v == "" || utf8.RuneCountInString(v) > MaxItemIdemLen {
		   return "", ErrItemIdem
	   }
	   return v, nil
  }

  func requireActor(s string) (string, error) {
	   v := strings.TrimSpace(s)
	   if v == "" || utf8.RuneCountInString(v) > MaxActorLen {
		   return "", ErrActor
	   }
	   return v, nil
   }

   func requireUser(s string) (string, error) {
	  v := strings.TrimSpace(s)
	   if v == "" {
		   return "", ErrUser
	   }
	   return v, nil
  }

   func requireAsset(s string) (string, error) {
	  v := strings.ToUpper(strings.TrimSpace(s))
	   if v == "" {
		   return "", ErrAsset
	  }
	  return v, nil
   }

   func requireAmount(s string) (string, error) {
	   v := strings.TrimSpace(s)
	  if v == "" || v == "0" {
		   return "", ErrAmount
	   }
	  for _, c := range v {
		   if (c < '0' || c > '9') && c != '.' {
			   return "", ErrAmount
		   }
	  }
	  return v, nil
  }

  func clipReason(s string) string {
	   v := strings.TrimSpace(s)
	   if utf8.RuneCountInString(v) > MaxReasonLen {
		   r := []rune(v)
		   return string(r[:MaxReasonLen])
	   }
	   return v
   }

   func forbidRetryPhase(p ItemPhase) error {
	   switch p {
	   case PhaseSubmitted:
		   return ErrItemSubmitted
	   case PhaseSkipped:
		  return ErrItemSkipped
	  case PhaseFailed:
		  return nil
	   default:
		   return ErrItemNotFailed
	   }
  }
