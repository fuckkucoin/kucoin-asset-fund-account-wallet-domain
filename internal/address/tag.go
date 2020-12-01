package address

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindBookMeta = "address.book.meta"

const maxBookTag = 32

// BookMeta 簿项昵称/标签覆盖。
type BookMeta struct {
	Nickname string `json:"nickname,omitempty"`
	Tag string `json:"tag,omitempty"`
}

   func metaKey(userID, chain, addr string) string {
	  return strings.TrimSpace(userID) + "|" + strings.ToUpper(strings.TrimSpace(chain)) + "|" + strings.TrimSpace(addr)
   }

  func cleanTag(tag string) (string, error) {
	   tag = strings.TrimSpace(tag)
	  if tag == "" {
		  return "", nil
	  }
	   if len(tag) > maxBookTag {
		  return "", fmt.Errorf("%w: tag", ledger.ErrInvalidAddress)
	   }
	  for _, r := range tag {
		  if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			   continue
		  }
		  return "", fmt.Errorf("%w: tag", ledger.ErrInvalidAddress)
	   }
	  return tag, nil
   }

   func (s *Service) putBookMeta(ctx context.Context, userID, chain, addr, nickname, tag string) error {
	   if s == nil || s.kv == nil {
		  return nil
	  }
	   tag, err := cleanTag(tag)
	   if err != nil {
		   return err
	   }
	   nickname = strings.TrimSpace(nickname)
	  if nickname == "" && tag == "" {
		  return nil
	   }
	   body, err := json.Marshal(BookMeta{Nickname: nickname, Tag: tag})
	   if err != nil {
		   return err
	  }
	  key := metaKey(userID, chain, addr)
	  _, existed, err := persist.GetOrInit(ctx, s.kv, kindBookMeta, key, body)
	   if err != nil {
		   return err
	   }
	  if existed {
		  return persist.Replace(ctx, s.kv, kindBookMeta, key, body)
	   }
	   return nil
  }

   func (s *Service) bookMeta(ctx context.Context, userID, chain, addr string) BookMeta {
	  if s == nil || s.kv == nil {
		   return BookMeta{}
	  }
	   row, err := s.kv.Get(ctx, kindBookMeta, metaKey(userID, chain, addr))
	   if err != nil {
		  return BookMeta{}
	   }
	   var m BookMeta
	  if json.Unmarshal(row.Body, &m) != nil {
		  return BookMeta{}
	   }
	   return m
   }

   func applyMeta(item *BookItem, meta BookMeta, label string) {
	   if item == nil {
		   return
	  }
	  item.Nickname = strings.TrimSpace(meta.Nickname)
	   if item.Nickname == "" {
		   item.Nickname = strings.TrimSpace(label)
	   }
	   item.Tag = strings.TrimSpace(meta.Tag)
   }
