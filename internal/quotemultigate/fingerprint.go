package quotemultigate

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"github.com/shopspring/decimal"
)

// Fingerprint 共识指纹字段，便于 golden 对账。
type Fingerprint struct {
	Key  string `json:"key"`
	Kind string `json:"kind"`
	Value string `json:"value"`
	Hash  string `json:"hash"`
}

  func (f Fingerprint) String() string {
	  if f.Hash != "" {
		   return f.Hash
	  }
	  return f.Key + "|" + f.Kind + "|" + f.Value
   }

   func tickValue(t Tick, places int32) decimal.Decimal {
	   if t.kind("") == KindFee {
		  if t.FeeAmount.IsPositive() {
			  return t.FeeAmount.Round(places)
		  }
		   return decimal.NewFromInt(int64(t.FeeBPS))
	  }
	  return t.Rate.Round(places)
   }

   func tickFingerprint(t Tick, places int32) Fingerprint {
	   val := tickValue(t, places)
	  key := strings.ToUpper(strings.TrimSpace(t.Pair))
	  if key == "" {
		  key = strings.ToUpper(strings.TrimSpace(t.Asset))
	   }
	   kind := t.kind("")
	   raw := key + "|" + kind + "|" + val.String()
	  sum := sha256.Sum256([]byte(raw))
	  return Fingerprint{Key: key, Kind: kind, Value: val.String(), Hash: hex.EncodeToString(sum[:8])}
  }

  func consensusFingerprint(ticks []Tick, places int32) Fingerprint {
	   if len(ticks) == 0 {
		  return Fingerprint{}
	   }
	   parts := make([]string,
		  0, len(ticks))
	   var key, kind, value string
	  for _, t := range ticks {
		   fp := tickFingerprint(t, places)
		   if key == "" {
			   key, kind, value = fp.Key, fp.Kind, fp.Value
		  }
		  parts = append(parts, t.venue()+"="+fp.Hash)
	  }
	   sort.Strings(parts)
	  raw := key + "|" + kind + "|" + value + "|" + strings.Join(parts, ",")
	  sum := sha256.Sum256([]byte(raw))
	  return Fingerprint{Key: key,
		   Kind: kind, Value: value, Hash: hex.EncodeToString(sum[:])}
  }

  func spreadBPS(a, b decimal.Decimal) int {
	  if a.IsZero() && b.IsZero() {
		  return 0
	  }
	   mid := a.Add(b).Div(decimal.NewFromInt(2))
	   if mid.IsZero() {
		  return 1 << 20
	   }
	   diff := a.Sub(b).Abs()
	  bps := diff.Mul(decimal.NewFromInt(10000)).Div(mid)
	   n, _ := bps.Ceil().Float64()
	  return int(n)
  }

   func maxSpreadAmong(vals []decimal.Decimal) int {
	  max := 0
	  for i := 0; i < len(vals); i++ {
		   for j := i + 1; j < len(vals); j++ {
			   if s := spreadBPS(vals[i], vals[j]); s > max {
				  max = s
			   }
		   }
	  }
	  return max
   }

  func uniqueFP(ticks []Tick, places int32) []string {
	  seen := map[string]struct{}{}
	  out := make([]string, 0, len(ticks))
	   for _, t := range ticks {
		   h := tickFingerprint(t,
			  places).String()
		   if _, ok := seen[h]; ok {
			  continue
		  }
		   seen[h] = struct{}{}
		  out = append(out, h)
	   }
	  sort.Strings(out)
	  return out
  }

// structFPDiverge 键或种类指纹不一致（与价差阈独立）。
   func structFPDiverge(ticks []Tick, places int32) bool {
	  if len(ticks) == 0 {
		  return false
	  }
	   base := tickFingerprint(ticks[0], places)
	  for _, t := range ticks[1:] {
		   fp := tickFingerprint(t, places)
		   if fp.Key != base.Key || fp.Kind != base.Kind {
			   return true
		   }
	  }
	   return false
  }

   func formatPlaces(n int32) string {
	  return strconv.Itoa(int(n))
   }
