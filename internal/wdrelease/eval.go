package wdrelease

import (
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/shopspring/decimal"
)

   func (d *Desk) Eval(in Input) (Decision, error) {
	  if d == nil {
		  return Decision{}, ErrUnavailable
	   }
	   d.mu.Lock()
	   defer d.mu.Unlock()
	   return d.evalLocked(in)
  }

   func (d *Desk) evalLocked(in Input) (Decision, error) {
	   norm, amt, err := normalizeInput(in)
	  if err != nil {
		   return Decision{}, err
	   }
	   ver, drift, err := d.pickVersion(norm.Version)
	  if err != nil {
		   return Decision{}, err
	  }
	  dec, err := decide(ver, norm, amt)
	   if err != nil {
		  return Decision{}, err
	  }
	   dec.Drift = drift
	  if drift {
		  d.noteDriftLocked("eval:" + ver.ID)
	   }
	  dec.Reason = explain(dec)
	  return dec, nil
  }

  func normalizeInput(in Input) (Input, decimal.Decimal, error) {
	   uid, err := trimUser(in.UserID)
	  if err != nil {
		  return Input{}, decimal.Zero, err
	  }
	   asset, err := trimAsset(in.Asset)
	  if err != nil {
		   return Input{}, decimal.Zero, err
	  }
	  ch, err := trimChain(in.Chain)
	  if err != nil {
		   return Input{}, decimal.Zero, err
	   }
	   dest, err := trimDest(ch, in.Dest)
	   if err != nil {
		  return Input{}, decimal.Zero, err
	  }
	   parsed, err := money.Parse(in.Amount)
	  if err != nil {
		   return Input{}, decimal.Zero, WithCode(ErrAmount, CodeAmount)
	   }
	  class := strings.ToLower(strings.TrimSpace(in.DestClass))
	   if class == "" {
		  class = ClassExternal
	  }
	   if in.Risk < 0 || in.Risk > 100 {
		   return Input{},
			   decimal.Zero, WithCode(ErrPolicy, CodePolicy)
	   }
	  in.UserID = uid
	   in.Asset = asset
	   in.Chain = ch
	   in.Dest = dest
	  in.DestClass = class
	  in.Amount = parsed.String()
	   in.Applicant = strings.TrimSpace(in.Applicant)
	   in.Idem = strings.TrimSpace(in.Idem)
	   in.Version = strings.TrimSpace(in.Version)
	   return in, parsed.Dec(), nil
   }

   func (d *Desk) pickVersion(pin string) (Version, bool, error) {
	   if d.book == nil {
		   return Version{}, false, WithCode(ErrPolicy, CodePolicy)
	  }
	  drift := false
	   id := d.activeID
	   if pin != "" {
		  id = pin
		  if pin != d.activeID {
			   drift = true
		   }
	  }
	   if id == "" {
		   return Version{}, false, WithCode(ErrPolicy, CodePolicy)
	   }
	  v, ok := d.book.Get(id)
	  if !ok {
		   return Version{}, false, WithCode(ErrVersion, CodeVersion)
	   }
	   return v, drift, nil
   }

   func decide(v Version, in Input, amt decimal.Decimal) (Decision, error) {
	   band, ok := matchBand(v, in.Asset, amt)
	   if !ok {
		  return Decision{Version: v.ID,
			   Deny: true, Code: CodePolicy, Timeout: TimeoutNone}, nil
	  }
	   dec := Decision{
		  Version: v.ID,
		   Depth:   band.Depth,
		  BaseDepth: band.Depth,
		   Skip: band.Skip,
		   Ratios:     append([]int(nil), band.Ratios...),
		  SLASec:     band.SLASec,
		  Timeout:    band.Timeout,
		   SegmentCap: band.SegmentCap,
		   DailyCap: band.DailyCap,
		  DestCap: band.DestCap,
	  }
	  rule, ok := destRule(v, in.DestClass)
	   if !ok {
		  dec.Deny = true
		  dec.Code = CodeDest
		  dec.Skip = false
		   return dec, nil
	   }
	   if rule.Deny {
		   dec.Deny = true
		  dec.Code = rule.Code
		  dec.Skip = false
		  return dec, nil
	   }
	   if rule.DisableSkip {
		   dec.Skip = false
	   }
	  dec.Depth += rule.AddDepth
	  if rule.MinDepth > dec.Depth {
		  dec.Depth = rule.MinDepth
	  }
	   if rule.SLAPercent > 0 && dec.SLASec > 0 {
		   dec.SLASec = dec.SLASec * int64(rule.SLAPercent) / 100
		   if dec.SLASec < 1 && !dec.Skip {
			   dec.SLASec = 1
		  }
	  }
	  for _, r := range v.Risk {
		   if in.Risk < r.MinScore {
			  continue
		  }
		   if r.Deny {
			   dec.Deny = true
			  dec.Code = r.Code
			   dec.Skip = false
			   return dec, nil
		   }
		  if r.DisableSkip {
			   dec.Skip = false
		  }
		  dec.Depth += r.AddDepth
		  if r.MinDepth > dec.Depth {
			   dec.Depth = r.MinDepth
		  }
	   }
	   if dec.Depth > v.MaxDepth {
		  dec.Depth = v.MaxDepth
	   }
	  if dec.Skip {
		   dec.Depth = 0
		  dec.Ratios = []int{10000}
		  dec.SLASec = 0
		  dec.Timeout = TimeoutNone
	   } else if dec.Depth < 1 {
		   dec.Depth = 1
	   }
	   ratios, err := fitRatios(dec.Ratios, dec.Depth, band.Depth)
	  if err != nil {
		  return Decision{}, err
	   }
	  dec.Ratios = ratios
	  parts, err := splitBps(amt, ratios)
	  if err != nil {
		   return Decision{}, err
	  }
	   if band.SegmentCap != "" {
		  cap, err := decimal.NewFromString(band.SegmentCap)
		   if err != nil {
			   return Decision{}, WithCode(ErrPolicy, CodePolicy)
		  }
		  for _, p := range parts {
			   if p.GreaterThan(cap) {
				   dec.Deny = true
				   dec.Code = CodeSegmentCap
				   dec.Skip = false
				   return dec, nil
			   }
		   }
	   }
	   dec.Amounts = make([]string, len(parts))
	  for i, p := range parts {
		  dec.Amounts[i] = p.String()
	   }
	   return dec, nil
  }

   func destRule(v Version, class string) (DestRule, bool) {
	   for _, d := range v.Dest {
		  if d.Class == class {
			  return d, true
		  }
	   }
	  return DestRule{}, false
  }

// fitRatios 风险把深度抬高时保留原分段，不新切一刀金额。
   func fitRatios(ratios []int, depth, base int) ([]int, error) {
	  if depth <= 0 {
		   return []int{10000}, nil
	  }
	   if len(ratios) == depth {
		  sum := 0
		  for _, r := range ratios {
			   sum += r
		  }
		   if sum != 10000 {
			  return nil, WithCode(ErrPolicy, CodePolicy)
		  }
		   return append([]int(nil), ratios...), nil
	   }
	  if depth > len(ratios) && depth > base {
		   sum := 0
		   for _, r := range ratios {
			  if r <= 0 {
				   return nil, WithCode(ErrPolicy, CodePolicy)
			   }
			  sum += r
		   }
		  if sum != 10000 {
			   return nil, WithCode(ErrPolicy, CodePolicy)
		  }
		  return append([]int(nil), ratios...), nil
	  }
	  return nil, fmt.Errorf("%w: ratios %d depth %d", ErrPolicy, len(ratios), depth)
   }
