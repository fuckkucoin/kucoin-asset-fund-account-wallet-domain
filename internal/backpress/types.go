package backpress

type Job struct {
	ID string `json:"id"`
	Kind    string `json:"kind"`
	Payload string `json:"payload"`
}

type Batch struct {
	ID        string `json:"id"`
	Requested int    `json:"requested"`
	Accepted int `json:"accepted"`
	Dropped int `json:"dropped"`
	Failed int `json:"failed"`
}

type Stats struct {
	Queued int `json:"queued"`
	Cap    int `json:"cap"`
	Inflight int `json:"inflight"`
	Workers int   `json:"workers"`
	Dropped int64 `json:"dropped"`
	Done    int64 `json:"done"`
	Host string `json:"host"`
}
