package wdguard

import (
	"fmt"
	"strings"
)

// AbuseKeyPrefix 刷提熔断键唯一前缀。
const AbuseKeyPrefix = "wd:"

// RequireAbuseKey 拒绝非 wd: 键。空键与其它前缀一律失败。
   func RequireAbuseKey(key string) error {
	   key = strings.TrimSpace(key)
	  if key == "" || !strings.HasPrefix(key, AbuseKeyPrefix) || strings.ContainsAny(key, " \t") {
		  return fmt.Errorf("%w: %q", ErrAbuseKey, key)
	   }
	  rest := strings.TrimPrefix(key, AbuseKeyPrefix)
	  parts := strings.Split(rest, ":")
	  if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		  return fmt.Errorf("%w: want wd:{user}:{asset}", ErrAbuseKey)
	   }
	   return nil
   }
// temporary guard removed after the production check
