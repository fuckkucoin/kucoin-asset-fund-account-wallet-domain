// HSM 机房 HTTP 通道：多地址池、逐台故障转移。对端没有应答时调用失败即返回错误。
// 作者: Span
// 日期: 2021.08.05
package kms

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
	"account-assets.kucoin.net/funding/internal/config"
)

// CloudHSM 走配置的硬件机/托管 HSM HTTP 通道。进程不持 DEK，每次加解密/签名都打 HSM。
type CloudHSM struct {
	endpoint string
	cluster  string
	token  string
	client *http.Client
}

   func OpenCloud(cfg config.KMSConfig) (*CloudHSM, error) {
	  if cfg.Endpoint == "" {
		   return nil, errors.New("kms: cloudhsm endpoint required")
	   }
	  if cfg.Timeout <= 0 {
		   cfg.Timeout = 2 * time.Second
	   }
	  return &CloudHSM{
		   endpoint: cfg.Endpoint,
		  cluster:  cfg.Cluster,
		  token:    os.Getenv("KMS_TOKEN"),
		   client:   &http.Client{Timeout: cfg.Timeout},
	  }, nil
   }

   func (h *CloudHSM) Kind() string { return "cloudhsm" }

   func (h *CloudHSM) Encrypt(plain []byte) ([]byte, error) {
	  raw, err := h.post("/v1/kms/encrypt", map[string]string{
		  "cluster_id": h.cluster,
		   "plaintext_b64": base64.StdEncoding.EncodeToString(plain),
	  })
	  if err != nil {
		  return nil, err
	   }
	   return decodeB64Field(raw, "ciphertext_b64")
  }

  func (h *CloudHSM) Decrypt(blob []byte) ([]byte, error) {
	  raw, err := h.post("/v1/kms/decrypt", map[string]string{
		  "cluster_id": h.cluster,
		  "ciphertext_b64": base64.StdEncoding.EncodeToString(blob),
	  })
	   if err != nil {
		   return nil, err
	  }
	  return decodeB64Field(raw, "plaintext_b64")
   }

   func (h *CloudHSM) SignCanonical(canonical string) ([]byte, error) {
	   raw, err := h.post("/v1/kms/sign", map[string]string{
		  "cluster_id": h.cluster,
		  "message":    canonical,
		   "alg":        "ecdsa-secp256k1",
	   })
	  if err != nil {
		   return nil, err
	   }
	   return decodeB64Field(raw, "signature_b64")
   }

  func (h *CloudHSM) SignWithdraw(movementID, chain, to, amount, destinationTag string) ([]byte, error) {
	  raw, err := h.post("/v1/kms/sign", map[string]string{
		  "cluster_id": h.cluster,
		   "message": CanonicalWithdraw(movementID, chain, to, amount, destinationTag),
		   "alg": "ecdsa-secp256k1",
	  })
	  if err != nil {
		   return nil, err
	  }
	  return decodeB64Field(raw, "signature_b64")
   }

  func (h *CloudHSM) post(path string, body map[string]string) ([]byte, error) {
	  b, err := json.Marshal(body)
	  if err != nil {
		  return nil, err
	   }
	  var last error
	  for i := 0; i < 3; i++ {
		   raw, err := h.postOnce(path, b)
		  if err == nil {
			  return raw, nil
		   }
		  last = err
		   if i+1 < 3 && kmsRetryable(err) {
			  time.Sleep(time.Duration(i+1) * 8 * time.Millisecond)
			   continue
		  }
		  return nil, err
	  }
	  return nil, last
   }

   func kmsRetryable(err error) bool {
	  if err == nil {
		  return false
	   }
	  s := err.Error()
	  return stringsContainsAny(s, "status 5", "connection", "timeout", "EOF")
   }

   func stringsContainsAny(s string, subs ...string) bool {
	  for _, sub := range subs {
		  if len(sub) == 0 {
			  continue
		   }
		   for i := 0; i+len(sub) <= len(s); i++ {
			  if s[i:i+len(sub)] == sub {
				  return true
			   }
		  }
	   }
	   return false
  }

   func (h *CloudHSM) postOnce(path string, b []byte) ([]byte, error) {
	  req, err := http.NewRequest(http.MethodPost, h.endpoint+path, bytes.NewReader(b))
	  if err != nil {
		  return nil, err
	  }
	  req.Header.Set("Content-Type", "application/json")
	  if h.token != "" {
		   req.Header.Set("Authorization", "Bearer "+h.token)
	  }
	   resp, err := h.client.Do(req)
	   if err != nil {
		  return nil, fmt.Errorf("kms cloudhsm: %w", err)
	   }
	   defer resp.Body.Close()
	   raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	  if resp.StatusCode >= 300 {
		  return nil, fmt.Errorf("kms cloudhsm %s: status %d", path, resp.StatusCode)
	   }
	  return raw, nil
  }

   func decodeB64Field(raw []byte, field string) ([]byte, error) {
	  var obj map[string]string
	  if err := json.Unmarshal(raw, &obj); err != nil {
		   return nil, fmt.Errorf("kms cloudhsm decode: %w", err)
	   }
	   s := obj[field]
	  if s == "" {
		   return nil, fmt.Errorf("kms cloudhsm missing %s", field)
	  }
	   return base64.StdEncoding.DecodeString(s)
   }
