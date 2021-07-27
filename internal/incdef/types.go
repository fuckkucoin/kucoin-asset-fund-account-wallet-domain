package incdef

import "time"

type Mode string

const (
	ModeNormal   Mode = "normal"
	ModeDegraded Mode = "degraded"
	ModeFrozen Mode = "frozen"
	ModeKilled Mode = "killed"
)

type Alert struct {
	ID string `json:"id"`
	Level string `json:"level"`
	Kind    string `json:"kind"`
	Subject string `json:"subject,omitempty"`
	Body    string `json:"body"`
	At time.Time `json:"at"`
}

type Event struct {
	Seq int64 `json:"seq"`
	Kind    string    `json:"kind"`
	Actor   string    `json:"actor"`
	Subject string    `json:"subject,omitempty"`
	Body    string    `json:"body"`
	At      time.Time `json:"at"`
}

type Timeline struct {
	Host string `json:"host"`
	Mode Mode `json:"mode"`
	Events []Event `json:"events"`
}

type StuckRec struct {
	TransferID string `json:"transfer_id"`
	Action     string `json:"action"`
	Reason string `json:"reason"`
}

type CircuitSnap struct {
	Name string `json:"name"`
	State string `json:"state"`
	Fails int    `json:"fails"`
}

type Status struct {
	Mode     Mode          `json:"mode"`
	Killed   bool          `json:"killed"`
	Degraded bool          `json:"degraded"`
	Frozen   bool          `json:"frozen"`
	Circuits []CircuitSnap `json:"circuits,omitempty"`
	Alerts  int    `json:"alerts"`
	Host    string `json:"host"`
	Runbook string `json:"runbook"`
}
