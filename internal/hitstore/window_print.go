package hitstore

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

  func hourDur() time.Duration { return time.Hour }

   func dayDur() time.Duration { return 24 * time.Hour }

   func kindUserAssetPrint(kind, user, asset, dest, today string, v1, v24, destN int) string {
	   raw := strings.Join([]string{
		   kind, user, strings.ToUpper(strings.TrimSpace(asset)), dest, today,
		   strconv.Itoa(v1),
		  strconv.Itoa(v24), strconv.Itoa(destN),
	  }, "|")
	   sum := sha256.Sum256([]byte(raw))
	   return hex.EncodeToString(sum[:])
  }
