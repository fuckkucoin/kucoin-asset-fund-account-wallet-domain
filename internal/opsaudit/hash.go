package opsaudit

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// LinkHash 计算当前环摘要。

   func LinkHash(seq int64, prev, kind, actor, subject, ticket, body string) string {
	  h := sha256.New()
	  _, _ = fmt.Fprintf(h, "%d|%s|%s|%s|%s|%s|%s",
		   seq,
		  strings.TrimSpace(prev),
		   strings.TrimSpace(kind),
		   strings.TrimSpace(actor),
		  strings.TrimSpace(subject),
		  strings.TrimSpace(ticket),
		  strings.TrimSpace(body),
	   )
	   return hex.EncodeToString(h.Sum(nil))
  }
