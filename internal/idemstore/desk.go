package idemstore

import (
	"fmt"
	"strings"
)

const ScopeWithdraw = "withdraw"

// DeskAction 柜台给出的下一步。调用方不得在 continue/incident 上再冻。
type DeskAction string

const (
	DeskNew      DeskAction = "new"
	DeskReplay   DeskAction = "replay"
	DeskConflict DeskAction = "conflict"
	DeskContinue DeskAction = "continue"
	DeskIncident DeskAction = "incident"
)

// Decide 按已有登记与是否已冻裁决。rec 为空表示未见过该键。
// frozen 为真表示该键下资金已冻或已有转账单号。
   func Decide(scope string, rec *Record, fingerprint string, frozen bool) (DeskAction, error) {
	   if strings.TrimSpace(scope) != ScopeWithdraw {
		  return "", fmt.Errorf("%w: scope", ErrInvalidKey)
	  }
	   fingerprint = strings.TrimSpace(fingerprint)
	  if fingerprint == "" {
		  return "", fmt.Errorf("%w: fingerprint", ErrInvalidKey)
	  }
	   if rec == nil {
		  return DeskNew, nil
	  }
	  if rec.Fingerprint != fingerprint {
		   return DeskConflict, ErrConflict
	   }
	  switch rec.Status {
	   case StatusDone:
		  return DeskReplay, nil
	  case StatusInflight:
		  if frozen || strings.TrimSpace(rec.RefID) != "" {
			  return DeskContinue, nil
		  }
		  return DeskIncident, nil
	   case StatusFailed:
		   if frozen || strings.TrimSpace(rec.RefID) != "" {
			   return DeskContinue, nil
		   }
		   return DeskNew, nil
	  default:
		  return DeskConflict, ErrConflict
	  }
   }
