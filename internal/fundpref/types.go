package fundpref

import "time"

// Preference 用户资金页偏好。落 KV fundpref.row 与 SQL aa_fund_pref。

type Preference struct {
	UserID          string `json:"user_id"`
	DefaultAccount  string `json:"default_account"`
	DisplayCurrency string `json:"display_currency"`
	HideZero bool `json:"hide_zero"`
	AssetOrder []string  `json:"asset_order"`
	UpdatedAt  time.Time `json:"updated_at"`
	Version int64 `json:"version"`
}

// WriteReq 写入口。同时认仓库蛇形与公开面驼峰，避免双面分叉。

type WriteReq struct {
	DefaultAccount string `json:"default_account"`
	DefaultAccount2 string `json:"defaultAccount"`
	DisplayCurrency string `json:"display_currency"`
	Currency string `json:"currency"`
	HideZero *bool  `json:"hide_zero"`
	HideSmall  *bool    `json:"hideSmallBalance"`
	AssetOrder []string `json:"asset_order"`
	AssetOrder2   []string `json:"assetOrder"`
	QuoteCurrency string   `json:"quoteCurrency"`
	HideSmallAlt  *bool    `json:"hideSmall"`
	CurrencyOrder []string `json:"currencyOrder"`
}

// AccountConfig 公开 KuCoin 风格账户配置快照。
type AccountConfig struct {
	Currency string `json:"currency"`
	DefaultAccountType string `json:"defaultAccountType"`
	HideSmallBalance   bool   `json:"hideSmallBalance"`
	AssetOrder []string `json:"assetOrder"`
	AccountTypes []string `json:"accountTypes"`
}

// AccountPreference 公开 KuCoin 风格偏好读/写体。

type AccountPreference struct {
	Currency       string `json:"currency"`
	DefaultAccount string `json:"defaultAccount"`
	HideSmallBalance bool     `json:"hideSmallBalance"`
	AssetOrder       []string `json:"assetOrder"`
}

// AuditRec 偏好变更审计。键空间 fundpref.audit，与 fundsurf 冻结键隔离。
type AuditRec struct {
	ID string `json:"id"`
	UserID string `json:"user_id"`
	Actor  string `json:"actor"`
	Action string `json:"action"`
	Before string `json:"before"`
	After  string `json:"after"`
	At time.Time `json:"at"`
}

// DefaultView 默认科目解析结果，供资金读面挂钩。

type DefaultView struct {
	UserID         string `json:"user_id"`
	DefaultAccount string `json:"default_account"`
	AccountType string `json:"accountType"`
	Book        string `json:"book"`
}
