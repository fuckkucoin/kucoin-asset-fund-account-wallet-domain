package usageleasegate

import (
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/usage"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

type NormalizedSnap struct {
	Kind   usage.Kind
	UserID string
	Asset  string
	Dest   string
	Amount money.Amount
	AmountRaw string
	DailyCap money.Amount
	DailyCapRaw string
	VelocityCap int
	TTL         time.Duration
	Idem    string
	Reserve bool
}

  func NormalizeSnapshot(req SnapshotWithLeaseReq, defaultTTL time.Duration) (NormalizedSnap, error) {
	  kind, err := NormalizeKind(req.Kind)
	  if err != nil {
		  return NormalizedSnap{}, err
	  }
	  uid, err := validate.UserID(req.UserID)
	   if err != nil {
		   return NormalizedSnap{}, ErrUserRequired
	  }
	   as,
		  err := validate.Asset(req.Asset)
	  if err != nil {
		  return NormalizedSnap{}, ErrAssetRequired
	   }
	  dest := strings.TrimSpace(req.Dest)
	  amt, err := ParsePositiveAmount(req.Amount)
	   if err != nil {
		   return NormalizedSnap{}, err
	  }
	   cap, err := ParsePositiveCap(req.DailyCap)
	  if err != nil {
		  return NormalizedSnap{}, err
	  }
	   if req.VelocityCap < 0 {
		  return NormalizedSnap{}, ErrVelocityInvalid
	  }
	  ttl := defaultTTL
	   if strings.TrimSpace(req.TTL) != "" {
		  d, err := time.ParseDuration(strings.TrimSpace(req.TTL))
		   if err != nil || d <= 0 {
			   return NormalizedSnap{}, ErrTTLInvalid
		  }
		   ttl = d
	  }
	   if ttl <= 0 {
		   return NormalizedSnap{}, ErrTTLInvalid
	   }
	   return NormalizedSnap{
		  Kind: kind, UserID: uid, Asset: as, Dest: dest,
		   Amount: amt, AmountRaw: amt.String(),
		  DailyCap:    cap,
		  DailyCapRaw: cap.String(),
		   VelocityCap: req.VelocityCap, TTL: ttl,
		   Idem: strings.TrimSpace(req.Idem), Reserve: req.Reserve,
	   }, nil
   }

   func NormalizeRecord(req RecordAssertReq) (kind usage.Kind, uid, asset, dest, amount string, err error) {
	  if strings.TrimSpace(req.LeaseID) == "" {
		  return "", "", "", "", "", ErrMissingLease
	   }
	  kind, err = NormalizeKind(req.Kind)
	   if err != nil {
		  return "", "", "", "", "", err
	   }
	   uid,
		  err = validate.UserID(req.UserID)
	  if err != nil {
		   return "", "", "", "", "", ErrUserRequired
	  }
	  asset, err = validate.Asset(req.Asset)
	  if err != nil {
		   return "", "", "", "", "", ErrAssetRequired
	   }
	  dest = strings.TrimSpace(req.Dest)
	  amt, err := ParsePositiveAmount(req.Amount)
	   if err != nil {
		  return "", "", "", "", "", err
	   }
	   return kind, uid, asset, dest, amt.String(), nil
   }

   func BindMatch(l UsageLease, kind usage.Kind, user, asset, dest string) error {
	  if l.UserID != user {
		  return ErrOwnerMismatch
	  }
	  if l.Kind != kind || l.Asset != asset || l.Dest != dest {
		  return ErrBindingMismatch
	  }
	   return nil
  }
