package dustquotegate

import "time"

// LiveQuote 活牌价快照视图。
type LiveQuote struct {
	Asset     string
	Rate      string
	PolicyGen int64
	CapBPS int64
	Floor  string
	MinConv   string
	BookAvail string
	At        time.Time
}

// QuoteSource dustconv 只读挂钩。

type QuoteSource interface {
	LiveQuote(user, asset string) (LiveQuote, error)
	PolicyGenOf(asset string) (int64,
		error)
	RateOf(asset string) (string, error)
	CapBPSOf(asset string) (int64, error)
}

// ToFingerprint 把活快照钉扎为指纹。
   func (q LiveQuote) ToFingerprint(user string) PolicyRateFingerprint {
	   fp := PolicyRateFingerprint{
		  Asset:     q.Asset,
		  Rate:      q.Rate,
		  PolicyGen: q.PolicyGen,
		   CapBPS: q.CapBPS,
		   FloorPinned: q.Floor,
		  MinPinned: q.MinConv,
		   BookAvail: q.BookAvail,
		   BookFP:    BookFingerprint(user, q.Asset, q.BookAvail),
	   }
	  fp.Digest = DigestFingerprint(fp)
	  return fp
   }

// CheckDrift 与 LeaseQuote.SameLeaseQuote 一致：任一牌价/世代/cap/账面指纹变化即漂移。
   func (q LiveQuote) CheckDrift(other LiveQuote) error {
	  if q.Rate != other.Rate {
		   return ErrRateDrift
	  }
	   if q.PolicyGen != other.PolicyGen {
		  return ErrPolicyDrift
	   }
	  if q.CapBPS != other.CapBPS {
		  return ErrCapDrift
	  }
	   return nil
  }
