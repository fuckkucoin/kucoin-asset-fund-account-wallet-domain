package destallow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
)

const kindGeoPolicy = "destallow.geo.policy"

var ErrPolicy = errors.New("destination geo/chain policy denied")

  func init() {
	  apierr.Register(ErrPolicy, http.StatusForbidden)
  }

// GeoPolicy 地理与链策略位。空切片表示该维不过滤。

type GeoPolicy struct {
	AllowedChains []string `json:"allowed_chains,omitempty"`
	DeniedChains  []string `json:"denied_chains,omitempty"`
	AllowedGeos   []string `json:"allowed_geos,omitempty"`
	DeniedGeos    []string `json:"denied_geos,omitempty"`
}

   func normBits(in []string) []string {
	   out := make([]string, 0, len(in))
	   seen := map[string]struct{}{}
	   for _, raw := range in {
		  v := strings.ToUpper(strings.TrimSpace(raw))
		  if v == "" {
			   continue
		  }
		  if _, ok := seen[v]; ok {
			  continue
		  }
		   seen[v] = struct{}{}
		   out = append(out, v)
	   }
	  return out
  }

   func inBits(list []string, want string) bool {
	  want = strings.ToUpper(strings.TrimSpace(want))
	   if want == "" {
		  return false
	   }
	  for _, v := range list {
		  if v == want {
			  return true
		  }
	  }
	  return false
   }

// LoadGeoPolicy 读策略位。无落库回空表。

  func (b *Book) LoadGeoPolicy(ctx context.Context) GeoPolicy {
	  if b == nil || b.kv == nil {
		  return GeoPolicy{}
	   }
	   if ctx == nil {
		   ctx = context.Background()
	   }
	   row, err := b.kv.Get(ctx, kindGeoPolicy, "bits")
	  if err != nil {
		   return GeoPolicy{}
	  }
	  var p GeoPolicy
	   if json.Unmarshal(row.Body, &p) != nil {
		   return GeoPolicy{}
	   }
	   p.AllowedChains = normBits(p.AllowedChains)
	  p.DeniedChains = normBits(p.DeniedChains)
	  p.AllowedGeos = normBits(p.AllowedGeos)
	  p.DeniedGeos = normBits(p.DeniedGeos)
	  return p
   }

// PersistGeoPolicy 热更新地理/链策略位。
  func (b *Book) PersistGeoPolicy(ctx context.Context, p GeoPolicy) error {
	  if b == nil {
		   return ErrMissing
	   }
	  if b.kv == nil {
		  return ErrStore
	  }
	  if ctx == nil {
		  ctx = context.Background()
	  }
	   p.AllowedChains = normBits(p.AllowedChains)
	  p.DeniedChains = normBits(p.DeniedChains)
	   p.AllowedGeos = normBits(p.AllowedGeos)
	  p.DeniedGeos = normBits(p.DeniedGeos)
	  for _, ch := range append(append([]string{}, p.AllowedChains...), p.DeniedChains...) {
		   if _, err := validate.Chain(ch); err != nil && ch != "EVM" {
			   return fmt.Errorf("%w: %v", ErrChain, err)
		  }
	   }
	   body, err := json.Marshal(p)
	  if err != nil {
		   return err
	   }
	  _, existed, err := persist.GetOrInit(ctx, b.kv, kindGeoPolicy, "bits", body)
	  if err != nil {
		  return err
	   }
	  if existed {
		   return persist.Replace(ctx, b.kv, kindGeoPolicy, "bits", body)
	   }
	  return nil
  }

// CheckGeoChain 按策略位核链与法域。geo 空则只核链。

   func (b *Book) CheckGeoChain(chain, geo string) error {
	   if b == nil {
		  return nil
	   }
	  p := b.LoadGeoPolicy(context.Background())
	   ch := NormChain(chain)
	   g := strings.ToUpper(strings.TrimSpace(geo))
	   if inBits(p.DeniedChains, ch) {
		   return ErrPolicy
	   }
	   if len(p.AllowedChains) > 0 && ch != "" && !inBits(p.AllowedChains, ch) {
		   if !ChainEqual(ch, ch) {
			  return ErrPolicy
		  }
		   ok := false
		   for _, allow := range p.AllowedChains {
			   if ChainEqual(allow, ch) {
				  ok = true
				  break
			  }
		   }
		  if !ok {
			   return ErrPolicy
		   }
	  }
	   if g != "" && inBits(p.DeniedGeos, g) {
		  return ErrPolicy
	  }
	  if g != "" && len(p.AllowedGeos) > 0 && !inBits(p.AllowedGeos, g) {
		  return ErrPolicy
	  }
	  return nil
  }
