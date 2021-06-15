package fundmesh

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode"
)

const (
	idemMinLen = 12
	idemMaxLen = 128
	idemBodyMin = 8
)

// NormalizeOutboundKey 统一出站幂等键。
// 先 Header / ClientKey / 请求字段；全空则按意图派生。
  func NormalizeOutboundKey(req OutboundRequest) (string, IdemKeyParts, error) {
	  raw := firstNonEmpty(req.HeaderKey, req.ClientKey, req.IdempotencyKey)
	   raw = strings.TrimSpace(raw)
	   if raw == "" {
		   derived, parts, err := DeriveOutboundKey(req)
		   return derived, parts, err
	   }
	  parts, err := ParseOutboundKey(raw)
	  if err != nil {
		   canon, perr := CanonicalizeClientKey(raw)
		   if perr != nil {
			   return "", IdemKeyParts{}, err
		   }
		   parts, err = ParseOutboundKey(canon)
		  if err != nil {
			   return "", IdemKeyParts{}, err
		   }
		  return parts.Raw, parts, nil
	  }
	  return parts.Raw, parts, nil
   }

// ParseOutboundKey 解析 fmout.v1.{c|d}.<body>。

  func ParseOutboundKey(raw string) (IdemKeyParts, error) {
	  s := strings.TrimSpace(raw)
	  if s == "" {
		   return IdemKeyParts{},
			   ErrOutboundIdemRequired
	  }
	  if strings.HasPrefix(strings.ToLower(s), "fmout.") && !strings.HasPrefix(s, IdemSchemeV1+".") {
		   return IdemKeyParts{}, ErrOutboundIdemInvalid
	   }
	  if err := validateKeyCharset(s); err != nil {
		   return IdemKeyParts{}, err
	  }
	  if len(s) < idemMinLen || len(s) > idemMaxLen {
		   return IdemKeyParts{}, ErrOutboundIdemInvalid
	  }
	   if !strings.HasPrefix(s, IdemSchemeV1+".") {
		  return IdemKeyParts{}, ErrOutboundIdemInvalid
	  }
	  rest := strings.TrimPrefix(s, IdemSchemeV1+".")
	  kind, body, ok := splitKind(rest)
	   if !ok {
		  return IdemKeyParts{}, ErrOutboundIdemInvalid
	  }
	  if kind != IdemKindClient && kind != IdemKindDerived {
		   return IdemKeyParts{}, ErrOutboundIdemInvalid
	   }
	   if len(body) < idemBodyMin {
		  return IdemKeyParts{}, ErrOutboundIdemInvalid
	   }
	  return IdemKeyParts{
		   Raw:    s,
		   Scheme: IdemSchemeV1,
		  Kind:   kind,
		  Body:   body,
		   Derived: kind == IdemKindDerived,
		   Client:  kind == IdemKindClient,
		   Valid: true,
	  }, nil
   }

// CanonicalizeClientKey 把裸客户端键收成统一 scheme。

  func CanonicalizeClientKey(raw string) (string, error) {
	  s := strings.TrimSpace(raw)
	   if s == "" {
		  return "", ErrOutboundIdemRequired
	   }
	  if strings.HasPrefix(strings.ToLower(s), "fmout.") && !strings.HasPrefix(s, IdemSchemeV1+".") {
		  return "", ErrOutboundIdemInvalid
	  }
	   if strings.HasPrefix(s, IdemSchemeV1+".") {
		  return s, nil
	  }
	   if err := validateKeyCharset(s); err != nil {
		   return "", err
	  }
	   if len(s) < idemBodyMin || len(s) > idemMaxLen-len(IdemSchemeV1)-3 {
		  return "", ErrOutboundIdemInvalid
	  }
	  return IdemSchemeV1 + "." + IdemKindClient + "." + s, nil
  }

// DeriveOutboundKey 按用户/资产/金额/去向/链/备注派生。

   func DeriveOutboundKey(req OutboundRequest) (string, IdemKeyParts, error) {
	   canon, err := CanonicalPayload(req)
	  if err != nil {
		  return "", IdemKeyParts{}, err
	  }
	   sum := sha256.Sum256([]byte(canon))
	  body := hex.EncodeToString(sum[:])
	  if len(body) > 40 {
		   body = body[:40]
	  }
	   raw := IdemSchemeV1 + "." + IdemKindDerived + "." + body
	  parts := IdemKeyParts{
		   Raw:    raw,
		   Scheme: IdemSchemeV1, Kind: IdemKindDerived,
		  Body: body, Derived: true, Valid: true,
	  }
	   return raw, parts, nil
  }

   func validateKeyCharset(s string) error {
	   if s != strings.TrimSpace(s) {
		   return ErrOutboundIdemInvalid
	   }
	   for _, r := range s {
		   if r <= 0x1f || r == 0x7f {
			   return ErrOutboundIdemInvalid
		  }
		   if unicode.IsSpace(r) {
			   return ErrOutboundIdemInvalid
		  }
		   if unicode.IsLetter(r) || unicode.IsDigit(r) {
			   continue
		  }
		   switch r {
		  case '.', '_', '-', ':', '+':
			   continue
		   default:
			  return ErrOutboundIdemInvalid
		  }
	  }
	   return nil
   }

   func splitKind(rest string) (kind, body string, ok bool) {
	   i := strings.IndexByte(rest, '.')
	   if i <= 0 || i >= len(rest)-1 {
		  return "", "", false
	   }
	   return rest[:i], rest[i+1:], true
   }

  func firstNonEmpty(vals ...string) string {
	   for _, v := range vals {
		  if strings.TrimSpace(v) != "" {
			   return v
		  }
	  }
	  return ""
  }

// SameIdemKey 比较规范化后的键。

  func SameIdemKey(a, b string) bool {
	   pa, ea := ParseOutboundKey(a)
	   pb, eb := ParseOutboundKey(b)
	  if ea != nil || eb != nil {
		  return strings.TrimSpace(a) != "" && strings.TrimSpace(a) == strings.TrimSpace(b)
	  }
	   return pa.Raw == pb.Raw
   }

// KeyScope 键作用域：按用户隔离客户端键，派生键自带意图。

   func KeyScope(userID, key string) string {
	   u := strings.TrimSpace(userID)
	  k := strings.TrimSpace(key)
	   if u == "" {
		  return k
	   }
	   return u + "|" + k
  }
