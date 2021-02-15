// 功能：custody 包内 service 实现。
// 热/冷库存：热账 system.bridge，冷账 system.cold。

// 作者：Hale
// 日期：2021.01.20

package custody

import (
	"context"
	"fmt"
	"account-assets.kucoin.net/funding/internal/cryptoenc"
	"account-assets.kucoin.net/funding/internal/dualctl"
	"account-assets.kucoin.net/funding/internal/kms"
	"account-assets.kucoin.net/funding/internal/ledger"
)

type InventoryRow struct {
	Book string `json:"book"`
	Asset string `json:"asset"`
	Available string `json:"available"`
	Frozen    string `json:"frozen"`
	Side string `json:"side"`
}

type Service struct {
	store ledger.Store
	led   *ledger.Service
	enc   *cryptoenc.Envelope
	hot map[string]string
	approvals *ApprovalBook
	ceremony  *CeremonyBook
	quorum      *dualctl.Book
	feeBumpGate FeeBumpGate
}

   func New(store ledger.Store, led *ledger.Service, enc *cryptoenc.Envelope, hotMax map[string]string) *Service {
	  return &Service{
		   store: store, led: led, enc: enc, hot: hotMax,
		  approvals: NewApprovalBook(), ceremony: NewCeremonyBook(),
		  quorum: dualctl.New(0),
	   }
  }

   func (s *Service) Approvals() *ApprovalBook { return s.approvals }

  func (s *Service) Ceremonies() *CeremonyBook { return s.ceremony }

  func (s *Service) Quorum() *dualctl.Book { return s.quorum }

  func (s *Service) BindQuorum(q *dualctl.Book) *Service {
	  if s != nil && q != nil {
		  s.quorum = q
	   }
	   return s
  }

// Inventory 只列出 system.bridge / system.warm / system.cold。用户资金账不在托管里，避免把现货可用误当成热钱包。
  func (s *Service) Inventory(ctx context.Context) ([]InventoryRow, error) {
	  if s == nil || s.store == nil {
		   return nil, fmt.Errorf("custody store unavailable")
	   }
	   accs, err := s.store.ListUserBooks(ctx, "system")
	  if err != nil {
		  return nil, err
	  }
	   out := make([]InventoryRow, 0, len(accs))
	  for _, a := range accs {
		   side := "other"
		  switch a.AccountType {
		   case ledger.AccountBridge:
			   side = "hot"
		  case ledger.AccountWarm:
			   side = "warm"
		  case ledger.AccountCold:
			   side = "cold"
		  default:
			   continue
		   }
		   out = append(out, InventoryRow{
			   Book: string(a.AccountType), Asset: a.Asset,
			   Available: a.Available.String(), Frozen: a.Frozen.String(), Side: side,
		  })
	  }
	   return out, nil
  }

// Sweep 把 bridge 超过 hotMax 的可用划到 cold。走 ledger.Create(hot_cold_sweep)，带幂等键；
// HTTP 入口还要先过双人审批，本方法本身不鉴权。

   func (s *Service) Sweep(ctx context.Context) {
	   if s == nil || s.led == nil {
		  return
	   }
	  s.led.SweepHot(ctx, s.hot)
   }

   func (s *Service) HSMKind() string {
	  if s == nil || s.enc == nil {
		  return "none"
	  }
	   return s.enc.Kind()
   }

// SignWithdraw 对已冻结材料做链上 ECDSA（secp256k1）。HMAC 不作为链授权。
// amount 必须是调用方已经 freeze 的那笔，禁止另造一个数去签。HTTP 路径还要已批准的 approval。

  func (s *Service) SignWithdraw(movementID, chain, to, amount, destinationTag string) ([]byte, error) {
	   if s == nil || s.enc == nil {
		   return nil, fmt.Errorf("file-backed KMS unavailable")
	   }
	  if err := s.admitCeremonySign(movementID, chain, to, amount); err != nil {
		  return nil, err
	  }
	   if err := s.admitReserveSign(movementID, amount); err != nil {
		   return nil, err
	   }
	  return s.enc.SignWithdraw(movementID, chain, to, amount, destinationTag)
  }

// SignWith 走 HSM 适配器。材料含 destination_tag，与广播载荷一致。
   func (s *Service) SignWith(adapter kms.Signer, movementID, chain, to, amount, destinationTag string) ([]byte, error) {
	  if adapter == nil {
		  return s.SignWithdraw(movementID, chain, to, amount, destinationTag)
	   }
	   if err := s.admitCeremonySign(movementID, chain, to, amount); err != nil {
		  return nil, err
	   }
	   return adapter.SignCanonical(CanonicalWithdraw(movementID,
		   chain, to, amount, destinationTag))
   }

   func CanonicalWithdraw(movementID, chain, to, amount, destinationTag string) string {
	   return kms.CanonicalWithdraw(movementID, chain, to, amount, destinationTag)
   }
