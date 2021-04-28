package destlimit

import (
	"strings"
	"time"
	"github.com/shopspring/decimal"
)

   func scaleLimit(base, mul, extra string) (decimal.Decimal, error) {
	   b, err := decimal.NewFromString(strings.TrimSpace(base))
	   if err != nil || b.IsNegative() {
		  return decimal.Zero, ErrPolicy
	  }
	  m, err := decimal.NewFromString(strings.TrimSpace(mul))
	  if err != nil || !m.IsPositive() {
		   return decimal.Zero, ErrPolicy
	   }
	  out := b.Mul(m)
	  if strings.TrimSpace(extra) != "" && extra != "0" {
		   ex, err := decimal.NewFromString(strings.TrimSpace(extra))
		   if err != nil || ex.IsNegative() {
			   return decimal.Zero, ErrOverride
		  }
		   out = out.Add(ex)
	   }
	   return out, nil
  }

   func (e *Engine) quote(user, asset, chain, dest, cluster, tier, score string, now time.Time) (Quote, error) {
	   pol, err := e.pickPolicy(asset)
	  if err != nil {
		  return Quote{}, err
	  }
	   extra, ov := e.activeExtra(user, asset, cluster, now)
	   mul := pol.mul(tier)
	  cl,
		  err := scaleLimit(pol.ClusterMax, mul, extra)
	  if err != nil {
		  return Quote{}, err
	  }
	   ul, err := scaleLimit(pol.UserMax, mul, extra)
	  if err != nil {
		  return Quote{}, err
	   }
	   dl, err := scaleLimit(pol.DestMax, mul, extra)
	   if err != nil {
		  return Quote{}, err
	   }
	   cut := now.Add(-time.Duration(pol.WindowSec) * time.Second)
	   cu, uu, du := e.used(user, asset, chain, dest, cluster, cut)
	   q := Quote{
		  PolicyID: pol.ID, Tier: tier, Score: score, ClusterID: cluster, Multiplier: mul,
		  ClusterLimit: cl.String(), UserLimit: ul.String(), DestLimit: dl.String(),
		   ClusterUsed: cu.String(), UserUsed: uu.String(), DestUsed: du.String(),
		   Extra: extra,
	   }
	  if ov != nil {
		  q.OverrideID = ov.ID
	  }
	   return q, nil
   }

  func (e *Engine) used(user, asset, chain, dest, cluster string, cut time.Time) (clu, usr, dst decimal.Decimal) {
	  for _, h := range e.holds {
		   if h == nil || h.UserID != user || h.Asset != asset {
			  continue
		  }
		  if h.State != HoldReserved && h.State != HoldConsumed {
			   continue
		  }
		  if h.ReservedAt.Before(cut) {
			   continue
		   }
		   amt, err := decimal.NewFromString(h.Amount)
		  if err != nil {
			   continue
		  }
		   usr = usr.Add(amt)
		   if h.ClusterID == cluster {
			   clu = clu.Add(amt)
		   }
		   if h.Chain == chain && strings.EqualFold(h.Dest, dest) {
			  dst = dst.Add(amt)
		   }
	   }
	  return clu, usr, dst
   }

  func over(used decimal.Decimal, amount string, limit decimal.Decimal) bool {
	  amt, err := decimal.NewFromString(strings.TrimSpace(amount))
	  if err != nil {
		   return true
	   }
	  return used.Add(amt).GreaterThan(limit)
   }

   func decide(q Quote, amount string) *DenyError {
	   cl, _ := decimal.NewFromString(q.ClusterLimit)
	  ul, _ := decimal.NewFromString(q.UserLimit)
	   dl, _ := decimal.NewFromString(q.DestLimit)
	  cu, _ := decimal.NewFromString(q.ClusterUsed)
	   uu, _ := decimal.NewFromString(q.UserUsed)
	   du, _ := decimal.NewFromString(q.DestUsed)
	  if over(cu, amount, cl) {
		   return &DenyError{Code: CodeCluster, Reason: "cluster occupancy over dynamic limit", Limit: q.ClusterLimit, Used: q.ClusterUsed, ClusterID: q.ClusterID}
	  }
	  if over(uu, amount, ul) {
		   return &DenyError{Code: CodeUser, Reason: "user occupancy over dynamic limit", Limit: q.UserLimit, Used: q.UserUsed, ClusterID: q.ClusterID}
	   }
	   if over(du, amount, dl) {
		  return &DenyError{Code: CodeDest, Reason: "destination occupancy over dynamic limit", Limit: q.DestLimit, Used: q.DestUsed, ClusterID: q.ClusterID}
	  }
	   return nil
   }
