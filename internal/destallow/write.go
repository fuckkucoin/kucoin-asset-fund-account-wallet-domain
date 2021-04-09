package destallow

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/validate"
)

  func init() {
	   apierr.Register(ErrUser, http.StatusBadRequest)
	   apierr.Register(ErrChain, http.StatusBadRequest)
	   apierr.Register(ErrAddr,
		  http.StatusBadRequest)
	   apierr.Register(ErrMissing, http.StatusNotFound)
	  apierr.Register(ErrCooldown, http.StatusForbidden)
	  apierr.Register(ErrStore, http.StatusServiceUnavailable)
  }

// PrefWriter 白名单偏好写。实现落在 fundsurf，本包只认接口。
type PrefWriter interface {
	PrefReader
	SetWhitelistOnly(ctx context.Context, userID string, on bool) error
}

   func exactKey(userID, chain, addr string) string {
	  return strings.ToLower(strings.TrimSpace(userID)) + "|" +
		   strings.ToUpper(strings.TrimSpace(chain)) + "|" +
		   strings.TrimSpace(addr)
   }

   func foldHit(b *Book, userID, chain, addr string) (Entry, bool) {
	  if b == nil {
		  return Entry{}, false
	   }
	   if !AddrFolded(addr, addr) {
		  return Entry{}, false
	   }
	   for _, e := range b.List(userID) {
		  if !AddrFolded(e.Address, addr) {
			  continue
		   }
		  if ChainEqual(e.Chain, chain) {
			   return e, true
		   }
	  }
	  return Entry{}, false
  }

// 精确增一条白名单。AddStrict 过 user/chain/address；已存在精确条只改标签，保留 AddedAt。不得用 ToLower 覆盖另一条大小写不同的地址。
  func (b *Book) AddExact(userID, chain, addr, label string) (Entry, error) {
	   if b == nil {
		  return Entry{}, ErrMissing
	  }
	  e, err := b.AddStrict(userID, chain, addr, label)
	   if err != nil {
		   return Entry{}, err
	  }
	   b.noteAudit(e.UserID, "add", e.UserID, e.Chain, e.Address, e.Label, "")
	   return e, nil
   }

// 按地址全等删除。大小写敏感；折叠命中不算本条。未精确命中不得静默当已删。
  func (b *Book) RemoveExact(userID, chain, addr string) error {
	  if b == nil {
		   return ErrMissing
	  }
	   uid, err := validate.UserID(userID)
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
	   if _, ok := b.GetExact(uid, ch, a); !ok {
		   return ErrMissing
	  }
	  b.Remove(uid, ch, a)
	  b.noteAudit(uid, "remove", uid, ch, a, "", "")
	  return nil
  }

// 写入 WhitelistOnly 偏好。user_id 过 validate；落 PrefWriter。写面为空则失败关闭，不得假装已切换。

  func SetPref(ctx context.Context, w PrefWriter, userID string, on bool) error {
	  if w == nil {
		  return fmt.Errorf("whitelist preference writer unavailable")
	  }
	   uid, err := validate.UserID(userID)
	   if err != nil {
		  return err
	   }
	   return w.SetWhitelistOnly(ctx, uid, on)
  }
