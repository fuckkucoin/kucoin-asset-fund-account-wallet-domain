package fundread

import "account-assets.kucoin.net/funding/internal/fundmat"

// Query 一次用户视图读取。FenceSeq 为 0 且未指定流时，强一致要求每条流都追平自己的日志头。
type Query struct {
	UserID string
	Asset  string
	Subject string
	FenceID string
	FenceSeq int64
	FenceEpoch int64
	Stream     fundmat.Stream
}

// AssetTotal 同一资产跨科目的分栏合计。
type AssetTotal struct {
	Asset string `json:"asset"`
	Available string `json:"available"`
	Frozen    string `json:"frozen"`
	InFlight  string `json:"in_flight"`
	Occupancy string `json:"occupancy"`
}

// View 返回给用户或客服的聚合结果。
type View struct {
	UserID      string           `json:"user_id"`
	Consistency string           `json:"consistency"`
	Stale       bool             `json:"stale"`
	Rows        []fundmat.MatRow `json:"rows"`
	Assets     []AssetTotal           `json:"assets"`
	Watermarks []fundmat.MatWatermark `json:"watermarks"`
	Version    fundmat.ViewVersion    `json:"version"`
	Fence fundmat.ReadFence `json:"fence"`
}
