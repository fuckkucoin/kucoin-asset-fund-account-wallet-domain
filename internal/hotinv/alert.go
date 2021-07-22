package hotinv

import (
	"context"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/pkg/money"
)

type AlertKind string

const (
	AlertUnread AlertKind = "unread"
	AlertUnparsable AlertKind = "unparsable"
	AlertOverCap    AlertKind = "over_cap"
)

type Alert struct {
	Asset string `json:"asset"`
	Kind AlertKind `json:"kind"`
	Note string    `json:"note,omitempty"`
}

var ErrAlert = fmt.Errorf("hot inventory alert")

// Alerts 读库存。Observe 失败直接返回错误，不得空列表当安全。
  func (b *Book) Alerts(ctx context.Context, st ledger.Store) ([]Alert, error) {
	  if b == nil || st == nil {
		   return nil, fmt.Errorf("hot inventory unavailable")
	  }
	   rows, err := b.Observe(ctx, st)
	  if err != nil {
		  return nil, err
	  }
	   out := make([]Alert, 0)
	  for _, r := range rows {
		  if _, err := money.ParseBook(orZero(r.HotAvail)); err != nil {
			  out = append(out,
				  Alert{Asset: r.Asset, Kind: AlertUnparsable, Note: err.Error()})
			  continue
		   }
		  if r.Over {
			  out = append(out, Alert{Asset: r.Asset, Kind: AlertOverCap, Note: r.Excess})
		  }
	  }
	   return out, nil
   }

// RequireReadable 库存读失败或金额不可解析失败即关。超 cap 留给归集，不在此挡读。
  func (b *Book) RequireReadable(ctx context.Context, st ledger.Store) error {
	  alerts, err := b.Alerts(ctx, st)
	  if err != nil {
		  return err
	  }
	   for _, a := range alerts {
		  if a.Kind == AlertUnparsable || a.Kind == AlertUnread {
			  return fmt.Errorf("%w: %s %s", ErrAlert, a.Asset, a.Kind)
		   }
	   }
	   return nil
  }

// RequireAsset 出金前：该资产库存必须可读且未超 cap。空资产失败即关。

   func (b *Book) RequireAsset(ctx context.Context, st ledger.Store, asset string) error {
	  if b == nil {
		   return fmt.Errorf("hot inventory unavailable")
	   }
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	   if asset == "" {
		   return fmt.Errorf("%w: asset required", ErrAlert)
	   }
	   if err := b.RequireReadable(ctx, st); err != nil {
		  return err
	   }
	  return b.Check(ctx, st, asset)
  }

// RequireAnomalyFree 对账熔断用：不可读、不可解析或超 HotMax 均失败即关。
// 超 cap 在出金单笔路径可由归集消化；全局 recon 视超 cap 为热钱包异常，须 Engage 停充停提。
   func (b *Book) RequireAnomalyFree(ctx context.Context, st ledger.Store) error {
	  alerts, err := b.Alerts(ctx, st)
	   if err != nil {
		   return err
	  }
	  for _, a := range alerts {
		   switch a.Kind {
		  case AlertUnparsable, AlertUnread, AlertOverCap:
			   return fmt.Errorf("%w: %s %s", ErrAlert, a.Asset, a.Kind)
		   }
	  }
	   return nil
   }
