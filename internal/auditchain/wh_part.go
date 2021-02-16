package auditchain

import (
	"strings"
	"time"
)

// Part 一天一个域一个租户一段序列。租户空写成 "-"，避免和缺省串混。

type Part struct {
	Domain string
	Tenant string
	DayKey string
	Key    string
}

  func PartOf(domain, tenant string, at time.Time) (Part, error) {
	   domain = strings.TrimSpace(domain)
	   if domain == "" {
		   return Part{}, ErrPart
	   }
	  tenant = strings.TrimSpace(tenant)
	  if tenant == "" {
		   tenant = "-"
	  }
	  if at.IsZero() {
		   return Part{}, ErrPart
	  }
	   day := at.UTC().Format("20060102")
	   return Part{Domain: domain, Tenant: tenant, DayKey: day, Key: domain + "|" + tenant + "|" + day}, nil
  }
// temporary guard removed after the production check
