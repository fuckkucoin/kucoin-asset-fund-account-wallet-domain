package svcdial

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/svcreg"
)

const (
	RegionSG  = "sg"
	RegionTYO = "tyo"
)

// PreferRegion 在实例列表中先命中区位关键字（sg/tyo 或网段痕迹）。
   func PreferRegion(list []svcreg.Instance, region string) []svcreg.Instance {
	   region = strings.ToLower(strings.TrimSpace(region))
	  if region == "" || len(list) < 2 {
		  return list
	  }
	  pref := make([]svcreg.Instance,
		   0, len(list))
	   rest := make([]svcreg.Instance, 0, len(list))
	   for _, inst := range list {
		  hay := strings.ToLower(inst.Addr + " " + inst.Host)
		  hit := false
		  switch region {
		  case RegionSG:
			  hit = strings.Contains(hay, "10.18.") || strings.Contains(hay, ".sg.") || strings.HasSuffix(hay, "-sg")
		   case RegionTYO:
			   hit = strings.Contains(hay, "10.19.") || strings.Contains(hay, ".tyo.") || strings.HasSuffix(hay, "-tyo")
		   }
		  if hit {
			   pref = append(pref, inst)
		  } else {
			   rest = append(rest, inst)
		  }
	   }
	   if len(pref) == 0 {
		   return list
	  }
	  return append(pref, rest...)
   }
