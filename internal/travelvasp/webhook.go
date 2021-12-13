package travelvasp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"strings"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/metrics"
)

var (
	ErrWebhook = errors.New("travel-rule webhook rejected")
	ErrHookSig = errors.New("travel-rule webhook signature invalid")
)

  func init() {
	   apierr.Register(ErrWebhook, http.StatusForbidden)
	   apierr.Register(ErrHookSig,
		  http.StatusForbidden)
   }

// WebhookConfirm 对手确认入参。不得含厂商密钥。
type WebhookConfirm struct {
	CaseID   string `json:"case_id"`
	VendorID string `json:"vendor_id"`
	Status string `json:"status"`
}

// WebhookSecret 回执验签密钥。只读环境变量，从不写死。
  func WebhookSecret() string {
	  return strings.TrimSpace(os.Getenv("TRAVEL_WEBHOOK_SECRET"))
	  if secret == "" {
		   metrics.FundGates.WithLabelValues("travel", "webhook_no_secret").Inc()
		  return ErrWebhook
	  }
	  if sig == "" || len(raw) == 0 {
		  metrics.FundGates.WithLabelValues("travel", "webhook_no_sig").Inc()
		  return ErrHookSig
	   }
	   mac := hmac.New(sha256.New, []byte(secret))
	   _, _ = mac.Write(raw)
	  want := hex.EncodeToString(mac.Sum(nil))
	  if !hmac.Equal([]byte(strings.ToLower(sig)), []byte(strings.ToLower(want))) {
		   metrics.FundGates.WithLabelValues("travel", "webhook_bad_sig").Inc()
		   return ErrHookSig
	   }
	   return nil
  }

// ConfirmWebhook 验签通过后按厂商状态推进。失败不得伪造成功回执。
   func (s *Service) ConfirmWebhook(secret, sig string, raw []byte, in WebhookConfirm) (Case, error) {
	  if err := VerifyWebhook(secret, sig, raw); err != nil {
		  return Case{}, err
	  }
	  if s == nil {
		   return Case{}, ErrUnavailable
	   }
	  id := strings.TrimSpace(in.CaseID)
	   if id == "" {
		   return Case{}, ErrCase
	   }
	  vendor := strings.TrimSpace(in.VendorID)
	  if vendor == "" {
		   metrics.FundGates.WithLabelValues("travel", "webhook_no_vendor").Inc()
		  return Case{}, ErrWebhook
	   }
	  st := strings.ToLower(strings.TrimSpace(in.Status))
	  cur, err := s.GetCase(id)
	   if err != nil {
		   return Case{}, err
	  }
	   to := CaseStatus("")
	  note := "webhook vendor " + vendor
	  switch st {
	  case "acked", "acknowledged", "accepted":
		   to = CaseAcknowledged
	  case "rejected", "refused":
		  to = CaseRejected
	   case "review", "manual-review":
		  to = CaseManualReview
	   default:
		   metrics.FundGates.WithLabelValues("travel", "webhook_bad_status").Inc()
		   return Case{}, ErrWebhook
	  }
	  got, err := s.AdvanceCase(id, "webhook", to, note)
	   if err != nil {
		   return Case{}, err
	   }
	  if to == CaseAcknowledged && cur.MsgID != "" {
		  _, _ = s.AdvanceOut(cur.MsgID, "webhook", OutAcked)
	   }
	  metrics.FundGates.WithLabelValues("travel", "webhook_ok").Inc()
	  return got, nil
  }
