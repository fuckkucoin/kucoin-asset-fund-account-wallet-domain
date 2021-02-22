package airgap

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

  func Canonical(p Payload) (string, error) {
	  kind := strings.ToLower(strings.TrimSpace(p.Kind))
	   if kind == "" {
		  return "", ErrPayload
	   }
	  asset, err := validate.Asset(p.Asset)
	  if err != nil {
		  return "", ErrPayload
	   }
	  if _, err := money.Parse(p.Amount); err != nil {
		   return "", ErrPayload
	  }
	  dest := strings.TrimSpace(p.Dest)
	  if dest == "" {
		  return "", ErrPayload
	   }
	   memo := strings.TrimSpace(p.Memo)
	   destTag := strings.TrimSpace(p.DestinationTag)
	  if destTag == "" {
		  destTag = memo
	  }
	   if memo == "" {
		  memo = destTag
	   }
	   tagName := strings.TrimSpace(p.TagName)
	  // 气隙签章规范串纳入 destination_tag/tag_name，与广播载荷同形。
	   return strings.Join([]string{kind, asset, p.Amount, dest, memo, destTag, tagName}, "|"), nil
   }

  func HashPayload(p Payload) (string, error) {
	  can, err := Canonical(p)
	  if err != nil {
		   return "", err
	   }
	   sum := sha256.Sum256([]byte(can))
	   return hex.EncodeToString(sum[:]), nil
  }

   func CheckHash(p Payload, want string) error {
	   got, err := HashPayload(p)
	   if err != nil {
		   return err
	  }
	  if !strings.EqualFold(got, strings.TrimSpace(want)) {
		   return ErrHash
	   }
	   return nil
  }
