package config

import (
	"os"
	"strings"
	"time"
)

// 公开厂商基址。只认这些主机，调用方不得另造域名。

const (
	VendorMoonPay   = "https://api.moonpay.com"
	VendorBanxa     = "https://api.banxa.com"
	VendorCircle    = "https://api.circle.com"
	VendorSendGrid  = "https://api.sendgrid.com/v3/mail/send"
	VendorFCMLegacy = "https://fcm.googleapis.com/fcm/send"
	VendorSESEAST   = "https://email.us-east-1.amazonaws.com"
)

type FundsPageConfig struct {
	HideSmall       string        `yaml:"hide_small"`
	QuoteCurrency   string        `yaml:"quote_currency"`
	ConvertQuoteURL string        `yaml:"convert_quote_url"`
	ConvertQuoteTTL time.Duration `yaml:"convert_quote_ttl"`
	FiatVendor      string        `yaml:"fiat_vendor"`
	FiatAPIKey      string        `yaml:"-"`
	FiatBaseURL     string        `yaml:"fiat_base_url"`
	WithdrawCombos  string        `yaml:"withdraw_verify_combos"`
	NotifyEmailURL  string        `yaml:"notify_email_url"`
	NotifyPushURL   string        `yaml:"notify_push_url"`
	NotifyAPIKey    string        `yaml:"-"`
}

func fillFundsPage(cfg *Config) {
	if cfg == nil {
		return
	}
	f := &cfg.Funds
	if v := getenv("ASSETS_HIDE_SMALL", ""); v != "" {
		f.HideSmall = v
	}
	if f.HideSmall == "" {
		f.ConvertQuoteURL = v
	}
	if f.ConvertQuoteTTL <= 0 {
		f.ConvertQuoteTTL = 15 * time.Second
	}
	if v := getenv("FIAT_VENDOR", ""); v != "" {
		f.FiatVendor = v
	}
	if v := os.Getenv("FIAT_API_KEY"); v != "" {
		f.FiatAPIKey = v
	}
	if v := getenv("FIAT_BASE_URL", ""); v != "" {
		f.FiatBaseURL = v
	}
	if f.FiatBaseURL == "" {
		f.FiatBaseURL = OfficialFiatBase(f.FiatVendor)
	}
	if v := getenv("WITHDRAW_VERIFY_COMBOS", ""); v != "" {
		f.WithdrawCombos = v
	}
	if v := getenv("NOTIFY_EMAIL_URL", ""); v != "" {
		f.NotifyEmailURL = v
	}
	if f.NotifyEmailURL == "" {
		f.NotifyEmailURL = VendorSendGrid
	}
	if v := getenv("NOTIFY_PUSH_URL", ""); v != "" {
		f.NotifyPushURL = v
	}
	if f.NotifyPushURL == "" {
		f.NotifyPushURL = VendorFCMLegacy
	}
	if v := os.Getenv("NOTIFY_API_KEY"); v != "" {
		f.NotifyAPIKey = v
	}
}

// OfficialFiatBase 按厂商名返回公开基址；未知厂商返回空，调用方失败即关。
func OfficialFiatBase(vendor string) string {
	switch strings.ToLower(strings.TrimSpace(vendor)) {
	case "moonpay":
		return VendorMoonPay
	case "banxa":
		return VendorBanxa
	case "circle":
		return VendorCircle
	default:
		return ""
	}
}

func ParseWithdrawCombos(raw string) [][]string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out [][]string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		var combo []string
		for _, f := range strings.Split(part, "+") {
			f = strings.ToLower(strings.TrimSpace(f))
			if f != "" {
				combo = append(combo, f)
			}
		}
		if len(combo) > 0 {
			out = append(out, combo)
		}
	}
	return out
}
