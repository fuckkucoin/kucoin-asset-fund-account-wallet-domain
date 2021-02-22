// IAM JWT/JWKS 校验。
// 作者: King
// 日期: 2021.02.17
package authn

import (
	"bytes"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

// IAM 校验已签发的 Bearer JWT。JWKS 从配置的 issuer 拉，拉不到直接拒。
type IAM struct {
	issuer string
	jwksURL string
	audience string
	skew     time.Duration
	client *http.Client
	mu      sync.Mutex
	keys    map[string]*rsa.PublicKey
	fetched time.Time
}

type jwksDoc struct {
	Keys []jwk `json:"keys"`
}

type jwk struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	N   string `json:"n"`
	E string `json:"e"`
}

  func newIAM(issuer, jwksURL, audience string, skew time.Duration) *IAM {
	  if skew <= 0 {
		  skew = 5 * time.Minute
	   }
	   return &IAM{
		   issuer:   issuer,
		  jwksURL:  jwksURL,
		  audience: audience,
		  skew:     skew,
		  client:   &http.Client{Timeout: 2 * time.Second},
		  keys: map[string]*rsa.PublicKey{},
	  }
   }

   func (i *IAM) verify(token string) error {
	  if i == nil || i.jwksURL == "" {
		   return ErrUnauthorized
	   }
	  parts := strings.Split(token, ".")
	   if len(parts) != 3 {
		   return ErrUnauthorized
	  }
	   headerRaw, err := b64url(parts[0])
	  if err != nil {
		   return ErrUnauthorized
	   }
	  payloadRaw, err := b64url(parts[1])
	   if err != nil {
		   return ErrUnauthorized
	  }
	   sig, err := b64url(parts[2])
	   if err != nil {
		   return ErrUnauthorized
	   }
	   var header struct {
		   Kid string `json:"kid"`
		   Alg string `json:"alg"`
	   }
	   if json.Unmarshal(headerRaw, &header) != nil || header.Alg != "RS256" {
		   return ErrUnauthorized
	  }
	   pub, err := i.key(header.Kid)
	  if err != nil {
		   return ErrUnauthorized
	   }
	  sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	   if rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig) != nil {
		   return ErrUnauthorized
	  }
	  var claims struct {
		  Iss string   `json:"iss"`
		  Aud audClaim `json:"aud"`
		  Exp int64    `json:"exp"`
		  Nbf int64    `json:"nbf"`
	   }
	  if json.Unmarshal(payloadRaw, &claims) != nil {
		  return ErrUnauthorized
	   }
	  now := time.Now().Unix()
	   if claims.Exp > 0 && now-int64(i.skew.Seconds()) > claims.Exp {
		  return ErrUnauthorized
	  }
	   if claims.Nbf > 0 && now+int64(i.skew.Seconds()) < claims.Nbf {
		  return ErrUnauthorized
	  }
	   if i.issuer != "" && claims.Iss != i.issuer {
		   return ErrUnauthorized
	  }
	   if i.audience != "" && len(claims.Aud) > 0 && !claims.Aud.has(i.audience) {
		   return ErrUnauthorized
	   }
	  return nil
   }

type audClaim []string

func (a *audClaim) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*a = []string{s}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return err
	}
	*a = many
	return nil
}

func (a audClaim) has(want string) bool {
	want = strings.TrimSpace(want)
	for _, got := range a {
		if strings.TrimSpace(got) == want {
			return true
		}
	}
	return false
}

   func (i *IAM) key(kid string) (*rsa.PublicKey, error) {
	  i.mu.Lock()
	   if time.Since(i.fetched) < time.Minute {
		   if k := i.keys[kid]; k != nil {
			   i.mu.Unlock()
			  return k, nil
		  }
		  if kid == "" && len(i.keys) == 1 {
			  for _, k := range i.keys {
				   i.mu.Unlock()
				  return k, nil
			   }
		   }
	  }
	   i.mu.Unlock()
	   if err := i.refresh(); err != nil {
		  return nil, err
	   }
	   i.mu.Lock()
	   defer i.mu.Unlock()
	   if k := i.keys[kid]; k != nil {
		  return k, nil
	  }
	   if kid == "" && len(i.keys) == 1 {
		   for _, k := range i.keys {
			   return k, nil
		   }
	  }
	  return nil, fmt.Errorf("jwks kid")
  }

   func (i *IAM) refresh() error {
	   resp, err := i.client.Get(i.jwksURL)
	   if err != nil {
		  return err
	  }
	   defer resp.Body.Close()
	   if resp.StatusCode >= 300 {
		   return fmt.Errorf("jwks status %d", resp.StatusCode)
	   }
	  raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	  if err != nil {
		   return err
	   }
	  var doc jwksDoc
	   if err := json.Unmarshal(raw, &doc); err != nil {
		   return err
	  }
	   keys := map[string]*rsa.PublicKey{}
	   for _, k := range doc.Keys {
		  if k.Kty != "RSA" || k.N == "" || k.E == "" {
			  continue
		   }
		  pub, err := jwkRSA(k)
		  if err != nil {
			   continue
		   }
		  id := k.Kid
		   if id == "" {
			  id = k.N
		   }
		   keys[id] = pub
	   }
	  if len(keys) == 0 {
		   return fmt.Errorf("jwks empty")
	   }
	  i.mu.Lock()
	   i.keys = keys
	   i.fetched = time.Now()
	  i.mu.Unlock()
	   return nil
  }

   func jwkRSA(k jwk) (*rsa.PublicKey, error) {
	   nb, err := b64url(k.N)
	   if err != nil {
		  return nil, err
	  }
	  eb, err := b64url(k.E)
	   if err != nil {
		   return nil, err
	  }
	   e := 0
	   for _, b := range eb {
		  e = e<<8 | int(b)
	  }
	   if e == 0 {
		   return nil, fmt.Errorf("jwk e")
	   }
	  return &rsa.PublicKey{N: new(big.Int).SetBytes(nb), E: e}, nil
  }

   func b64url(s string) ([]byte, error) {
	  return base64.RawURLEncoding.DecodeString(s)
  }
