package config

import (
	"gopkg.in/yaml.v3"
	"os"
	"strings"
)

type accountAssetsOverlay struct {
	Hosts map[string]string `yaml:"hosts"`
	Mesh  MeshServices      `yaml:"mesh"`
}

// mergeAccountAssetsCatalog 静默忽略缺失文件，便于单测与最小配置剖面。
func mergeAccountAssetsCatalog(cfg *Config) {
	if cfg == nil {
		return
	}
	path := FindRepoFile("configs/account-assets.yaml")
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var overlay accountAssetsOverlay
	if err := yaml.Unmarshal(b, &overlay); err != nil {
		return
	}
	if cfg.Hosts == nil {
		cfg.Hosts = map[string]string{}
	}
	for k, v := range overlay.Hosts {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if strings.TrimSpace(cfg.Hosts[k]) == "" {
			cfg.Hosts[k] = v
		}
	}
	if strings.TrimSpace(overlay.Mesh.Ledger) != "" && cfg.Mesh.Ledger == "" {
		cfg.Mesh.Ledger = overlay.Mesh.Ledger
	}
	if strings.TrimSpace(overlay.Mesh.DepositWithdraw) != "" && cfg.Mesh.DepositWithdraw == "" {
		cfg.Mesh.DepositWithdraw = overlay.Mesh.DepositWithdraw
	}
	if strings.TrimSpace(overlay.Mesh.Address) != "" && cfg.Mesh.Address == "" {
		cfg.Mesh.Address = overlay.Mesh.Address
	}
	if strings.TrimSpace(overlay.Mesh.AccountBook) != "" && cfg.Mesh.AccountBook == "" {
		cfg.Mesh.AccountBook = overlay.Mesh.AccountBook
	}
	if strings.TrimSpace(overlay.Mesh.Custody) != "" && cfg.Mesh.Custody == "" {
		cfg.Mesh.Custody = overlay.Mesh.Custody
	}
	if strings.TrimSpace(overlay.Mesh.KYT) != "" && cfg.Mesh.KYT == "" {
		cfg.Mesh.KYT = overlay.Mesh.KYT
	}
	if strings.TrimSpace(overlay.Mesh.Convert) != "" && cfg.Mesh.Convert == "" {
		cfg.Mesh.Convert = overlay.Mesh.Convert
	}
	if strings.TrimSpace(overlay.Mesh.StatementExport) != "" && cfg.Mesh.StatementExport == "" {
		cfg.Mesh.StatementExport = overlay.Mesh.StatementExport
	}
	if strings.TrimSpace(overlay.Mesh.FundingBFF) != "" && cfg.Mesh.FundingBFF == "" {
		cfg.Mesh.FundingBFF = overlay.Mesh.FundingBFF
	}
	if strings.TrimSpace(overlay.Mesh.P2PFunding) != "" && cfg.Mesh.P2PFunding == "" {
		cfg.Mesh.P2PFunding = overlay.Mesh.P2PFunding
	}
}
