package p2pgate

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/validate"
)

var (
	ErrQueryRange = errors.New("p2p occupancy time range invalid")
	ErrQueryPage = errors.New("p2p occupancy page invalid")
	ErrHoldGone   = errors.New("p2p occupancy hold not found")
	ErrQueryAsset = errors.New("p2p occupancy asset invalid")
)

// ParseOccupancyQuery 从 URL 查询串构造 OccupancyQuery。
// 接受 asset/currency、reason/kind、ref/order_id/orderId、status、action、
// startAt/endAt（毫秒或 RFC3339）、page/currentPage、pageSize、sort、adjacent。

  func ParseOccupancyQuery(userID string, vals url.Values) (OccupancyQuery, error) {
	   q := OccupancyQuery{UserID: strings.TrimSpace(userID), Status: StatusAll, Page: DefaultPage, PageSize: DefaultPageSize}
	   if vals == nil {
		   return q, nil
	  }
	   asset := firstNonEmpty(vals, "asset", "currency")
	  if strings.TrimSpace(asset) != "" {
		   as, err := validate.Asset(asset)
		  if err != nil {
			   return OccupancyQuery{},
				   ErrQueryAsset
		   }
		  q.Asset = as
	  }
	  q.Reason = NormalizeReason(firstNonEmpty(vals, "reason", "kind"))
	   q.Ref = strings.TrimSpace(firstNonEmpty(vals, "ref", "order_id", "orderId"))
	   q.HoldID = strings.TrimSpace(firstNonEmpty(vals, "hold_id", "holdId", "id"))
	   q.Status = NormalizeStatus(firstNonEmpty(vals, "status", "state"))
	  q.Action = NormalizeAction(vals.Get("action"))
	  q.Adjacent = parseBool(firstNonEmpty(vals, "adjacent", "include_adjacent"))
	   q.SortAsc = strings.EqualFold(strings.TrimSpace(vals.Get("sort")), "asc")
	   start, err := ParseQueryTime(firstNonEmpty(vals, "startAt", "start_at", "start"))
	  if err != nil {
		  return OccupancyQuery{},
			  ErrQueryRange
	  }
	   end, err := ParseQueryTime(firstNonEmpty(vals, "endAt", "end_at", "end"))
	  if err != nil {
		  return OccupancyQuery{}, ErrQueryRange
	   }
	   if !start.IsZero() && !end.IsZero() && !end.After(start) {
		  return OccupancyQuery{},
			   ErrQueryRange
	  }
	   q.StartAt, q.EndAt = start, end
	   page, err := parsePositive(firstNonEmpty(vals, "page", "currentPage"), DefaultPage)
	   if err != nil {
		  return OccupancyQuery{},
			  ErrQueryPage
	  }
	  size, err := parsePositive(firstNonEmpty(vals, "pageSize", "page_size", "limit"), DefaultPageSize)
	   if err != nil {
		   return OccupancyQuery{}, ErrQueryPage
	  }
	  if size > MaxPageSize {
		  size = MaxPageSize
	   }
	  q.Page, q.PageSize = page, size
	   return q, nil
  }

// NormalizeReason 将 sell/merchant 别名落到 holdcat 原因；未知值原样小写返回以便过滤落空。
   func NormalizeReason(s string) string {
	  s = strings.ToLower(strings.TrimSpace(s))
	  switch s {
	  case "", "all", "*":
		  return ""
	  case "sell", "p2p_sell", "p2psell":
		  return holdcat.ReasonP2PSell
	  case "merchant", "margin", "p2p_merchant_margin", "p2p_margin":
		   return holdcat.ReasonP2PMargin
	   default:
		   return s
	   }
  }

// NormalizeStatus 仅接受 open / released / all。
   func NormalizeStatus(s string) string {
	  s = strings.ToLower(strings.TrimSpace(s))
	  switch s {
	  case StatusOpen, "true", "1":
		   return StatusOpen
	  case StatusReleased, "closed", "false", "0":
		  return StatusReleased
	   default:
		   return StatusAll
	   }
  }

// NormalizeAction 流水动作。空表示不按动作过滤。
   func NormalizeAction(s string) string {
	  s = strings.ToLower(strings.TrimSpace(s))
	   switch s {
	  case ActionBuy, "p2p_buy", "credit_buy":
		  return ActionBuy
	   case ActionSellHold, "sell", "p2p_sell":
		  return ActionSellHold
	   case ActionMerchantHold, "merchant", "margin", "p2p_margin":
		   return ActionMerchantHold
	  case ActionRelease, "rel", "p2p_rel":
		   return ActionRelease
	  case ActionAdjacent, "other":
		   return ActionAdjacent
	   default:
		   return ""
	  }
   }

  func firstNonEmpty(vals url.Values, keys ...string) string {
	  for _, k := range keys {
		   if v := strings.TrimSpace(vals.Get(k)); v != "" {
			   return v
		  }
	  }
	  return ""
  }

   func parsePositive(raw string, fallback int) (int, error) {
	  raw = strings.TrimSpace(raw)
	   if raw == "" {
		   return fallback, nil
	  }
	   n, err := strconv.Atoi(raw)
	   if err != nil || n <= 0 {
		   return 0, ErrQueryPage
	   }
	   return n, nil
   }

  func parseBool(s string) bool {
	   s = strings.ToLower(strings.TrimSpace(s))
	   return s == "1" || s == "true" || s == "yes"
  }

  func (q *OccupancyQuery) prepare() {
	   if q == nil {
		   return
	   }
	   q.UserID = strings.TrimSpace(q.UserID)
	   q.Asset = strings.ToUpper(strings.TrimSpace(q.Asset))
	   q.Reason = NormalizeReason(q.Reason)
	  q.Ref = strings.TrimSpace(q.Ref)
	  q.HoldID = strings.TrimSpace(q.HoldID)
	  q.Status = NormalizeStatus(q.Status)
	   if q.Action != "" {
		  q.Action = NormalizeAction(q.Action)
	   }
	  q.Page,
		   q.PageSize = normalizePage(q.Page, q.PageSize)
  }

// MatchHold 判定占用行是否命中查询（不含分页）。
   func (q OccupancyQuery) MatchHold(v HoldView) bool {
	   if q.Asset != "" && !strings.EqualFold(v.Asset, q.Asset) {
		   return false
	   }
	  if q.Reason != "" && v.Reason != q.Reason {
		   return false
	  }
	  if q.Ref != "" && v.Ref != q.Ref {
		   return false
	   }
	  if q.HoldID != "" && v.ID != q.HoldID {
		   return false
	   }
	   switch q.Status {
	   case StatusOpen:
		   if !v.Open {
			  return false
		  }
	   case StatusReleased:
		   if v.Open {
			   return false
		   }
	   }
	  if !q.StartAt.IsZero() || !q.EndAt.IsZero() {
		  ts := parseRFC3339(v.CreatedAt)
		  if !inRange(ts, q.StartAt, q.EndAt) {
			   return false
		   }
	  }
	   return true
  }

// MatchFlow 判定流水行是否命中查询。
  func (q OccupancyQuery) MatchFlow(e FlowEntry) bool {
	   if !q.Adjacent && e.Adjacent {
		  return false
	   }
	  if q.Asset != "" && !strings.EqualFold(e.Asset, q.Asset) {
		  return false
	   }
	  if q.Action != "" && e.Action != q.Action {
		  return false
	  }
	   if q.Ref != "" && e.OrderID != q.Ref && e.HoldID != q.Ref {
		   return false
	   }
	  if q.HoldID != "" && e.HoldID != q.HoldID {
		  return false
	   }
	   if q.Reason == holdcat.ReasonP2PSell && e.Action != ActionSellHold && e.Action != ActionRelease && e.Action != ActionBuy {
		  // 卖出占用语境下仍允许买入入账与释放回显，排除商家保证金流水。
		  if e.Action == ActionMerchantHold {
			   return false
		  }
	  }
	   if q.Reason == holdcat.ReasonP2PMargin && e.Action != ActionMerchantHold && e.Action != ActionRelease {
		  return false
	   }
	  if !q.StartAt.IsZero() || !q.EndAt.IsZero() {
		   ts := time.UnixMilli(e.CreatedAtMs).UTC()
		  if e.CreatedAtMs == 0 {
			   ts = parseRFC3339(e.CreatedAt)
		   }
		   if !inRange(ts, q.StartAt, q.EndAt) {
			  return false
		   }
	   }
	  return true
  }

   func inRange(ts, start, end time.Time) bool {
	   if ts.IsZero() {
		  return start.IsZero() && end.IsZero()
	  }
	  if !start.IsZero() && ts.Before(start) {
		  return false
	   }
	  if !end.IsZero() && !ts.Before(end) && !ts.Equal(end) {
		  // 闭区间右端：等于 end 仍计入。
		   if ts.After(end) {
			   return false
		  }
	  }
	  return true
   }

  func parseRFC3339(s string) time.Time {
	   s = strings.TrimSpace(s)
	   if s == "" {
		  return time.Time{}
	  }
	  if t, err := time.Parse(time.RFC3339, s); err == nil {
		  return t.UTC()
	  }
	   if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		  return t.UTC()
	   }
	   return time.Time{}
  }
