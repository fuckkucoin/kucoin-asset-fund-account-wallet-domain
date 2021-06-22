package fundsurf

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/persist"
)

// MemoDepositor Memo 找回入账适配：归因绑定 + 链观察入账。金额只认观察，禁止工单自报金额。
type MemoDepositor interface {
	BindMemo(userID, chain, memo string) error
	ObserveCredit(ctx context.Context, userID, chain, txHash, asset, memo, addr string) (*ledger.Transfer, error)
}

var (
	ErrMemoAsset = errors.New("memo recovery asset required for credit")
	ErrMemoPending = errors.New("memo ticket not pending")
	ErrMemoCredit  = errors.New("memo recovery deposit credit failed")
)

// ApproveMemoCredit 审批通过后走充值入账：绑定 memo → 观察入账 → ApplyInbound。
// 错漏 Memo 自助找回的资金落地。
// 须已有链上观察；金额不得取自工单字段。
// 重复审批靠幂等键 memo-recover:{id}；拒绝路径不得入账。
   func (b *Book) ApproveMemoCredit(ctx context.Context, id, asset string, dep MemoDepositor) (MemoTicket, error) {
	  if b == nil || b.kv == nil {
		   return MemoTicket{}, ErrStore
	   }
	   if dep == nil {
		   return MemoTicket{}, ErrMemoCredit
	  }
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	  if asset == "" {
		   return MemoTicket{}, ErrMemoAsset
	   }
	  t, err := b.GetMemoTicket(ctx, id)
	  if err != nil {
		   return MemoTicket{}, err
	   }
	   if !strings.EqualFold(t.Status, "pending") {
		   return MemoTicket{}, ErrMemoPending
	   }
	   if err := dep.BindMemo(t.UserID, t.Chain, t.Memo); err != nil {
		  return MemoTicket{}, err
	   }
	  tr, err := dep.ObserveCredit(ctx, t.UserID, t.Chain, t.TxHash, asset, t.Memo, t.Address)
	  if err != nil {
		   return MemoTicket{}, fmt.Errorf("%w: %v", ErrMemoCredit, err)
	   }
	  if tr == nil {
		   return MemoTicket{}, ErrMemoCredit
	  }
	  if tr.State == ledger.StateCredited {
		   if err := b.ApplyInbound(ctx, t.UserID, t.Chain, asset, tr.Amount.String(), "memo-recover:"+t.ID); err != nil {
			  return MemoTicket{}, err
		  }
	   }
	   t.Status = "approved"
	  body, _ := json.Marshal(t)
	   row,
		   err := b.kv.Get(ctx, kindMemo, t.ID)
	  if err != nil {
		   return MemoTicket{}, err
	  }
	  if err := b.kv.Put(ctx, persist.Row{Kind: kindMemo, Key: t.ID, Body: body}, row.Fence); err != nil {
		   return MemoTicket{}, err
	  }
	   return t, nil
   }
