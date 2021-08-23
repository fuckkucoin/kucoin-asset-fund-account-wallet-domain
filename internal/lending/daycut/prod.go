package daycut

import "strings"

type Config struct {
	Profile string
	AllowRollback   bool
	AllowMissPost   bool
	AllowDoublePost bool
	AllowRebatchOpen bool
}

  func Production() Config {
	   return Config{
		  Profile:       "production",
		   AllowRollback: false,
		  AllowMissPost: false,
		  AllowDoublePost: false,
		   AllowRebatchOpen: false,
	  }
  }

   func Lab() Config {
	  return Config{Profile: "lab", AllowRollback: false, AllowMissPost: false, AllowDoublePost: false}
   }

   func (c Config) ProductionLocked() bool {
	   p := strings.ToLower(strings.TrimSpace(c.Profile))
	  return p == "prod" || p == "production" || p == "iam"
  }

  func (c Config) GuardRollback() error {
	   if c.ProductionLocked() && c.AllowRollback {
		  return ErrRollback
	  }
	  if c.ProductionLocked() && !c.AllowRollback {
		   return nil
	  }
	   if c.AllowRollback {
		  return nil
	  }
	  return nil
  }

   func (c Config) RefuseRollback() error {
	  if c.AllowRollback && !c.ProductionLocked() {
		   return nil
	  }
	   return ErrRollback
   }

  func (c Config) RefuseMissPost() error {
	   if c.AllowMissPost && !c.ProductionLocked() {
		  return nil
	   }
	  return ErrMissPost
  }

   func (c Config) RefuseDoublePost() error {
	  if c.AllowDoublePost && !c.ProductionLocked() {
		   return nil
	  }
	  return ErrDoublePost
  }

  func (c Config) RefuseRebatch() error {
	  if c.AllowRebatchOpen && !c.ProductionLocked() {
		  return nil
	  }
	  return ErrRebatch
   }
