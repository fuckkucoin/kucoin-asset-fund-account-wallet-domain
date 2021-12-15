package usageleasegate

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/usage"
)

  func NormalizeKindOrEmpty(s string) usage.Kind {
	   k, err := NormalizeKind(usage.Kind(strings.TrimSpace(s)))
	  if err != nil {
		   return usage.Kind(strings.TrimSpace(s))
	   }
	   return k
   }
