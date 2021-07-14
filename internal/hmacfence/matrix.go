package hmacfence

import (
	"fmt"
	"strconv"
	"time"
)

type MatrixExpect struct {
	Name string
	Case ReplayCase
}

  func (s *Service) FixedClock(unix int64) {
	  if s == nil {
		  return
	   }
	  s.clock = func() time.Time { return time.Unix(unix, 0).UTC() }
  }

   func SignCase(keyID, secret, ts, nonce, method, path string, body []byte) ReplayCase {
	   return ReplayCase{
		  KeyID: keyID, Secret: secret, TS: ts, Nonce: nonce,
		  Method: method, Path: path, Body: body,
		   Sig: Sign(secret, ts, nonce, method, path, body),
	  }
  }

// ReplayMatrix 覆盖未知 key、短 nonce、偏斜、坏签、同 key 重放、异 key 同 nonce、停机。
  func ReplayMatrix(now int64) []MatrixExpect {
	  ts := strconv.FormatInt(now, 10)
	   body := []byte(`{"n":1}`)
	  ok := SignCase("k1", "sec-a", ts, "nonce-ok-1", "POST", "/v1/wd", body)
	   replay := ok
	   replay.Want = ErrReplay
	   otherKey := SignCase("k2", "sec-b", ts, "nonce-ok-1", "POST", "/v1/wd", body)
	  short := SignCase("k1", "sec-a", ts, "short", "POST", "/v1/wd", body)
	   short.Want = ErrNonce
	  skew := SignCase("k1", "sec-a", "1", "nonce-skew1", "POST", "/v1/wd", body)
	  skew.Want = ErrSkew
	   bad := SignCase("k1", "sec-a", ts, "nonce-bad-1", "POST", "/v1/wd", body)
	  bad.Sig = "00"
	   bad.Want = ErrSign
	   emptyKey := SignCase("", "sec-a", ts, "nonce-emptyk", "POST", "/v1/wd", body)
	   emptyKey.Want = ErrKey
	  pathDrift := SignCase("k1", "sec-a", ts, "nonce-path-1", "POST", "/v1/wd", body)
	  pathDrift.Path = "/v1/other"
	   pathDrift.Want = ErrSign
	   return []MatrixExpect{
		   {Name: "first", Case: ok},
		  {Name: "replay", Case: replay},
		   {Name: "other-key-same-nonce", Case: otherKey},
		   {Name: "short-nonce", Case: short},
		  {Name: "skew", Case: skew},
		  {Name: "bad-sig", Case: bad},
		  {Name: "empty-key", Case: emptyKey},
		   {Name: "path-drift", Case: pathDrift},
	  }
  }

   func CheckReplayErr(got, want error) error {
	   if want == nil {
		  if got != nil {
			  return fmt.Errorf("want nil got %v", got)
		  }
		   return nil
	   }
	  if got != want {
		   return fmt.Errorf("want %v got %v", want, got)
	  }
	   return nil
   }
