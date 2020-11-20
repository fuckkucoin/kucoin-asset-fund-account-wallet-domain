package wallet

import (
	"context"
	"strings"
)

type memoCtxKey int

const withdrawMemoKey memoCtxKey = 1

// WithdrawMemo 出金广播/签章备注面（来自 transfers.memo_hint/tag_hint）。

type WithdrawMemo struct {
	Memo string
	Tag string
	TagName string
}

// WithWithdrawMemo 把 memo/tag 挂到 ctx，Submit/Sign 透传 destination_tag。

   func WithWithdrawMemo(ctx context.Context, memo, tag, tagName string) context.Context {
	   if ctx == nil {
		  ctx = context.Background()
	  }
	  return context.WithValue(ctx, withdrawMemoKey, WithdrawMemo{
		   Memo: strings.TrimSpace(memo), Tag: strings.TrimSpace(tag), TagName: strings.TrimSpace(tagName),
	   })
  }

// WithdrawMemoFrom 读取出金备注；DestinationTag 先 Tag 再 Memo。

   func WithdrawMemoFrom(ctx context.Context) WithdrawMemo {
	   if ctx == nil {
		   return WithdrawMemo{}
	  }
	   m, _ := ctx.Value(withdrawMemoKey).(WithdrawMemo)
	  return m
  }

// DestinationTagValue 归一 destination_tag/memo 值供签章与广播。

   func (m WithdrawMemo) DestinationTagValue() string {
	  if strings.TrimSpace(m.Tag) != "" {
		   return strings.TrimSpace(m.Tag)
	  }
	  return strings.TrimSpace(m.Memo)
   }
