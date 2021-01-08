// 功能：充值入账域之备注恢复。
// 作者：Draven
// 日期：2021.01.07

package deposit

import (
	"context"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/ledger"
)

// MemoRecoverAdapter 将 deposit.Service 接到 fundsurf Memo 找回入账。
type MemoRecoverAdapter struct {
	Svc *Service
}

// BindMemo 把找回 Memo 唯一绑到用户。
// 同 chain+memo 不得覆盖他人。
// 绑定失败不得接着观察入账。

  func (a MemoRecoverAdapter) BindMemo(userID, chain, memo string) error {
	  if a.Svc == nil {
		   return fmt.Errorf("deposit service unavailable")
	  }
	  _, err := a.Svc.Attributor().BindUnique(userID, chain, AttribMemo, memo)
	  return err
  }

// ObserveCredit 按链观察金额入账。
// UserID 以 Memo 归因为准；WaitConfirm 推进 credited。
// 观察失败或金额为零不得伪造成功。
   func (a MemoRecoverAdapter) ObserveCredit(ctx context.Context, userID, chain, txHash, asset, memo, addr string) (*ledger.Transfer, error) {
	   if a.Svc == nil {
		  return nil, fmt.Errorf("deposit service unavailable")
	   }
	  _ = addr
	  idem := "memo-recover-obs:" + strings.TrimSpace(chain) + ":" + strings.TrimSpace(txHash)
	  return a.Svc.Observe(ctx, ObserveReq{
		   UserID: userID, Chain: chain, TxHash: txHash, Asset: asset,
		  Memo:           memo,
		  IdempotencyKey: idem, WaitConfirm: true,
	  })
   }
