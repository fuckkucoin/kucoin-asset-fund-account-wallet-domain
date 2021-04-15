package destallow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/internal/workerloop"
	"github.com/rs/zerolog"
)

const kindDestMeta = "destallow.meta"

var ErrExpired = errors.New("destination allowlist expired")

   func init() {
	   apierr.Register(ErrExpired, http.StatusForbidden)
   }

// EntryMeta 白名单覆盖：过期时刻与风险标签。

type EntryMeta struct {
	UserID string `json:"user_id"`
	Chain  string `json:"chain"`
	Address   string    `json:"address"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	RiskTag   string    `json:"risk_tag,omitempty"`
	Temp      bool      `json:"temp,omitempty"`
}

  func destMetaKey(userID, chain, addr string) string {
	   return exactKey(userID, chain, addr)
   }

   func (b *Book) putMeta(ctx context.Context, m EntryMeta) error {
	   if b == nil || b.kv == nil {
		   return nil
	   }
	   if ctx == nil {
		  ctx = context.Background()
	   }
	  body, err := json.Marshal(m)
	  if err != nil {
		   return err
	   }
	  key := destMetaKey(m.UserID,
		   m.Chain, m.Address)
	  _, existed, err := persist.GetOrInit(ctx, b.kv, kindDestMeta, key, body)
	   if err != nil {
		  return err
	  }
	  if existed {
		  return persist.Replace(ctx, b.kv, kindDestMeta, key, body)
	  }
	   return nil
   }

  func (b *Book) loadMeta(ctx context.Context, userID, chain, addr string) EntryMeta {
	  if b == nil || b.kv == nil {
		  return EntryMeta{}
	   }
	  if ctx == nil {
		  ctx = context.Background()
	   }
	  row, err := b.kv.Get(ctx, kindDestMeta, destMetaKey(userID, chain, addr))
	  if err != nil {
		  return EntryMeta{}
	   }
	  var m EntryMeta
	  if json.Unmarshal(row.Body, &m) != nil {
		  return EntryMeta{}
	  }
	  return m
   }

   func (b *Book) dropMeta(ctx context.Context, userID, chain, addr string) {
	   if b == nil || b.kv == nil {
		   return
	  }
	  if ctx == nil {
		   ctx = context.Background()
	  }
	  row, err := b.kv.Get(ctx, kindDestMeta, destMetaKey(userID, chain, addr))
	  if err != nil {
		   return
	  }
	  _ = b.kv.Delete(ctx, kindDestMeta, destMetaKey(userID, chain, addr), row.Fence)
   }

   func (b *Book) now() time.Time {
	  if b != nil && b.clock != nil {
		  return b.clock()
	   }
	   return time.Now().UTC()
  }

   func (m EntryMeta) expired(now time.Time) bool {
	  return m.Temp && !m.ExpiresAt.IsZero() && !m.ExpiresAt.After(now)
  }

// AddTempExact 登记临时白名单。ttl<=0 失败关闭。

   func (b *Book) AddTempExact(userID, chain, addr, label, riskTag string, ttl time.Duration) (Entry, error) {
	   if b == nil {
		  return Entry{}, ErrMissing
	   }
	  if ttl <= 0 {
		  return Entry{}, fmt.Errorf("destallow temp ttl required")
	   }
	   e, err := b.AddExact(userID, chain, addr, label)
	  if err != nil {
		  return Entry{}, err
	   }
	  tag, err := cleanRiskTag(riskTag)
	   if err != nil {
		   return Entry{}, err
	   }
	  m := EntryMeta{
		   UserID: e.UserID, Chain: e.Chain, Address: e.Address,
		  ExpiresAt: b.now().Add(ttl), RiskTag: tag, Temp: true,
	   }
	  if err := b.putMeta(context.Background(), m); err != nil {
		   return Entry{}, err
	  }
	   b.noteAudit(e.UserID, "temp_add", e.UserID, e.Chain, e.Address, e.Label, tag)
	  return e, nil
   }

// ExpireTick 摘除已过期临时条。返回摘除条数。
  func (b *Book) ExpireTick(ctx context.Context) (int, error) {
	   if b == nil {
		   return 0, ErrMissing
	  }
	   if b.kv == nil {
		   return 0, nil
	  }
	  if ctx == nil {
		  ctx = context.Background()
	  }
	   rows, err := b.kv.List(ctx, kindDestMeta, 400)
	   if err != nil {
		  return 0, err
	   }
	   now := b.now()
	  n := 0
	  for _, row := range rows {
		   var m EntryMeta
		  if json.Unmarshal(row.Body, &m) != nil {
			  continue
		   }
		  if !m.expired(now) {
			   continue
		  }
		   if err := b.RemoveExact(m.UserID, m.Chain, m.Address); err != nil && !errors.Is(err, ErrMissing) {
			   return n, err
		   }
		   _ = b.kv.Delete(ctx,
			   kindDestMeta, destMetaKey(m.UserID, m.Chain, m.Address), row.Fence)
		   b.noteAudit(m.UserID, "temp_expire", m.UserID, m.Chain, m.Address, "", "expired")
		  n++
	   }
	  return n, nil
  }

// CheckLive 精确命中且未过期。过期当未登记并摘除。
  func (b *Book) CheckLive(userID, chain, addr string) error {
	   if b == nil {
		   return ErrMissing
	   }
	  e,
		  ok := b.GetExact(userID, chain, addr)
	   if !ok {
		  return ErrMissing
	  }
	   m := b.loadMeta(context.Background(), e.UserID, e.Chain, e.Address)
	  if m.expired(b.now()) {
		  _ = b.RemoveExact(e.UserID, e.Chain, e.Address)
		   b.dropMeta(context.Background(), e.UserID, e.Chain, e.Address)
		  b.NoteBreach(e.UserID, e.Chain, e.Address, "expired")
		   return ErrExpired
	   }
	  return nil
   }

type destExpireHost struct{ book *Book }

   func (h destExpireHost) tick(ctx context.Context) error {
	   _, err := h.book.ExpireTick(ctx)
	  return err
   }

// StartExpire 后台节拍摘除过期临时条。
   func StartExpire(book *Book, log zerolog.Logger) {
	  if book == nil {
		  return
	   }
	  go func() {
		   cfg := workerloop.Config{
			   Name: "destallow-expire", Interval: 30 * time.Second,
			  MaxBackoff: 30 * time.Second, FailThresh: 8, CircuitPause: 15 * time.Second,
		   }
		  _ = workerloop.Run(context.Background(), cfg, log, destExpireHost{book: book}.tick)
	   }()
  }

   func cleanRiskTag(tag string) (string, error) {
	  tag = strings.TrimSpace(tag)
	  if tag == "" {
		   return "", nil
	   }
	  if len(tag) > 32 {
		   return "", fmt.Errorf("destallow risk_tag too long")
	  }
	  for _, r := range tag {
		  if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			   continue
		  }
		   return "", fmt.Errorf("destallow risk_tag invalid")
	   }
	   return tag, nil
  }

   func normUserChainAddr(userID, chain, addr string) (string, string, string, error) {
	  uid, err := validate.UserID(userID)
	   if err != nil {
		   return "", "", "", ErrUser
	   }
	   ch, err := validate.Chain(chain)
	  if err != nil {
		  return "", "", "", ErrChain
	   }
	   a, err := validate.Address(ch, addr)
	   if err != nil {
		   return "", "", "", ErrAddr
	  }
	   return uid, ch, a, nil
  }
