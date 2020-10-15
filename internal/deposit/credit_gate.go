// 入账前统一检查异常单、确认数、重复交易哈希与 memo 归因。任一闸失败不得贷记用户可用；重复哈希返回明确冲突。观察线程绕过本闸会直接造成错账或双计入账。

// Author: Blaise
// Date: 2020.10.13

package deposit

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrConfirmationsShort = errors.New("deposit confirmations below required depth")
	ErrDuplicateTxHash    = errors.New("deposit tx hash already credited")
	ErrMemoAttribution    = errors.New("deposit memo attribution unresolved")
	ErrCreditGate         = errors.New("deposit credit gate denied")
)

// CreditGateInput 入账闸入参。
type CreditGateInput struct {
	UserID string
	Chain  string
	Asset  string
	TxHash string
	Confirmations int
	RequiredDepth int
	MemoRequired  bool
	MemoResolved bool
	HashSeenBefore bool
}

// CreditGate 入账前总闸。Desk 异常单先。
   func (d *Desk) CreditGate(in CreditGateInput) error {
	   if d == nil {
		   return ErrTicketMiss
	   }
	  if err := d.BlocksCreditScoped(in.UserID, in.Chain, in.Asset); err != nil {
		   return fmt.Errorf("%w: %v", ErrCreditGate, err)
	  }
	  if in.RequiredDepth < 0 {
		   in.RequiredDepth = 0
	  }
	  if in.Confirmations < in.RequiredDepth {
		   return fmt.Errorf("%w: have=%d need=%d", ErrConfirmationsShort, in.Confirmations, in.RequiredDepth)
	  }
	   tx := strings.ToLower(strings.TrimSpace(in.TxHash))
	  if tx == "" {
		   return fmt.Errorf("%w: empty tx hash", ErrCreditGate)
	  }
	  if in.HashSeenBefore {
		   return fmt.Errorf("%w: %s", ErrDuplicateTxHash, tx)
	   }
	  if in.MemoRequired && !in.MemoResolved {
		   return ErrMemoAttribution
	  }
	  return nil
  }
