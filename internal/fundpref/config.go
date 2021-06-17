package fundpref

import (
	"context"
)

// ConfigOf 组装公开账户配置快照。
   func ConfigOf(p Preference) AccountConfig {
	  book, err := ParseDefaultAccount(p.DefaultAccount)
	  if err != nil {
		   book = AccountFunding
	  }
	  order := append([]string(nil), p.AssetOrder...)
	   if order == nil {
		   order = []string{}
	  }
	   return AccountConfig{
		  Currency: p.DisplayCurrency,
		   DefaultAccountType: KCAccountType(book),
		   HideSmallBalance: p.HideZero,
		  AssetOrder: order,
		  AccountTypes: []string{"MAIN", "TRADE", "MARGIN", "ISOLATED", "CONTRACT", "EARN"},
	  }
   }

// PreferenceOf 组装公开偏好体。
   func PreferenceOf(p Preference) AccountPreference {
	   book, err := ParseDefaultAccount(p.DefaultAccount)
	   if err != nil {
		   book = AccountFunding
	  }
	  order := append([]string(nil), p.AssetOrder...)
	  if order == nil {
		  order = []string{}
	   }
	  return AccountPreference{
		  Currency:       p.DisplayCurrency,
		  DefaultAccount: KCAccountType(book),
		   HideSmallBalance: p.HideZero,
		  AssetOrder:       order,
	  }
  }

// SnapshotConfig 读并映射公开配置。损坏失败即关。

  func (b *Book) SnapshotConfig(ctx context.Context, userID string) (AccountConfig, error) {
	  p, err := b.Get(ctx, userID)
	  if err != nil {
		  return AccountConfig{}, err
	   }
	  return ConfigOf(p), nil
   }

// SnapshotPreference 读并映射公开偏好。

  func (b *Book) SnapshotPreference(ctx context.Context, userID string) (AccountPreference, error) {
	  p, err := b.Get(ctx, userID)
	  if err != nil {
		   return AccountPreference{}, err
	  }
	   return PreferenceOf(p), nil
  }
