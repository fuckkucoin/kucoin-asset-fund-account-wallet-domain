package hasync

import "time"

type Probe struct {
	Region string `json:"region"`
	Label  string `json:"label"`
	CIDR   string `json:"cidr"`
	Ready  bool   `json:"ready"`
	Leader bool   `json:"leader"`
	Split  bool   `json:"split"`
	Active string `json:"active,omitempty"`
	At time.Time `json:"at"`
}

type CampaignRec struct {
	Region string `json:"region"`
	Leader bool   `json:"leader"`
	Epoch int64 `json:"fence_epoch"`
	Split bool  `json:"split"`
	Note string `json:"note,omitempty"`
}

type WriteReq struct {
	Actor string `json:"actor"`
	Kind  string `json:"kind"`
	Reason string `json:"reason,omitempty"`
}

type WriteRec struct {
	OK     bool   `json:"ok"`
	Reason string `json:"reason,omitempty"`
	Probe  Probe  `json:"probe"`
}
