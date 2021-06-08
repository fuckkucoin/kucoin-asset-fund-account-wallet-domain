package fundgraph

import (
	"fmt"
	"strings"
	"time"
)

   func invalidf(format string, args ...any) error {
	  return fmt.Errorf("%w: "+format, append([]any{ErrInvalid}, args...)...)
   }

   func cleanRef(ref string) (string, error) {
	  ref = strings.TrimSpace(ref)
	   if ref == "" {
		  return "", invalidf("ref required")
	   }
	  return ref, nil
  }

   func positiveAmount(amount string) (string, error) {
	  a, err := NormAmount(amount)
	  if err != nil {
		  return "", invalidf("amount")
	   }
	   if a == "" || a == "0" {
		   return "", invalidf("amount must be positive")
	  }
	   return a, nil
   }

  func atOr(t, fallback time.Time) time.Time {
	  if t.IsZero() {
		   return fallback.UTC()
	  }
	   return t.UTC()
   }

   func putAttr(m map[string]string, k, v string) map[string]string {
	  v = strings.TrimSpace(v)
	  if v == "" {
		  return m
	  }
	   if m == nil {
		   m = map[string]string{}
	   }
	  m[k] = v
	   return m
  }

  func baseNode(kind NodeKind, ref, user, asset, amount, source string, at time.Time) FundNode {
	   return FundNode{
		   ID:   NodeID(kind, ref),
		   Kind: kind,
		   Ref:  ref,
		   UserID:     strings.TrimSpace(user),
		   Asset:      NormAsset(asset),
		  Amount:     amount,
		  Source:     source,
		   OccurredAt: at,
	  }
   }

   func baseEdge(from,
	   to string, typ EdgeType, ref, user, asset, amount, source, envelope string, at time.Time) FundEdge {
	  return FundEdge{
		  ID: EdgeID(from,
			   to, typ, ref),
		   FromID: from,
		   ToID:   to,
		   EdgeType: typ,
		  Ref:    ref,
		   UserID: strings.TrimSpace(user),
		  Asset:  NormAsset(asset),
		   Amount: amount,
		  Source: source,
		   EnvelopeID: strings.TrimSpace(envelope),
		  OccurredAt: at,
	   }
  }

// MergeBatches 把多段摄取合成一批。后来源的发生时间取最晚的一个。

  func MergeBatches(parts ...IngestBatch) IngestBatch {
	  out := IngestBatch{Stream: "default"}
	  for _, p := range parts {
		  if out.Source == "" {
			   out.Source = p.Source
		   }
		  if p.Stream != "" && out.Stream == "default" {
			   out.Stream = p.Stream
		   }
		  if p.Cursor != "" {
			   out.Cursor = p.Cursor
		   }
		   if p.EventAt.After(out.EventAt) {
			   out.EventAt = p.EventAt
		   }
		   out.Nodes = append(out.Nodes, p.Nodes...)
		  out.Edges = append(out.Edges, p.Edges...)
	  }
	   return out
   }
