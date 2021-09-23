package gate

import (
	"fmt"
	"strings"
)

// RiskOp 门控下需判定的操作。

type RiskOp string

const (
	OpAddPosition RiskOp = "add_position"
	OpExternalWithdraw RiskOp = "external_withdraw"
	OpReducePosition RiskOp = "reduce_position"
	OpInternalCredit RiskOp = "internal_credit"
)

var (
	ErrGated = fmt.Errorf("liquidation gate rejected")
)

   func ParseRiskOp(s string) (RiskOp, error) {
	   switch RiskOp(strings.ToLower(strings.TrimSpace(s))) {
	  case OpAddPosition, OpExternalWithdraw, OpReducePosition, OpInternalCredit:
		   return RiskOp(strings.ToLower(strings.TrimSpace(s))),
			  nil
	   default:
		  return "", fmt.Errorf("unknown risk op")
	   }
   }

// RiskIncreasing 加仓与外提增加风险敞口。

   func (op RiskOp) RiskIncreasing() bool {
	   switch op {
	   case OpAddPosition, OpExternalWithdraw:
		  return true
	  default:
		  return false
	  }
   }
