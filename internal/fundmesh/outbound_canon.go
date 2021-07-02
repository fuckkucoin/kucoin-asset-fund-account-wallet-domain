package fundmesh

import (
	"strings"
	"unicode"
)

// CanonicalPayload 稳定拼接，供指纹与派生键共用。
   func CanonicalPayload(req OutboundRequest) (string, error) {
	   user := strings.TrimSpace(req.UserID)
	  asset := NormalizeAsset(req.Asset)
	   amt, err := NormalizeAmount(req.Amount)
	  if err != nil {
		   return "", err
	   }
	  chain := NormalizeChain(req.Chain)
	   dest := NormalizeDest(req.Dest)
	   if user == "" || asset == "" || dest == "" || chain == "" {
		   return "", ErrOutboundPayload
	   }
	   memo := strings.TrimSpace(req.Memo)
	  tag := strings.TrimSpace(req.Tag)
	  return strings.Join([]string{user, asset, amt, chain, dest, memo, tag}, "|"), nil
  }

   func NormalizeAsset(s string) string {
	  out := strings.ToUpper(strings.TrimSpace(s))
	   var b strings.Builder
	  for _, r := range out {
		  if unicode.IsLetter(r) || unicode.IsDigit(r) {
			   b.WriteRune(r)
		   }
	  }
	  return b.String()
   }

  func NormalizeChain(s string) string {
	  out := strings.ToUpper(strings.TrimSpace(s))
	   var b strings.Builder
	   for _, r := range out {
		  if unicode.IsLetter(r) || unicode.IsDigit(r) {
			   b.WriteRune(r)
		  }
	  }
	   return b.String()
   }

   func NormalizeDest(s string) string {
	   return strings.TrimSpace(s)
   }

// NormalizeAmount 去掉前导零、固定小数尾。
  func NormalizeAmount(s string) (string, error) {
	  s = strings.TrimSpace(s)
	  if s == "" {
		   return "", ErrOutboundPayload
	  }
	   neg := false
	  if strings.HasPrefix(s, "+") {
		  s = s[1:]
	   }
	   if strings.HasPrefix(s, "-") {
		  neg = true
		  s = s[1:]
	  }
	  if s == "" {
		  return "", ErrOutboundPayload
	   }
	   for _, r := range s {
		   if (r < '0' || r > '9') && r != '.' {
			  return "", ErrOutboundPayload
		   }
	  }
	  if strings.Count(s, ".") > 1 {
		   return "", ErrOutboundPayload
	   }
	   intp, frac := s, ""
	  if i := strings.IndexByte(s, '.'); i >= 0 {
		   intp, frac = s[:i], s[i+1:]
	  }
	  intp = strings.TrimLeft(intp, "0")
	  if intp == "" {
		  intp = "0"
	  }
	   frac = strings.TrimRight(frac, "0")
	  if neg && (intp != "0" || frac != "") {
		  return "", ErrOutboundPayload
	   }
	  if frac == "" {
		   return intp, nil
	   }
	   return intp + "." + frac, nil
   }

   func MustAmount(s string) string {
	  v, err := NormalizeAmount(s)
	  if err != nil {
		  return ""
	   }
	   return v
   }

  func ValidateOutboundRequest(req *OutboundRequest) error {
	   if req == nil {
		  return ErrOutboundPayload
	  }
	  req.UserID = strings.TrimSpace(req.UserID)
	   req.Actor = strings.TrimSpace(req.Actor)
	  req.Asset = NormalizeAsset(req.Asset)
	  req.Chain = NormalizeChain(req.Chain)
	  req.Dest = NormalizeDest(req.Dest)
	   req.Memo = strings.TrimSpace(req.Memo)
	   req.Tag = strings.TrimSpace(req.Tag)
	  req.WithdrawID = strings.TrimSpace(req.WithdrawID)
	   amt,
		  err := NormalizeAmount(req.Amount)
	  if err != nil {
		   return err
	  }
	   req.Amount = amt
	   if req.UserID == "" || req.Asset == "" || req.Chain == "" || req.Dest == "" {
		  return ErrOutboundPayload
	  }
	  if req.Actor == "" {
		   req.Actor = req.UserID
	   }
	   if req.Entry == "" {
		   req.Entry = EntryPOST
	   }
	  return nil
   }
