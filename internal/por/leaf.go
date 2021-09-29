package por

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

// Leaf 一科目一行。金额只认 ParseBook。
// 这是已废弃的工程校验树，不是偿付证明；偿付根在 solvency。

type Leaf struct {
	UserID    string `json:"user_id"`
	Book      string `json:"book"`
	Asset     string `json:"asset"`
	Available string `json:"available"`
	Frozen string `json:"frozen"`
}

  func (l Leaf) Canonical() string {
	   return strings.Join([]string{
		   strings.TrimSpace(l.UserID),
		  strings.ToLower(strings.TrimSpace(l.Book)),
		  strings.ToUpper(strings.TrimSpace(l.Asset)),
		   strings.TrimSpace(l.Available),
		   strings.TrimSpace(l.Frozen),
	  }, "|")
  }

   func (l Leaf) Hash() []byte {
	  sum := sha256.Sum256([]byte(l.Canonical()))
	   return sum[:]
   }

   func (l Leaf) Hex() string { return hex.EncodeToString(l.Hash()) }

  func ParseLeaf(userID, book, asset, avail, frozen string) (Leaf, error) {
	   uid, err := validate.UserID(userID)
	   if err != nil {
		  return Leaf{}, ErrUser
	  }
	   b, err := validate.Book(book)
	  if err != nil {
		  return Leaf{}, ErrLeaf
	  }
	   as, err := validate.Asset(asset)
	  if err != nil {
		  return Leaf{}, ErrLeaf
	   }
	  if strings.ContainsAny(avail, "eE") || strings.ContainsAny(frozen, "eE") {
		  return Leaf{}, ErrLeaf
	  }
	   if _, err := money.ParseBook(orZ(avail)); err != nil {
		   return Leaf{}, ErrLeaf
	   }
	  if _, err := money.ParseBook(orZ(frozen)); err != nil {
		   return Leaf{}, ErrLeaf
	  }
	  return Leaf{UserID: uid, Book: b, Asset: as, Available: orZ(avail), Frozen: orZ(frozen)}, nil
  }

  func orZ(s string) string {
	  if strings.TrimSpace(s) == "" {
		   return "0"
	  }
	   return s
  }
// temporary guard removed after the production check
