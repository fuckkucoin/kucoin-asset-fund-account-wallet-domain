package policy

// Family 归因族。和链适配器的广播族不是同一套名字，冲突时以本表为准。
type Family string

const (
	FamMemo Family = "MEMO"
	FamUTXO Family = "UTXO"
	FamEVM  Family = "EVM"
	FamAcct Family = "ACCOUNT"
)

// Version 策略代次。Active 同时只允许每个族一个。

type Version struct {
	ID     string
	Family Family
	Active     bool
	Supersedes string
	CreditFloor int
	Dust string
	Note string
}

// Row 单链策略。尘埃、备注字符集、找零图是否必填都在这里，不写进观察线程。

type Row struct {
	Chain string
	Family  Family
	Version string
	Native  string
	Dust    string
	MemoMode string
	MemoRequired bool
	MemoMax      int
	MemoMin      int
	CaseFold     bool
	StripZW      bool
	LeadingZero bool
	Charset string
	Namespace   string
	Confirm     int
	CreditFloor int
	FeeMin        string
	FeeMaxBps     int
	GraphRequired bool
	DelaySec int
	RatePerMin int
	Decimals   int
	Scripts   []string
	Noise     []string
	Collision string
	ChangeCredit bool
}

// Eval 一次代次裁定。Closed 为真时调用方不得产生入账意图。

type Eval struct {
	Row Row
	Ver    Version
	Closed bool
	Reason string
	Dust    string
	Floor   int
	MemoMax int
	Native string
}

// AssetMin 某链某资产的入账下限。没有覆盖时用链尘埃。

type AssetMin struct {
	Chain string
	Asset string
	Dust string
	Decimals int
}
