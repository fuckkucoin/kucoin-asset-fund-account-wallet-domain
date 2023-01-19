package usage

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/pkg/money"
)

// WindowFingerprint 日累计+速度+去向计数指纹，供租约复核。

   func (s *Store) WindowFingerprint(kind Kind, user, asset, dest string) string {
	   if s == nil {
		   return ""
	  }
	  today := s.TodaySum(kind, user, asset).String()
	  v1 := s.Velocity(kind, user, asset, time.Hour)
	  v24 := s.Velocity(kind,
		   user, asset, 24*time.Hour)
	  dn := s.DestCount(user, dest, 24*time.Hour)
	   raw := strings.Join([]string{
		   string(kind), user, strings.ToUpper(asset), dest, today,
		  strconv.Itoa(v1), strconv.Itoa(v24), strconv.Itoa(dn),
	  }, "|")
	   sum := sha256.Sum256([]byte(raw))
	  return hex.EncodeToString(sum[:])
   }

// CapFingerprint 限额指纹。
   func CapFingerprint(dailyCap string, velocityCap int) string {
	   raw := dailyCap + "#" + strconv.Itoa(velocityCap)
	   sum := sha256.Sum256([]byte(raw))
	   return hex.EncodeToString(sum[:])
  }

// RecordIfAccepted 仅在调用方已判定接受时记账。失败单不要走这里。
  func (s *Store) RecordIfAccepted(kind Kind, user, asset, dest, amount string, accepted bool) error {
	   if !accepted {
		   return nil
	  }
	   return s.Record(kind, user, asset, dest, amount)
   }

// WouldExceedDaily 预检：今日占用 + 金额是否超过 cap。不写入。
  func (s *Store) WouldExceedDaily(kind Kind, user, asset, amount, cap string) (bool, error) {
	  if s == nil {
		  return true, ErrUnavailable
	   }
	  max, err := money.Parse(cap)
	   if err != nil {
		  return true, ErrAmount
	  }
	   add, err := money.Parse(amount)
	   if err != nil {
		  return true, ErrAmount
	   }
	   sum,
		  err := s.TodaySum(kind, user, asset).Add(add)
	  if err != nil {
		   return true, err
	  }
	  return sum.Cmp(max) > 0, nil
  }
