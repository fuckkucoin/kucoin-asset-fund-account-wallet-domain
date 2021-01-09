// 充值入账域之扫链共享逻辑。

// Author: MilesFord
// Date: 2021.01.08

package deposit

import (
	"context"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/addrpool"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/wallet"
)

// SharedScanner 扫 FamMEMO 共享地址，按观察 memo/tag 自动归因入账。
type SharedScanner struct {
	Led *ledger.Service
	Wallet *wallet.Service
	Attr   *Attributor
	Pool   *addrpool.SharedMemoPool
	Dep *Service
}

// ScanOnce 对已登记共享地址拉已确认入金，Inspect 取备注，Resolve 后 Observe。
// 金额只认链观察；无备注/未绑归因跳过，不得猜用户。
// 与按用户托管地址扫链并行；幂等键 scan-fam:chain:hash 防双记。

  func (s *SharedScanner) ScanOnce(ctx context.Context) (n int, err error) {
	  if s == nil || s.Wallet == nil || s.Pool == nil {
		  return 0, nil
	   }
	  attr := s.Attr
	   if attr == nil && s.Dep != nil {
		   attr = s.Dep.Attributor()
	  }
	  if attr == nil {
		  return 0, nil
	  }
	  for _, row := range s.Pool.List() {
		   if row.Address == "" || !wallet.RequiresFamMemo(row.Chain) {
			  continue
		   }
		   txs, e := s.Wallet.InboundConfirmed(ctx, row.Chain, row.Address)
		  if e != nil {
			   continue
		  }
		  for _, tx := range txs {
			   if tx.Hash == "" || tx.Amount.IsZero() {
				   continue
			  }
			   obs, e := s.Wallet.Inspect(ctx, row.Chain, tx.Hash)
			  if e != nil {
				   continue
			   }
			   memo := strings.TrimSpace(obs.Memo)
			   tag := strings.TrimSpace(obs.Tag)
			  if memo == "" && tag == "" {
				   continue
			  }
			  got, e := attr.ResolveObserved(row.Chain, memo, tag)
			   if e != nil {
				  continue
			  }
			  asset := tx.Asset
			  if asset == "" {
				   asset = obs.Asset
			   }
			  if asset == "" {
				   continue
			  }
			   idem := "scan-fam:" + row.Chain + ":" + tx.Hash
			  if s.Dep != nil {
				   _, e = s.Dep.Observe(ctx, ObserveReq{
					  UserID: got.UserID,
					   Chain: row.Chain, TxHash: tx.Hash, Asset: asset,
					  IdempotencyKey: idem, WaitConfirm: true,
					   Memo: memo, Tag: tag,
				   })
				   if e == nil {
					   n++
				  }
				  continue
			   }
			   if s.Led == nil {
				  continue
			  }
			   _, e = s.Led.Create(ctx, ledger.CreateReq{
				   UserID: got.UserID, Direction: ledger.WalletDeposit,
				  Asset: asset, Amount: tx.Amount, Chain: row.Chain, TxHash: tx.Hash,
				  IdempotencyKey: idem, WaitConfirm: true,
				   FromAddress: strings.TrimSpace(obs.FromAddress),
				   Memo:        memo, Tag: tag, TagName: obs.TagName,
			  })
			   if e == nil {
				  n++
			   }
		  }
	   }
	  return n, nil
   }

// EnsureSharedBind 用户领 FamMEMO 地址时登记共享面并绑归因。

  func EnsureSharedBind(pool *addrpool.SharedMemoPool, attr *Attributor, userID, chain, memo string) (addrpool.SharedMemoBinding, error) {
	  if pool == nil {
		   return addrpool.SharedMemoBinding{}, fmt.Errorf("shared memo pool unavailable")
	  }
	   var binder addrpool.MemoBinder
	  if attr != nil {
		  binder = AsMemoBinder(attr)
	   }
	   return pool.BindUser(userID, chain, memo, binder)
   }
