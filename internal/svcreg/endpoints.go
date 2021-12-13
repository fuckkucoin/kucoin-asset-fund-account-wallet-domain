package svcreg

import (
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/mwtopo"
)

// NormalizeEndpoints 去空、去尾斜杠。

   func NormalizeEndpoints(endpoints []string) []string {
	  out := make([]string, 0, len(endpoints))
	   seen := map[string]struct{}{}
	   for _, ep := range endpoints {
		  ep = strings.TrimRight(strings.TrimSpace(ep), "/")
		  if ep == "" {
			  continue
		   }
		  if _, ok := seen[ep]; ok {
			   continue
		  }
		   seen[ep] = struct{}{}
// FilterRegionEndpoints 校验区位并返回核验列表。
   func FilterRegionEndpoints(endpoints []string) ([]string, error) {
	  eps := NormalizeEndpoints(endpoints)
	  if len(eps) == 0 {
		   return nil, ErrEndpoint
	  }
	  for _, ep := range eps {
		  if !mwtopo.RegionCIDROK(ep) {
			   return nil, fmt.Errorf("%w: region refused: %s", ErrEndpoint, ep)
		  }
	  }
	  return eps, nil
   }

// RotateEndpoints 把 start 下标轮到队首，便于故障转移避开坏节点。

  func RotateEndpoints(endpoints []string, start int) []string {
	   eps := NormalizeEndpoints(endpoints)
	   n := len(eps)
	   if n == 0 {
		  return eps
	   }
	  if start < 0 {
		  start = 0
	   }
	  start = start % n
	  if start == 0 {
		   return append([]string(nil), eps...)
	  }
	  out := make([]string, 0, n)
	   out = append(out, eps[start:]...)
	  out = append(out, eps[:start]...)
	  return out
  }

// CatalogEndpoints 按区位取目录三节点。

  func CatalogEndpoints(region string) []string {
	  return mwtopo.EtcdEndpoints(region)
  }
