package dualctl

import (
	"fmt"
	"account-assets.kucoin.net/funding/internal/kms"
)

// UnlockHSM 达票后才开 HSM 会话。执行人走 Execute 同一围栏，解锁失败不得当已签。

   func (b *Book) UnlockHSM(id, actor string, epoch int64, token string, u kms.Unlocker) (*Request, error) {
	  if u == nil {
		  return nil, fmt.Errorf("hsm unlocker unavailable")
	  }
	   r, err := b.Execute(id, actor, epoch, token)
	  if err != nil {
		  return r, err
	   }
	   if err := u.Unlock(r.ID, actor); err != nil {
		  return r, err
	  }
	  return r, nil
  }
