package solvency

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Compare 生成准备金快照。观察读失败时覆盖结论为 unreadable，不得把缺失观察当成 0 后封成已覆盖。
// 负债大于观察时必开 DriftCase，并写明负债、观察、差额、基点与分档。
   func (p *Pipeline) Compare(ctx context.Context, id string) (Attestation, error) {
	   if p == nil || p.store == nil {
		   return Attestation{}, ErrUnavailable
	  }
	  att, err := p.store.GetAttestation(ctx, id)
	   if err != nil {
		  return Attestation{}, err
	   }
	  if att.Status != StatusBuilding {
		   return att, ErrImmutable
	   }
	   if att.Root == "" {
		  return att, ErrNotCompared
	   }
	   leaves,
		   err := p.store.ListLeaves(ctx, id)
	  if err != nil {
		  return att, err
	   }
	  now := p.now()
	  legs,
		   source, obsErr := p.observe(ctx)
	  snaps, drifts, coverage, err := diffReserve(att, leaves, legs, source, obsErr, now)
	   if err != nil {
		   return att, err
	   }
	  att.Coverage = coverage
	  att.Class, att.Disclaimer = classOf(att)
	   att.UpdatedAt = now
	  att.Solvency, att.Deprecated = deriveSolvency(att.Class)
	   if err := p.store.SaveCompare(ctx, att, snaps, drifts); err != nil {
		  return att, err
	   }
	  for i := range drifts {
		  p.emitDrift(ctx, drifts[i])
		  noteDrift(drifts[i].Severity)
	   }
	   noteCoverage(coverage)
	  return p.store.GetAttestation(ctx, id)
  }

  func diffReserve(att Attestation, leaves []LiabilityLeaf, legs []ReserveLeg, source string, obsErr error, now time.Time) ([]ReserveSnapshot, []DriftCase, string, error) {
	  liab := map[string]string{}
	  for _, l := range leaves {
		  cur := liab[l.Asset]
		  next, err := addAmt(cur, l.Total)
		  if err != nil {
			   return nil, nil, "", err
		  }
		   liab[l.Asset] = next
	  }
	   if obsErr != nil {
		  drifts := make([]DriftCase, 0, len(liab))
		   assets := mapKeys(liab)
		   if len(assets) == 0 {
			   assets = []string{"*"}
		   }
		   for _, asset := range assets {
			  drifts = append(drifts, DriftCase{
				   ID: uuid.NewString(), AttestationID: att.ID, Asset: asset,
				  Status: DriftOpen, Severity: SevDeficit,
				   Liability: or0(liab[asset]), Observed: "", Hot: "", Warm: "", Cold: "",
				   Deficit: or0(liab[asset]), RatioBps: 10000,
				   Explain:  fmt.Sprintf("准备金观察不可读 asset=%s liability=%s source=%s err=%v；禁止把缺失观察当成 0 后声称已覆盖", asset, or0(liab[asset]), SourceUnreadable, obsErr),
				   OpenedAt: now,
			  })
		   }
		   return nil, drifts, CoverageUnread, nil
	  }
	  obs := map[string]ReserveLeg{}
	   for _, lg := range legs {
		   if !scopeAllows(att.Scope, lg.Asset) {
			   continue
		  }
		  obs[lg.Asset] = lg
	   }
	  names := map[string]struct{}{}
	   for a := range liab {
		   names[a] = struct{}{}
	  }
	   for a := range obs {
		   names[a] = struct{}{}
	   }
	   assets := make([]string, 0, len(names))
	   for a := range names {
		  assets = append(assets, a)
	  }
	  sort.Strings(assets)
	  snaps := make([]ReserveSnapshot, 0, len(assets))
	   drifts := make([]DriftCase, 0)
	  coverage := CoverageCovered
	  for _, asset := range assets {
		  lg := obs[asset]
		   if lg.Asset == "" {
			   lg = ReserveLeg{Asset: asset, Hot: "0", Warm: "0", Cold: "0", Source: source}
		   }
		   if lg.Source == "" {
			  lg.Source = source
		   }
		   observed, err := addAmt(lg.Hot, lg.Warm, lg.Cold)
		  if err != nil {
			   return nil, nil, "", err
		  }
		  liability := or0(liab[asset])
		  surplus, deficit := "0", "0"
		  cmp, err := cmpAmt(liability, observed)
		  if err != nil {
			  return nil, nil, "", err
		   }
		  switch {
		  case cmp > 0:
			   deficit, err = subAmt(liability, observed)
			   if err != nil {
				  return nil, nil, "", err
			  }
		  case cmp < 0:
			   surplus, err = subAmt(observed, liability)
			  if err != nil {
				  return nil, nil, "", err
			   }
		   }
		   snaps = append(snaps, ReserveSnapshot{
			   ID: uuid.NewString(), AttestationID: att.ID, Asset: asset,
			   Hot: or0(lg.Hot), Warm: or0(lg.Warm), Cold: or0(lg.Cold),
			   Observed: observed, Liability: liability, Surplus: surplus, Deficit: deficit,
			   Source: lg.Source, TakenAt: now,
		  })
		   if deficit != "0" {
			  coverage = CoverageShort
			  sev, bps, err := classifyDeficit(liability, deficit)
			  if err != nil {
				   return nil, nil, "", err
			  }
			  drifts = append(drifts, DriftCase{
				  ID: uuid.NewString(), AttestationID: att.ID, Asset: asset,
				  Status: DriftOpen, Severity: sev,
				  Liability: liability, Observed: observed,
				  Hot: or0(lg.Hot),
				   Warm: or0(lg.Warm), Cold: or0(lg.Cold),
				  Deficit: deficit, RatioBps: bps,
				   Explain:  explainDeficit(asset, liability, observed, lg, deficit, bps, sev, att.FenceEpoch, lg.Source),
				   OpenedAt: now,
			   })
		   }
	  }
	  return snaps, drifts, coverage, nil
  }

   func explainDeficit(asset, liability, observed string, lg ReserveLeg, deficit string, bps int64, sev string, epoch int64, source string) string {
	   return fmt.Sprintf(
		   "准备金观察低于用户负债 asset=%s liability=%s observed=%s hot=%s warm=%s cold=%s deficit=%s ratio_bps=%d severity=%s fence_epoch=%d source=%s；分档 watch<100bps shortage<500bps deficit>=500bps",
		  asset, liability, observed, or0(lg.Hot), or0(lg.Warm), or0(lg.Cold), deficit, bps, sev, epoch, source,
	  )
   }

   func classifyDeficit(liability, deficit string) (string, int64, error) {
	  l, err := money.ParseBook(or0(liability))
	  if err != nil {
		  return "", 0, ErrAmount
	   }
	  d, err := money.ParseBook(or0(deficit))
	   if err != nil {
		  return "", 0, ErrAmount
	  }
	  if d.IsZero() {
		   return "", 0, nil
	   }
	  var bps int64
	  if l.IsZero() {
		   bps = 10000
	   } else {
		  bps = d.Dec().Mul(decimal.NewFromInt(10000)).Div(l.Dec()).IntPart()
		  if bps < 1 {
			   bps = 1
		  }
	   }
	  switch {
	   case bps >= 500:
		  return SevDeficit, bps, nil
	   case bps >= 100:
		   return SevShortage, bps, nil
	  default:
		  return SevWatch, bps, nil
	   }
   }

  func mapKeys(m map[string]string) []string {
	  out := make([]string, 0, len(m))
	   for k := range m {
		   out = append(out, k)
	  }
	  sort.Strings(out)
	  return out
  }

  func coverAssets(leaves []LiabilityLeaf, snaps []ReserveSnapshot) error {
	  need := map[string]struct{}{}
	  for _, l := range leaves {
		  need[l.Asset] = struct{}{}
	   }
	   have := map[string]struct{}{}
	   for _, s := range snaps {
		  have[s.Asset] = struct{}{}
	  }
	  missing := make([]string, 0)
	   for a := range need {
		   if _, ok := have[a]; !ok {
			  missing = append(missing, a)
		  }
	  }
	   if len(missing) > 0 {
		   sort.Strings(missing)
		  return fmt.Errorf("%w: %s", ErrReserveMissing, strconv.Quote(stringsJoin(missing)))
	  }
	  return nil
   }

   func stringsJoin(in []string) string {
	  out := ""
	   for i, s := range in {
		  if i > 0 {
			  out += ","
		  }
		  out += s
	   }
	   return out
   }
