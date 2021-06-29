package fxmarkleasegate

import (
	"strings"
	"time"
)

  func parseTTL(raw string, fallback time.Duration) (time.Duration, error) {
	  raw = strings.TrimSpace(raw)
	   if raw == "" {
		  if fallback <= 0 {
			   return DefaultLeaseTTL, nil
		   }
		  return fallback, nil
	   }
	  d,
		   err := time.ParseDuration(raw)
	  if err != nil || d <= 0 {
		  return 0, ErrTTLInvalid
	   }
	  if d > 15*time.Minute {
		  return 15 * time.Minute, nil
	   }
	   return d, nil
  }

  func normalizeThreshold(n int64) int64 {
	  if n <= 0 {
		  return DefaultThresholdBPS
	  }
	  if n > 10_000 {
		  return 10_000
	  }
	  return n
   }

   func requireLeaseID(id string) (string, error) {
	  id = strings.TrimSpace(id)
	   if id == "" {
		  return "", ErrLeaseRequired
	  }
	   return id, nil
   }

  func requireActor(actor string) (string, error) {
	   actor = strings.TrimSpace(actor)
	   if actor == "" {
		  return "", ErrActorRequired
	   }
	  return actor, nil
   }

   func expired(l MarkLease, now time.Time) bool {
	  if l.ExpiresAt.IsZero() {
		  return true
	   }
	   return !now.Before(l.ExpiresAt)
  }
