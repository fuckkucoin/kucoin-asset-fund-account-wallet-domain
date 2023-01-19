package usage

import (
	"fmt"
	"time"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/validate"
)

var (
	ErrUnavailable = fmt.Errorf("usage store unavailable")
	ErrUser        = fmt.Errorf("usage user invalid")
	ErrAsset       = fmt.Errorf("usage asset invalid")
	ErrAmount = fmt.Errorf("usage amount invalid")
)

type WindowSnap struct {
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Kind Kind `json:"kind"`
	Today      string `json:"today"`
	Velocity1h int    `json:"velocity_1h"`
	Velocity24 int `json:"velocity_24h"`
	DestN int `json:"dest_count"`
}

// Record 只记已接受占用。科学计数失败即关。
   func (s *Store) Record(kind Kind, user, asset, dest, amount string) error {
	   if s == nil {
		  return ErrUnavailable
	   }
	  if _, err := validate.UserID(user); err != nil {
		   return ErrUser
	   }
	  if _, err := validate.Asset(asset); err != nil {
		  return ErrAsset
	   }
	  if _, err := money.Parse(amount); err != nil {
		  return ErrAmount
	   }
	  return s.Add(kind, user, asset, dest, amount)
   }

   func (s *Store) Snapshot(kind Kind, user, asset, dest string) (WindowSnap, error) {
	  if s == nil {
		  return WindowSnap{}, ErrUnavailable
	   }
	   uid, err := validate.UserID(user)
	   if err != nil {
		  return WindowSnap{}, ErrUser
	  }
	  as, err := validate.Asset(asset)
	   if err != nil {
		  return WindowSnap{}, ErrAsset
	  }
	   return WindowSnap{
		  UserID: uid, Asset: as, Kind: kind,
		  Today:      s.TodaySum(kind, uid, as).String(),
		  Velocity1h: s.Velocity(kind, uid, as, time.Hour),
		  Velocity24: s.Velocity(kind, uid, as, 24*time.Hour),
		  DestN: s.DestCount(uid, dest, 24*time.Hour),
	   }, nil
  }

  func (s *Store) BreachDaily(kind Kind, user, asset, cap string) (bool, error) {
	  if s == nil {
		  return true, ErrUnavailable
	   }
	   max, err := money.Parse(cap)
	   if err != nil {
		  return true, ErrAmount
	   }
	  sum := s.TodaySum(kind, user, asset)
	  return sum.Cmp(max) > 0, nil
  }
