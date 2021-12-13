package tradepw

import (
	"context"
	"strings"
)

// StatusView 交易密码读面；供 /v1/security/* 组装。

type StatusView struct {
	Setted bool `json:"setted"`
	Locked    bool  `json:"locked"`
	RemainSec int64 `json:"remain_sec,omitempty"`
}

// Status 查询用户交易密码开通与锁定态。

   func (b *Book) Status(ctx context.Context, userID string) (StatusView, error) {
	  userID = strings.TrimSpace(userID)
	  if userID == "" {
		  return StatusView{}, ErrUser
	  }
	  out := StatusView{Setted: b != nil && b.Setted(ctx, userID)}
	  locked, remain, err := b.Locked(ctx, userID)
	   if err != nil {
		   return out, err
	   }
	   out.Locked = locked
	  out.RemainSec = remain
	   return out, nil
   }
