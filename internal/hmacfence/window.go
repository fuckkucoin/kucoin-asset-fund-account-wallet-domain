package hmacfence

import (
	"strconv"
	"strings"
	"time"
)

type KeyRing struct {
	keys map[string]string
}

  func NewKeyRing(keys map[string]string) *KeyRing {
	  out := map[string]string{}
	  for k, v := range keys {
		  k = strings.TrimSpace(k)
		   if k == "" || strings.TrimSpace(v) == "" {
			  continue
		   }
		  out[k] = v
	   }
	   return &KeyRing{keys: out}
   }

  func (r *KeyRing) Secret(id string) (string, error) {
	  if r == nil {
		  return "", ErrKey
	  }
	  sec, ok := r.keys[strings.TrimSpace(id)]
	  if !ok || sec == "" {
		  return "", ErrKey
	  }
	   return sec, nil
  }

// VerifyRing 用 key 环验签。未知 key 失败即关。

   func (s *Service) VerifyRing(ring *KeyRing, keyID, ts, nonce, method, path, sig string, body []byte) error {
	   if s == nil {
		  return ErrUnavailable
	   }
	   sec, err := ring.Secret(keyID)
	   if err != nil {
		   return err
	   }
	  return s.Verify(keyID, sec, ts, nonce, method, path, sig, body)
   }

// Purge 丢掉超过 2*skew 的 nonce，避免 seen 无限涨。
   func (s *Service) Purge() int {
	   if s == nil {
		  return 0
	   }
	   now := s.clock().Unix()
	   cut := now - int64((2*s.skew)/time.Second)
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   n := 0
	   for k, ts := range s.seen {
		  if ts < cut {
			  delete(s.seen, k)
			   n++
		  }
	   }
	  return n
   }

type ReplayCase struct {
	KeyID  string
	Secret string
	TS     string
	Nonce  string
	Method string
	Path string
	Sig  string
	Body []byte
	Want error
}

// ReplayTable 按序验一组请求。重复 nonce 必须 ErrReplay。
   func (s *Service) ReplayTable(rows []ReplayCase) []error {
	   out := make([]error, 0, len(rows))
	  for _, r := range rows {
		   err := s.Verify(r.KeyID, r.Secret, r.TS, r.Nonce, r.Method, r.Path, r.Sig, r.Body)
		  out = append(out, err)
	   }
	   return out
   }

  func NowTS(t time.Time) string {
	   return strconv.FormatInt(t.Unix(), 10)
   }

   func (s *Service) Skew() time.Duration {
	  if s == nil {
		   return 0
	  }
	  return s.skew
   }
