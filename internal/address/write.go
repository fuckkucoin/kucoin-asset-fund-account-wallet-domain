package address

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/validate"
)

var (
	ErrBookOwner = errors.New("address book owner mismatch")
	ErrBookGone = errors.New("address book entry not found")
)

   func init() {
	   apierr.Register(ErrBookOwner, http.StatusForbidden)
	  apierr.Register(ErrBookGone, http.StatusNotFound)
   }

// BookWrite 地址簿写入参。身份由调用方绑定后再入。
type BookWrite struct {
	UserID  string `json:"user_id"`
	Chain   string `json:"chain"`
	Asset   string `json:"asset,omitempty"`
	Address string `json:"address"`
	Label    string `json:"label,omitempty"`
	Nickname string `json:"nickname,omitempty"`
	Tag string `json:"tag,omitempty"`
	Memo string `json:"memo,omitempty"`
	Actor string `json:"actor,omitempty"`
	IdemKey string `json:"idempotency_key,omitempty"`
}

// 清洗并校验簿写字段。UserID/Chain/Address 必填；Asset 有则过 Asset；Memo 过 FamMEMO 闸。地址不得 ToLower 折叠；形态失败不得入库。

   func ValidateBookWrite(in BookWrite) (ledger.AddressBookEntry, error) {
	  uid, err := validate.UserID(in.UserID)
	   if err != nil {
		  return ledger.AddressBookEntry{}, err
	   }
	   ch, err := validate.Chain(in.Chain)
	  if err != nil {
		   return ledger.AddressBookEntry{}, err
	   }
	   addr, err := validate.Address(ch, in.Address)
	  if err != nil {
		  return ledger.AddressBookEntry{}, err
	  }
	  asset := strings.TrimSpace(in.Asset)
	   if asset != "" {
		  asset, err = validate.Asset(asset)
		   if err != nil {
			   return ledger.AddressBookEntry{}, err
		   }
	   }
	  memo := strings.TrimSpace(in.Memo)
	   if _, err := CheckFormatMatrix(ch, addr, memo); err != nil {
		  return ledger.AddressBookEntry{}, err
	   }
	   label := strings.TrimSpace(in.Label)
	  if label == "" {
		  label = strings.TrimSpace(in.Nickname)
	  }
	   if _, err := cleanTag(in.Tag); err != nil {
		   return ledger.AddressBookEntry{}, err
	  }
	  e := ledger.AddressBookEntry{
		   UserID: uid, Chain: ch, Address: addr, Label: label,
		  Memo: memo, Asset: asset, Actor: strings.TrimSpace(in.Actor),
	  }
	  if err := e.Normalize(); err != nil {
		   return ledger.AddressBookEntry{}, err
	  }
	   return e, nil
  }

   func stampWrite(e *ledger.AddressBookEntry, prev ledger.AddressBookEntry, actor string, now time.Time, reenable bool) {
	   if e == nil {
		  return
	   }
	   if actor != "" {
		   e.Actor = actor
	   }
	   if e.Actor == "" {
		  e.Actor = e.UserID
	   }
	   e.UpdatedAt = now
	  if !prev.CreatedAt.IsZero() {
		   e.CreatedAt = prev.CreatedAt
	   } else if e.CreatedAt.IsZero() {
		  e.CreatedAt = now
	   }
	   if reenable {
		  e.Disabled = false
	   }
   }

   func (s *Service) auditBook(ctx context.Context, action, actor string, e ledger.AddressBookEntry) {
	   if s == nil || s.led == nil {
		   return
	   }
	  _ = s.led.AppendAudit(ctx,
		   ledger.AuditEvent{
			   EntityType: "address_book",
			   EntityID: e.UserID + "|" + e.Chain + "|" + e.Address,
			  Action: action,
			   Actor:  actor,
			  Detail: map[string]any{
				  "user_id": e.UserID, "chain": e.Chain, "address": e.Address,
				  "asset": e.Asset, "disabled": e.Disabled, "label": e.Label,
			   },
			   CreatedAt: time.Now().UTC(),
		  })
  }

// 读地址簿审计轨迹。按属主过滤 entity_type=address_book；空 entity 回该用户全部簿审计。禁止跨户回放他人审计。
  func (s *Service) ListBookAudit(ctx context.Context, userID, chain, addr string, limit int) ([]ledger.AuditEvent, error) {
	  if s == nil || s.led == nil {
		  return nil, fmt.Errorf("address ledger unavailable")
	   }
	   uid,
		  err := validate.UserID(userID)
	  if err != nil {
		  return nil, err
	   }
	  entityID := ""
	   if strings.TrimSpace(chain) != "" && strings.TrimSpace(addr) != "" {
		  entityID = uid + "|" + strings.ToUpper(strings.TrimSpace(chain)) + "|" + strings.TrimSpace(addr)
	  }
	  list, err := s.led.ListAudit(ctx, "address_book", entityID, validate.Limit(limit, 50, 200))
	   if err != nil {
		   return nil, err
	   }
	  out := make([]ledger.AuditEvent, 0, len(list))
	   prefix := uid + "|"
	   for _, e := range list {
		  if entityID != "" {
			  out = append(out, e)
			   continue
		   }
		   if strings.HasPrefix(e.EntityID, prefix) {
			   out = append(out, e)
		   }
	  }
	  return out, nil
   }

   func ownEntry(got ledger.AddressBookEntry, userID string) error {
	  if strings.TrimSpace(got.UserID) != strings.TrimSpace(userID) {
		  return ErrBookOwner
	  }
	   return nil
  }

// 新增或复用簿项。formal 校验后 Upsert；已存在保留 created_at，清除停用。属主必须是绑定后的 user_id。
   func (s *Service) AddBook(ctx context.Context, e ledger.AddressBookEntry) (ledger.AddressBookEntry, error) {
	   return s.AddBookWrite(ctx, BookWrite{
		   UserID: e.UserID, Chain: e.Chain, Asset: e.Asset, Address: e.Address,
		   Label: e.Label, Nickname: e.Label, Memo: e.Memo, Actor: e.Actor,
	  })
   }

// 新增或复用簿项（含昵称/标签/幂等）。formal 校验后 Upsert；已存在保留 created_at，清除停用。限流超限失败关闭；幂等键冲突不得覆盖他人地址。
   func (s *Service) AddBookWrite(ctx context.Context, in BookWrite) (ledger.AddressBookEntry, error) {
	   if s == nil || s.led == nil {
		   return ledger.AddressBookEntry{}, fmt.Errorf("address ledger unavailable")
	   }
	   if idem := strings.TrimSpace(in.IdemKey); idem != "" {
		   if prev, ok, err := s.loadBookIdem(ctx, in.UserID, idem); err != nil {
			  return ledger.AddressBookEntry{}, err
		   } else if ok {
			   if prev.Chain != "" && prev.Address != "" {
				   if strings.TrimSpace(in.Address) != "" && prev.Address != strings.TrimSpace(in.Address) {
					  return ledger.AddressBookEntry{}, ErrBookIdem
				  }
				  return prev, nil
			  }
		  }
	  }
	  got, err := ValidateBookWrite(in)
	  if err != nil {
		  return ledger.AddressBookEntry{},
			   err
	  }
	   if err := s.checkBookRate(ctx, got.UserID, time.Now().UTC()); err != nil {
		  return ledger.AddressBookEntry{}, err
	   }
	   prev, ok, err := s.GetBook(ctx, got.UserID, got.Chain, got.Address)
	  if err != nil {
		   return ledger.AddressBookEntry{}, err
	   }
	   if ok {
		   if err := ownEntry(prev, got.UserID); err != nil {
			   return ledger.AddressBookEntry{}, err
		  }
	  } else if err := s.checkBookQuota(ctx, got.UserID); err != nil {
		   return ledger.AddressBookEntry{}, err
	   }
	  stampWrite(&got,
		   prev, got.Actor, time.Now().UTC(), true)
	  out,
		  err := s.led.AddAddressBook(ctx, got)
	   if err != nil {
		   return ledger.AddressBookEntry{}, err
	   }
	   if err := s.putBookMeta(ctx, out.UserID, out.Chain, out.Address, firstNonEmpty(in.Nickname, out.Label), in.Tag); err != nil {
		  return ledger.AddressBookEntry{}, err
	  }
	   if err := s.saveBookIdem(ctx, out.UserID, in.IdemKey, out.Chain, out.Address); err != nil {
		   return ledger.AddressBookEntry{},
			  err
	  }
	  s.auditBook(ctx, "book_add", out.Actor, out)
	   s.noteAnomaly(ctx, out.UserID, time.Now().UTC())
	   return out, nil
  }

   func firstNonEmpty(a, b string) string {
	   if strings.TrimSpace(a) != "" {
		   return strings.TrimSpace(a)
	   }
	   return strings.TrimSpace(b)
   }

// 更新簿项标签/备注/资产。身份键 user+chain+address 不变；须先命中且属主一致。未命中不得 upsert 冒充新增。
  func (s *Service) UpdateBook(ctx context.Context, in BookWrite) (ledger.AddressBookEntry, error) {
	   if s == nil || s.led == nil {
		   return ledger.AddressBookEntry{}, fmt.Errorf("address ledger unavailable")
	  }
	   got, err := ValidateBookWrite(in)
	   if err != nil {
		  return ledger.AddressBookEntry{}, err
	  }
	   prev, ok, err := s.GetBook(ctx, got.UserID, got.Chain, got.Address)
	  if err != nil {
		   return ledger.AddressBookEntry{}, err
	   }
	   if !ok {
		   return ledger.AddressBookEntry{},
			   ErrBookGone
	  }
	  if err := ownEntry(prev, got.UserID); err != nil {
		   return ledger.AddressBookEntry{}, err
	  }
	   got.Disabled = prev.Disabled
	   stampWrite(&got,
		   prev, in.Actor, time.Now().UTC(), false)
	   out, err := s.led.AddAddressBook(ctx, got)
	   if err != nil {
		  return ledger.AddressBookEntry{}, err
	  }
	   if err := s.putBookMeta(ctx, out.UserID, out.Chain, out.Address, firstNonEmpty(in.Nickname, out.Label), in.Tag); err != nil {
		  return ledger.AddressBookEntry{}, err
	  }
	   s.auditBook(ctx, "book_update", out.Actor, out)
	   return out, nil
  }

// 停用簿项，保留行以便审计。属主校验后标 disabled；Whitelist 置假。停用后 Allowlisted 必须为假，不得再放行出金。

   func (s *Service) DisableBook(ctx context.Context, userID, chain, addr, actor string) (ledger.AddressBookEntry, error) {
	   if s == nil || s.led == nil {
		   return ledger.AddressBookEntry{}, fmt.Errorf("address ledger unavailable")
	  }
	   uid, err := validate.UserID(userID)
	  if err != nil {
		  return ledger.AddressBookEntry{},
			  err
	  }
	   prev, ok, err := s.GetBook(ctx, uid, chain, addr)
	  if err != nil {
		   return ledger.AddressBookEntry{},
			   err
	  }
	   if !ok {
		  return ledger.AddressBookEntry{}, ErrBookGone
	   }
	   if err := ownEntry(prev, uid); err != nil {
		   return ledger.AddressBookEntry{}, err
	  }
	  prev.Disabled = true
	  prev.Whitelist = false
	   if strings.TrimSpace(actor) != "" {
		   prev.Actor = strings.TrimSpace(actor)
	  }
	  prev.UpdatedAt = time.Now().UTC()
	   if prev.CreatedAt.IsZero() {
		  prev.CreatedAt = prev.UpdatedAt
	  }
	  out, err := s.led.AddAddressBook(ctx, prev)
	   if err != nil {
		   return ledger.AddressBookEntry{}, err
	   }
	   s.auditBook(ctx, "book_disable", out.Actor, out)
	   return out, nil
  }

// 复用已停用簿项。属主校验后清 disabled，Whitelist 恢复。未命中不得当新增；他人簿项不得复用。

   func (s *Service) EnableBook(ctx context.Context, userID, chain, addr, actor string) (ledger.AddressBookEntry, error) {
	   if s == nil || s.led == nil {
		  return ledger.AddressBookEntry{}, fmt.Errorf("address ledger unavailable")
	   }
	   uid, err := validate.UserID(userID)
	   if err != nil {
		   return ledger.AddressBookEntry{}, err
	  }
	   prev, ok, err := s.GetBook(ctx, uid, chain, addr)
	   if err != nil {
		   return ledger.AddressBookEntry{}, err
	   }
	   if !ok {
		  return ledger.AddressBookEntry{}, ErrBookGone
	  }
	  if err := ownEntry(prev, uid); err != nil {
		   return ledger.AddressBookEntry{}, err
	  }
	   if prev.Disabled {
		   if err := s.checkBookQuota(ctx, uid); err != nil {
			  return ledger.AddressBookEntry{}, err
		   }
	  }
	  prev.Disabled = false
	  prev.Whitelist = true
	   if strings.TrimSpace(actor) != "" {
		  prev.Actor = strings.TrimSpace(actor)
	  }
	   prev.UpdatedAt = time.Now().UTC()
	   if prev.CreatedAt.IsZero() {
		  prev.CreatedAt = prev.UpdatedAt
	   }
	   out, err := s.led.AddAddressBook(ctx, prev)
	   if err != nil {
		   return ledger.AddressBookEntry{}, err
	   }
	  s.auditBook(ctx, "book_enable", out.Actor, out)
	  return out, nil
   }

// RestoreBook 恢复软删簿项。属主校验后清 disabled。

   func (s *Service) RestoreBook(ctx context.Context, userID, chain, addr, actor string) (ledger.AddressBookEntry, error) {
	   return s.EnableBook(ctx,
		   userID, chain, addr, actor)
  }

  func (s *Service) RemoveBook(ctx context.Context, userID, chain, addr string) error {
	  if s == nil || s.led == nil {
		  return fmt.Errorf("address ledger unavailable")
	   }
	   uid, err := validate.UserID(userID)
	   if err != nil {
		  return err
	   }
	  prev, ok, err := s.GetBook(ctx, uid, chain, addr)
	   if err != nil {
		  return err
	  }
	  if !ok {
		   return s.led.RemoveAddressBook(ctx, uid, chain, addr)
	   }
	  if err := ownEntry(prev, uid); err != nil {
		  return err
	  }
	   if err := s.led.RemoveAddressBook(ctx, uid, chain, addr); err != nil {
		   return err
	   }
	  s.auditBook(ctx, "book_remove", uid, prev)
	   return nil
  }
