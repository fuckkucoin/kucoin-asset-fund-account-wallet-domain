package travelvasp

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"account-assets.kucoin.net/funding/internal/metrics"
)

// Link 把当前信封接到前一封 checksum，形成证据链。空 prev 表示链首。
  func (e Envelope) Link(prevChecksum string) Envelope {
	  prev := strings.ToLower(strings.TrimSpace(prevChecksum))
	  e.PrevChecksum = prev
	  e.Canonical = strings.Join([]string{prev, envelopeBody(e)}, "|")
	   sum := sha256.Sum256([]byte(e.Canonical))
	   e.Checksum = hex.EncodeToString(sum[:])
	  return e
   }

  func (s *Service) attachLocked(env Envelope) (Envelope, error) {
	  linked := env.Link(s.chainHead)
	   if err := linked.Verify(); err != nil {
		   metrics.TravelEvidence.WithLabelValues("verify").Inc()
		   return Envelope{}, err
	  }
	  s.chain = append(s.chain, linked.Checksum)
	  s.chainHead = linked.Checksum
	  metrics.TravelEvidence.WithLabelValues("link").Inc()
	  return linked, nil
  }

  func (s *Service) ChainHead() string {
	  if s == nil {
		  return ""
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  return s.chainHead
   }

// VerifyChain 按入链顺序核 prev→checksum。断链失败即关。

   func (s *Service) VerifyChain() error {
	   if s == nil {
		   return ErrUnavailable
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  return s.verifyChainLocked()
   }

   func (s *Service) verifyChainLocked() error {
	   bySum := map[string]Envelope{}
	  for _, m := range s.msgs {
		   if m == nil {
			   continue
		   }
		   if err := m.Envelope.Verify(); err != nil {
			  metrics.TravelEvidence.WithLabelValues("broken").Inc()
			   return err
		   }
		  bySum[m.Envelope.Checksum] = m.Envelope
	   }
	   prev := ""
	   for _, sum := range s.chain {
		  env, ok := bySum[sum]
		   if !ok || env.PrevChecksum != prev {
			  metrics.TravelEvidence.WithLabelValues("broken").Inc()
			  return ErrChain
		  }
		  prev = sum
	  }
	  if s.chainHead != prev {
		  metrics.TravelEvidence.WithLabelValues("broken").Inc()
		   return ErrChain
	   }
	   return nil
   }

type Gap struct {
	ID     string `json:"id,omitempty"`
	Sum    string `json:"checksum,omitempty"`
	Reason string `json:"reason"`
}

// Gaps 扫描报文与 checksum 链的缺口。工程信封校验，不是申报通道。

   func (s *Service) Gaps() []Gap {
	  if s == nil {
		   return []Gap{{Reason: "unavailable"}}
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  return s.gapsLocked()
  }

   func (s *Service) gapsLocked() []Gap {
	   inChain := map[string]struct{}{}
	   for _, sum := range s.chain {
		   inChain[sum] = struct{}{}
	   }
	   bySum := map[string]*Message{}
	  out := make([]Gap, 0)
	   for _, m := range s.msgs {
		   if m == nil {
			  continue
		  }
		   if err := m.Envelope.Verify(); err != nil {
			  out = append(out, Gap{ID: m.ID, Reason: "hash"})
			   continue
		   }
		   bySum[m.Envelope.Checksum] = m
		   if _, ok := inChain[m.Envelope.Checksum]; !ok {
			  out = append(out, Gap{ID: m.ID, Sum: m.Envelope.Checksum, Reason: "orphan"})
		  }
	  }
	  prev := ""
	   for _, sum := range s.chain {
		   m, ok := bySum[sum]
		  if !ok {
			   out = append(out, Gap{Sum: sum, Reason: "missing"})
			  prev = sum
			   continue
		  }
		  if m.Envelope.PrevChecksum != prev {
			   out = append(out, Gap{ID: m.ID, Sum: sum, Reason: "prev"})
		  }
		  prev = sum
	   }
	  if s.chainHead != prev {
		   out = append(out, Gap{Sum: s.chainHead, Reason: "head"})
	   }
	  if len(out) > 0 {
		   metrics.TravelEvidence.WithLabelValues("gap").Inc()
	   }
	  return out
  }
