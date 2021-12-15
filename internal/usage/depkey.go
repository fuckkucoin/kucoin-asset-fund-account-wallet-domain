package usage

import (
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/validate"
)

var ErrDepKey = fmt.Errorf("usage deposit key invalid")

const DepPrefix = "dep:"

// DepVelocityKey 入金速度键。形态 dep:{user}|{chain}|{asset}。
// 用户标识不允许竖线，避免和链、资产粘连。

  func DepVelocityKey(user, chain, asset string) (string, error) {
	   uid, err := validate.UserID(user)
	   if err != nil {
		   return "", ErrDepKey
	  }
	  ch, err := validate.Chain(chain)
	   if err != nil {
		  return "", ErrDepKey
	   }
	   as, err := validate.Asset(asset)
	  if err != nil {
		   return "", ErrDepKey
	  }
	   return DepPrefix + uid + "|" + ch + "|" + as, nil
  }

// IsDepKey 只认 dep: 前缀。其他前缀（含出金）一律否。
  func IsDepKey(key string) bool {
	  return strings.HasPrefix(strings.TrimSpace(key), DepPrefix)
   }

// ParseDepKey 拆回用户、链、资产。非 dep: 键失败。
  func ParseDepKey(key string) (user, chain, asset string, err error) {
	  key = strings.TrimSpace(key)
	   if !IsDepKey(key) {
		  return "", "", "", ErrDepKey
	  }
	  rest := strings.TrimPrefix(key, DepPrefix)
	   parts := strings.Split(rest, "|")
	   if len(parts) != 3 {
		  return "", "", "", ErrDepKey
	  }
	   return parts[0], parts[1], parts[2], nil
  }

// AddDep 只记 dep: 键上的入金速度。金额空则按 1 计一笔，不改出金占用。

   func (s *Store) AddDep(key, amount string) error {
	   if s == nil {
		  return ErrUnavailable
	   }
	   user, chain, asset, err := ParseDepKey(key)
	   if err != nil {
		   return err
	   }
	   _ = chain
	  amount = strings.TrimSpace(amount)
	  if amount == "" || amount == "0" {
		   amount = "1"
	  }
	  return s.Add(KindDeposit, user, asset, key, amount)
   }

// DepVelocity 窗口内同一 dep: 键的笔数。非 dep: 键返回 0，避免误计出金。
  func (s *Store) DepVelocity(key string, win time.Duration) int {
	  if s == nil || !IsDepKey(key) {
		  return 0
	   }
	   user, _, asset, err := ParseDepKey(key)
	   if err != nil {
		   return 0
	  }
	  if win <= 0 {
		   win = time.Minute
	  }
	  cut := s.now().Add(-win)
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  n := 0
	  for _, h := range s.hits {
		   if h.Kind != KindDeposit || h.UserID != user || h.Asset != asset {
			   continue
		   }
		  if h.Dest != key {
			  continue
		  }
		   if h.At.Before(cut) {
			  continue
		   }
		  n++
	  }
	  return n
  }
