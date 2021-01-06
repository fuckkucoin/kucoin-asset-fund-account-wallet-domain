// 提现地址簿和白名单。
// 作者: Likeo
// 日期: 2021.01.05
package ledger

import (
	"context"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/wallet"
)

// AddressBookEntry 用户提现地址。名单上的才能出金。
// 公开面含 tagName（按链推导）与可选 memo（FamMEMO 出金目标备注）。
type AddressBookEntry struct {
	UserID string `json:"user_id"`
	Chain   string `json:"chain"`
	Address string `json:"address"`
	Label   string `json:"label"`
	Memo  string `json:"memo,omitempty"`
	Asset string `json:"asset,omitempty"`
	TagName  string `json:"tagName,omitempty"`
	NeedMemo bool   `json:"needMemo,omitempty"`
	Whitelist bool `json:"whitelist"` // 本表即提现白名单
	Disabled bool `json:"disabled,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
	Actor     string    `json:"actor,omitempty"`
}

   func (e *AddressBookEntry) Normalize() error {
	  if e == nil {
		  return ErrInvalidAddress
	  }
	  e.UserID = strings.TrimSpace(e.UserID)
	   e.Chain = strings.ToUpper(strings.TrimSpace(e.Chain))
	  e.Address = strings.TrimSpace(e.Address)
	   e.Label = strings.TrimSpace(e.Label)
	  e.Memo = strings.TrimSpace(e.Memo)
	  e.Asset = strings.ToUpper(strings.TrimSpace(e.Asset))
	   e.Actor = strings.TrimSpace(e.Actor)
	   if e.Disabled {
		  e.Whitelist = false
	  } else {
		  e.Whitelist = true
	  }
	  if e.UserID == "" || e.Address == "" {
		   return fmt.Errorf("%w: user_id and address required", ErrInvalidAddress)
	   }
	   if len(e.Address) > 256 || strings.ContainsAny(e.Address, " \t\n") {
		   return fmt.Errorf("%w: address", ErrInvalidAddress)
	   }
	  if len(e.Label) > 64 {
		   return fmt.Errorf("%w: label", ErrInvalidAddress)
	   }
	   if e.Chain == "" {
		  return fmt.Errorf("%w: chain required", ErrInvalidAddress)
	   }
	  if e.Chain != "EVM" && !supportedAddressChain(e.Chain) {
		   return fmt.Errorf("%w: unknown chain %s", ErrInvalidAddress, e.Chain)
	   }
	   if err := validateChainAddress(e.Chain, e.Address); err != nil {
		   return err
	  }
	  // 公开 tagName / needMemo；FamMEMO 有 memo 时深化校验。
	  e.TagName = wallet.FamMemoTagName(e.Chain)
	  e.NeedMemo = wallet.RequiresFamMemo(e.Chain)
	  if e.Memo != "" {
		   if err := wallet.ValidFamMemoTag(e.Chain, e.Memo); err != nil {
			   return fmt.Errorf("%w: %v", ErrAddressFormat, err)
		  }
	   }
	  if len(e.Memo) > 128 {
		   return fmt.Errorf("%w: memo", ErrInvalidAddress)
	  }
	   return nil
  }

  func supportedAddressChain(chain string) bool {
	  for _, c := range wallet.SupportedChains() {
		  if c == chain {
			  return true
		   }
	   }
	  return false
  }

// validateChainAddress 按链校验主网地址。形态不对不得进白名单，也不得进提现广播。

  func validateChainAddress(chain, addr string) error {
	  if err := wallet.ValidChainAddress(chain, addr); err != nil {
		  return fmt.Errorf("%w: %v", ErrAddressFormat, err)
	  }
	   return nil
   }

   func (s *Service) ListAddressBook(ctx context.Context, userID string) ([]AddressBookEntry, error) {
	  list,
		   err := s.store.ListAddressBook(ctx, userID)
	  if err != nil {
		  return nil, err
	  }
	   if list == nil {
		   return []AddressBookEntry{}, nil
	  }
	   for i := range list {
		   list[i].TagName = wallet.FamMemoTagName(list[i].Chain)
		  list[i].NeedMemo = wallet.RequiresFamMemo(list[i].Chain)
		  list[i].Whitelist = !list[i].Disabled
	   }
	   return list, nil
   }

  func (s *Service) AddAddressBook(ctx context.Context, e AddressBookEntry) (AddressBookEntry, error) {
	   if err := e.Normalize(); err != nil {
		  return AddressBookEntry{}, err
	   }
	   if err := s.store.UpsertAddressBook(ctx, e); err != nil {
		  return AddressBookEntry{}, err
	   }
	   return e, nil
   }

// AppendAudit 地址簿写路径落稽核。store 故障原样返回。
  func (s *Service) AppendAudit(ctx context.Context, e AuditEvent) error {
	   if s == nil || s.store == nil {
		  return fmt.Errorf("address ledger unavailable")
	   }
	   if e.CreatedAt.IsZero() {
		  e.CreatedAt = time.Now().UTC()
	   }
	   return s.store.AppendAudit(ctx, e)
   }

  func (s *Service) RemoveAddressBook(ctx context.Context, userID, chain, addr string) error {
	   e := AddressBookEntry{UserID: userID, Chain: chain, Address: addr}
	  if err := e.Normalize(); err != nil {
		   return err
	   }
	  return s.store.DeleteAddressBook(ctx, e.UserID, e.Chain, e.Address)
   }
