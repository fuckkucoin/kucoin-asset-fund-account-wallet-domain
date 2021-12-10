package travelvasp

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/validate"
)

const (
	IVMSNatural = "NATN"
	IVMSLegal = "LEGL"
)

// IVMSParty IVMS101 风格一方。

type IVMSParty struct {
	Name          string `json:"name"`
	AccountNumber string `json:"account_number"`
	VASP          string `json:"vasp"`
	PersonType    string `json:"person_type,omitempty"`
	Country           string `json:"country,omitempty"`
	GeographicAddress string `json:"geographic_address,omitempty"`
	NationalIdentifier string `json:"national_identifier,omitempty"`
	DateOfBirth        string `json:"date_of_birth,omitempty"`
	CustomerNumber string `json:"customer_number,omitempty"`
}

// IVMSMap 提交前字段图。

type IVMSMap struct {
	Asset      string    `json:"asset"`
	Amount     string    `json:"amount"`
	Chain      string    `json:"chain,omitempty"`
	Originator IVMSParty `json:"originator"`
	Beneficiary IVMSParty `json:"beneficiary"`
}

  func (p IVMSParty) personType() string {
	   t := strings.ToUpper(strings.TrimSpace(p.PersonType))
	  if t == "" {
		  return IVMSNatural
	  }
	  return t
   }

   func (p IVMSParty) identityOK() bool {
	   return strings.TrimSpace(p.GeographicAddress) != "" ||
		  strings.TrimSpace(p.NationalIdentifier) != "" ||
		  strings.TrimSpace(p.DateOfBirth) != ""
  }

  func checkIVMSParty(p IVMSParty, legalNeedAddr bool) error {
	   if strings.TrimSpace(p.Name) == "" || strings.TrimSpace(p.AccountNumber) == "" || strings.TrimSpace(p.VASP) == "" {
		  return ErrIVMS
	  }
	   if strings.TrimSpace(p.Country) == "" {
		   return ErrIVMS
	  }
	   switch p.personType() {
	   case IVMSLegal:
		  if legalNeedAddr && strings.TrimSpace(p.GeographicAddress) == "" {
			  return ErrIVMS
		  }
	  case IVMSNatural:
		  if !p.identityOK() {
			   return ErrIVMS
		  }
	  default:
		   return ErrIVMS
	   }
	   return nil
   }

// ValidateIVMS 强制字段图。缺任一项失败关闭。
  func ValidateIVMS(m IVMSMap) error {
	  if _, err := validate.Asset(m.Asset); err != nil {
		   return ErrIVMS
	   }
	  if strings.TrimSpace(m.Amount) == "" {
		  return ErrIVMS
	   }
	   if err := checkIVMSParty(m.Originator, true); err != nil {
		  return err
	  }
	  if err := checkIVMSParty(m.Beneficiary, true); err != nil {
		   return err
	   }
	   return nil
  }

// InspectIVMS 按案件补全后校验字段图。缺强制项失败关闭。

  func (s *Service) InspectIVMS(id string, extra IVMSMap) (IVMSMap, error) {
	  cs, err := s.GetCase(id)
	   if err != nil {
		  return IVMSMap{}, err
	  }
	  m := ivmsFromCase(cs, extra)
	  if err := ValidateIVMS(m); err != nil {
		   return m, err
	  }
	   return m, nil
  }

   func ivmsFromCase(cs Case, extra IVMSMap) IVMSMap {
	  m := extra
	   if strings.TrimSpace(m.Asset) == "" {
		  m.Asset = cs.Asset
	  }
	  if strings.TrimSpace(m.Amount) == "" {
		   m.Amount = cs.Amount
	   }
	  if strings.TrimSpace(m.Chain) == "" {
		   m.Chain = cs.Chain
	   }
	  m.Originator = fillIVMS(m.Originator, cs.Originator)
	  m.Beneficiary = fillIVMS(m.Beneficiary, cs.Beneficiary)
	   return m
   }

  func fillIVMS(p IVMSParty, src Party) IVMSParty {
	  if strings.TrimSpace(p.Name) == "" {
		  p.Name = src.Name
	   }
	   if strings.TrimSpace(p.AccountNumber) == "" {
		  p.AccountNumber = src.Account
	  }
	  if strings.TrimSpace(p.VASP) == "" {
		   p.VASP = src.VASP
	   }
	   return p
  }

  func ivmsFromReq(req OutboundReq, orig, bene Party) IVMSMap {
	   return IVMSMap{
		   Asset: req.Asset, Amount: req.Amount, Chain: req.Chain,
		   Originator: IVMSParty{
			  Name: orig.Name, AccountNumber: orig.Account, VASP: orig.VASP,
			   PersonType: req.OrigType, Country: req.OrigCountry,
			   GeographicAddress: req.OrigAddress, NationalIdentifier: req.OrigNatID,
			  DateOfBirth: req.OrigDOB,
			  CustomerNumber: req.UserID,
		   },
		  Beneficiary: IVMSParty{
			   Name: bene.Name, AccountNumber: bene.Account, VASP: bene.VASP,
			  PersonType:        req.BeneType,
			  Country:           req.BeneCountry,
			   GeographicAddress: req.BeneAddress, NationalIdentifier: req.BeneNatID,
			  DateOfBirth: req.BeneDOB,
		   },
	  }
   }
