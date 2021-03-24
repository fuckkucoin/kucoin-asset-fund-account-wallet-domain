package config

// MeshServices 逻辑角色 → etcd 注册名。
type MeshServices struct {
	Ledger          string `yaml:"ledger"`
	DepositWithdraw string `yaml:"deposit_withdraw"`
	Address         string `yaml:"address"`
	AccountBook     string `yaml:"account_book"`
	Custody         string `yaml:"custody"`
	KYT             string `yaml:"kyt"`
	Convert         string `yaml:"convert"`
	FundSurface     string `yaml:"fund_surface"`
	StatementExport string `yaml:"statement_export"`
	P2PFunding      string `yaml:"p2p_funding"`
	FundingBFF      string `yaml:"funding_bff"`
}

func defaultMesh() MeshServices {
	return MeshServices{
		Ledger:          "ledger-service",
		DepositWithdraw: "deposit-withdraw",
		Address:         "address-service",
		AccountBook:     "account-service",
		Custody:         "custody-service",
		KYT:             "kyt-service",
		Convert:         "convert",
		FundSurface:     "fund-surface",
		StatementExport: "stmt-export",
		P2PFunding:      "p2p-funding",
		FundingBFF:      "funding-bff-service",
	}
}

func fillMesh(cfg *Config) {
	if cfg == nil {
		return
	}
	d := defaultMesh()
	if cfg.Mesh.Ledger == "" {
		cfg.Mesh.Ledger = d.Ledger
	}
	if cfg.Mesh.DepositWithdraw == "" {
		cfg.Mesh.DepositWithdraw = d.DepositWithdraw
	}
	if cfg.Mesh.Address == "" {
		cfg.Mesh.Address = d.Address
	}
	if cfg.Mesh.AccountBook == "" {
		cfg.Mesh.AccountBook = d.AccountBook
	}
	if cfg.Mesh.Custody == "" {
		cfg.Mesh.Custody = d.Custody
	}
	if cfg.Mesh.KYT == "" {
		cfg.Mesh.KYT = d.KYT
	}
	if cfg.Mesh.Convert == "" {
		cfg.Mesh.Convert = d.Convert
	}
	if cfg.Mesh.FundSurface == "" {
		cfg.Mesh.FundSurface = d.FundSurface
	}
	if cfg.Mesh.StatementExport == "" {
		cfg.Mesh.StatementExport = d.StatementExport
	}
	if cfg.Mesh.P2PFunding == "" {
		cfg.Mesh.P2PFunding = d.P2PFunding
	}
	if cfg.Mesh.FundingBFF == "" {
		cfg.Mesh.FundingBFF = d.FundingBFF
	}
}
