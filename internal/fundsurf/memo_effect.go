package fundsurf

import "account-assets.kucoin.net/funding/internal/ledger"

// Ledger 邻接只读出口。效果闸门冲正时复用同一账本，禁止另开资金科目。

   func (b *Book) Ledger() *ledger.Service {
	  if b == nil {
		  return nil
	  }
	  return b.led
  }

// MemoEffectHint 审批成功后可回写的效果标识。工单 JSON 仍保持原字段，避免改文案。
type MemoEffectHint struct {
	TicketID string
	MemoEffectID string
	OutputKey string
	FenceID string
	HalfProofID string
}

// LastMemoEffect 进程内提示，不落工单正文。
   func (b *Book) RememberEffect(_ MemoEffectHint) {}
