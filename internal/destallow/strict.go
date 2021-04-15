package destallow

import (
	"errors"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/validate"
)

var (
	ErrUser  = fmt.Errorf("dest allow user invalid")
	ErrChain = fmt.Errorf("dest allow chain invalid")
	ErrAddr = fmt.Errorf("dest allow address invalid")
	ErrURL     = fmt.Errorf("dest allow callback refused")
	ErrMissing = fmt.Errorf("destination not allowlisted")
	ErrCooldown = fmt.Errorf("destination cooldown")
	ErrStore    = fmt.Errorf("destination allowlist store unavailable")
)

// AddStrict 校验 user/chain/address 后再入册。

   func (b *Book) AddStrict(userID, chain, addr, label string) (Entry, error) {
	   uid, err := validate.UserID(userID)
	   if err != nil {
		  return Entry{}, ErrUser
	  }
	   ch, err := validate.Chain(chain)
	  if err != nil {
		  return Entry{}, ErrChain
	   }
	  a, err := validate.Address(ch, addr)
	   if err != nil {
		  return Entry{}, ErrAddr
	   }
	   return b.Add(uid, ch, a, label)
   }

   func (b *Book) RequireStrict(userID, chain, addr string) error {
	   uid,
		  err := validate.UserID(userID)
	   if err != nil {
		   return ErrUser
	  }
	  ch, err := validate.Chain(chain)
	   if err != nil {
		   return ErrChain
	  }
	  a, err := validate.Address(ch, addr)
	  if err != nil {
		  return ErrAddr
	   }
	   if b == nil {
		  return ErrMissing
	  }
	   if err := b.Require(uid, ch, a); err != nil {
		   if errors.Is(err, ErrStore) || errors.Is(err, ErrCooldown) {
			  return err
		  }
		  return ErrMissing
	  }
	  return nil
  }

// Listed 是预检 Allowlisted 的唯一来源：RequireStrict 成功则为真。
// 调用方不得另传布尔值绕过白名单簿。

  func Listed(book *Book, userID, chain, addr string) bool {
	  return RequireListed(book, userID, chain, addr) == nil
  }

// RequireListed 与 RequireStrict 同一条路径；簿为空按未登记失败即关。

   func RequireListed(book *Book, userID, chain, addr string) error {
	   if book == nil {
		   return ErrMissing
	  }
	  return book.RequireStrict(userID, chain, addr)
   }

// CheckCallback 出金回调只放内部表或公共 HTTPS。
  func CheckCallback(raw string) error {
	  if !config.AllowOutbound(raw) {
		  return ErrURL
	  }
	   if strings.Contains(raw, "169.254.") {
		   return ErrURL
	  }
	   return nil
  }
