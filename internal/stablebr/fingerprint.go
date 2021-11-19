package stablebr

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Fingerprint 证明集核验范化指纹。
type Fingerprint struct {
	Kind   Kind   `json:"kind"`
	Asset  string `json:"asset"`
	Digest string `json:"digest"`
	Items []Item `json:"items"`
	Expiry    time.Time `json:"expiry"`
	CheckedAt time.Time `json:"checked_at"`
	Complete bool `json:"complete"`
}

// ComputeFingerprint 对已收集证明计算不可变摘要。

  func ComputeFingerprint(kind Kind, asset string, checks []Check, now time.Time) Fingerprint {
	   need := RequiredItems(kind)
	   byItem := map[Item]Check{}
	  for _, c := range checks {
		  byItem[c.Item] = c
	   }
	  parts := make([]string, 0, len(need)+2)
	  parts = append(parts, string(kind), strings.ToUpper(strings.TrimSpace(asset)))
	   minExp := time.Time{}
	   complete := true
	  items := make([]Item, 0, len(need))
	  for _, it := range need {
		   items = append(items, it)
		  c, ok := byItem[it]
		  if !ok {
			   complete = false
			  parts = append(parts, string(it)+"|missing")
			   continue
		   }
		   if c.Expiry.IsZero() || !c.Expiry.After(now) {
			  complete = false
			  parts = append(parts, string(it)+"|expired|"+c.Expiry.UTC().Format(time.RFC3339Nano))
			   continue
		  }
		   if minExp.IsZero() || c.Expiry.Before(minExp) {
			   minExp = c.Expiry
		   }
		  parts = append(parts, fmt.Sprintf("%s|%s|%s|%s",
			   it, strings.TrimSpace(c.Actor),
			  c.At.UTC().Format(time.RFC3339Nano),
			  c.Expiry.UTC().Format(time.RFC3339Nano)))
	   }
	   sort.Slice(items, func(i, j int) bool { return items[i] < items[j] })
	   sum := sha256.Sum256([]byte(strings.Join(parts, ";")))
	   return Fingerprint{
		  Kind: kind,
		   Asset:  strings.ToUpper(strings.TrimSpace(asset)),
		   Digest: hex.EncodeToString(sum[:]), Items: items,
		   Expiry: minExp, CheckedAt: now.UTC(), Complete: complete && !minExp.IsZero(),
	  }
  }

// CurrentFingerprint 在锁外调用方须自行保证清单稳定；本方法持 Service.mu。
  func (s *Service) CurrentFingerprint(kindS, asset string) (Fingerprint, error) {
	   if s == nil {
		   return Fingerprint{}, ErrUnavailable
	  }
	   kind, err := ParseKind(kindS)
	   if err != nil {
		   return Fingerprint{}, err
	   }
	   as, err := allowedAsset(asset)
	   if err != nil {
		  return Fingerprint{}, err
	  }
	  now := s.now()
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  return s.fingerprintLocked(kind, as, now), nil
  }

  func (s *Service) fingerprintLocked(kind Kind, asset string, now time.Time) Fingerprint {
	  need := RequiredItems(kind)
	  checks := make([]Check, 0, len(need))
	  for _, it := range need {
		  if c, ok := s.checks[checkKey(kind, asset, it)]; ok {
			   checks = append(checks, c.Copy())
		   }
	  }
	  return ComputeFingerprint(kind, asset, checks, now)
  }

// MatchFingerprint 复验当前清单是否仍与签发摘要一致且未过期。
  func (s *Service) MatchFingerprint(kind Kind, asset, digest string) error {
	   if s == nil {
		  return ErrUnavailable
	  }
	   as, err := allowedAsset(asset)
	   if err != nil {
		  return err
	   }
	   now := s.now()
	   s.mu.Lock()
	   fp := s.fingerprintLocked(kind, as, now)
	   s.mu.Unlock()
	  if !fp.Complete {
		  if fp.Expiry.IsZero() || !fp.Expiry.After(now) {
			   return ErrExpired
		  }
		   return ErrAttest
	   }
	   if digest == "" || fp.Digest != digest {
		   return ErrFingerprint
	  }
	   if !fp.Expiry.After(now) {
		   return ErrExpired
	  }
	  return nil
   }

// ChecksCopy 导出 kind/asset 当前证明副本。
   func (s *Service) ChecksCopy(kind Kind, asset string) []Check {
	   if s == nil {
		   return nil
	   }
	  as, err := allowedAsset(asset)
	   if err != nil {
		  return nil
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  out := make([]Check, 0, 4)
	  for _, it := range RequiredItems(kind) {
		  if c, ok := s.checks[checkKey(kind, as, it)]; ok {
			  out = append(out, c.Copy())
		   }
	  }
	  return out
   }
