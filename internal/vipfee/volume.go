package vipfee

import (
	"strings"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/google/uuid"
)

   func parseBook(s string) (money.Amount, error) {
	   if strings.TrimSpace(s) == "" {
		  return money.Zero(), nil
	   }
	   return money.ParseBook(s)
  }

// RecordVolume 把成交名义量记进 30 日窗。科学计数拒。
   func (s *Service) RecordVolume(req VolumeReq) (Fill, error) {
	   if s == nil {
		   return Fill{}, ErrUnavailable
	  }
	   uid,
		   err := validate.UserID(req.UserID)
	  if err != nil {
		   return Fill{}, ErrUser
	  }
	  as, err := validate.Asset(req.Asset)
	  if err != nil {
		  return Fill{}, ErrAsset
	  }
	  amt, err := money.Parse(req.Notional)
	   if err != nil {
		   return Fill{}, ErrAmount
	  }
	   now := s.now()
	   row := Fill{
		  ID: uuid.NewString(), UserID: uid, Asset: as,
		  Notional: amt.String(), Day: now.UTC().Format("2006-01-02"), At: now,
	   }
	   s.mu.Lock()
	   s.fills = append(s.fills, row)
	   s.mu.Unlock()
	  if err := s.persistFill(row); err != nil {
		   s.mu.Lock()
		   if n := len(s.fills); n > 0 && s.fills[n-1].ID == row.ID {
			   s.fills = s.fills[:n-1]
		  }
		   s.mu.Unlock()
		   return Fill{}, err
	  }
	  return row.Copy(), nil
  }

  func (s *Service) WindowOf(userID, asset string) (string, error) {
	  if s == nil {
		  return "", ErrUnavailable
	  }
	  uid, err := validate.UserID(userID)
	   if err != nil {
		   return "", ErrUser
	   }
	  as := strings.ToUpper(strings.TrimSpace(asset))
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  sum, err := s.windowLocked(uid, as)
	   if err != nil {
		  return "", err
	   }
	  return sum.String(), nil
   }

   func (s *Service) windowLocked(userID, asset string) (money.Amount, error) {
	  cut := s.now().Add(-s.window)
	  sum := money.Zero()
	   for _, f := range s.fills {
		  if f.UserID != userID {
			  continue
		  }
		   if asset != "" && f.Asset != asset && asset != "*" {
			   continue
		  }
		   if f.At.Before(cut) {
			  continue
		   }
		  amt, err := money.ParseBook(f.Notional)
		  if err != nil {
			   return money.Zero(), err
		   }
		   next, err := sum.Add(amt)
		  if err != nil {
			   return money.Zero(), err
		  }
		  sum = next
	   }
	   return sum, nil
  }

  func (s *Service) Fills(userID string, limit int) []Fill {
	   if s == nil {
		  return nil
	  }
	   if limit <= 0 {
		   limit = 50
	  }
	   userID = strings.TrimSpace(userID)
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   out := make([]Fill, 0, limit)
	  for i := len(s.fills) - 1; i >= 0 && len(out) < limit; i-- {
		  if userID != "" && s.fills[i].UserID != userID {
			  continue
		  }
		  out = append(out, s.fills[i].Copy())
	   }
	   return out
  }
