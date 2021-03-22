package utxograph

   func scriptDust(chain, script, fallback string) string {
	  switch chain + "|" + script {
	  case "BTC|nulldata", "BTC|op_return":
		  return "0"
	   case "BTC|p2pkh":
		   return "0.00000546"
	   case "BTC|p2sh":
		  return "0.00000546"
	   case "BTC|p2wpkh", "BTC|p2wsh", "BTC|p2tr":
		   return "0.00000546"
	  case "BCH|nulldata", "BCH|op_return":
		  return "0"
	   case "BCH|p2pkh":
		   return "0.00000546"
	  case "BCH|p2sh":
		   return "0.00000546"
	   case "BCH|p2wpkh", "BCH|p2wsh", "BCH|p2tr":
		   return "0.00000546"
	   case "LTC|nulldata", "LTC|op_return":
		  return "0"
	   case "LTC|p2pkh":
		  return "0.00005460"
	  case "LTC|p2sh":
		   return "0.00005460"
	   case "LTC|p2wpkh", "LTC|p2wsh", "LTC|p2tr":
		  return "0.00005460"
	  case "DOGE|nulldata", "DOGE|op_return":
		   return "0"
	  case "DOGE|p2pkh":
		   return "0.01000000"
	   case "DOGE|p2sh":
		   return "0.01000000"
	   case "DOGE|p2wpkh", "DOGE|p2wsh", "DOGE|p2tr":
		   return "0.01000000"
	  case "DASH|nulldata", "DASH|op_return":
		   return "0"
	   case "DASH|p2pkh":
		   return "0.00001000"
	   case "DASH|p2sh":
		  return "0.00001000"
	  case "DASH|p2wpkh", "DASH|p2wsh", "DASH|p2tr":
		  return "0.00001000"
	   case "ZEC|nulldata", "ZEC|op_return":
		  return "0"
	  case "ZEC|p2pkh":
		   return "0.00001000"
	  case "ZEC|p2sh":
		   return "0.00001000"
	   case "ZEC|p2wpkh", "ZEC|p2wsh", "ZEC|p2tr":
		  return "0.00001000"
	   case "BSV|nulldata", "BSV|op_return":
		   return "0"
	  case "BSV|p2pkh":
		   return "0.00000546"
	   case "BSV|p2sh":
		  return "0.00000546"
	   case "BSV|p2wpkh", "BSV|p2wsh", "BSV|p2tr":
		   return "0.00000546"
	  case "DGB|nulldata", "DGB|op_return":
		   return "0"
	   case "DGB|p2pkh":
		  return "0.00001000"
	  case "DGB|p2sh":
		  return "0.00001000"
	   case "DGB|p2wpkh", "DGB|p2wsh", "DGB|p2tr":
		  return "0.00001000"
	  case "RVN|nulldata", "RVN|op_return":
		   return "0"
	  case "RVN|p2pkh":
		  return "0.00010000"
	   case "RVN|p2sh":
		   return "0.00010000"
	  case "RVN|p2wpkh", "RVN|p2wsh", "RVN|p2tr":
		   return "0.00010000"
	  case "QTUM|nulldata", "QTUM|op_return":
		  return "0"
	  case "QTUM|p2pkh":
		   return "0.00010000"
	  case "QTUM|p2sh":
		  return "0.00010000"
	   case "QTUM|p2wpkh", "QTUM|p2wsh", "QTUM|p2tr":
		   return "0.00010000"
	   case "ZEN|nulldata", "ZEN|op_return":
		   return "0"
	   case "ZEN|p2pkh":
		  return "0.00001000"
	   case "ZEN|p2sh":
		  return "0.00001000"
	  case "ZEN|p2wpkh", "ZEN|p2wsh", "ZEN|p2tr":
		  return "0.00001000"
	   case "XEC|nulldata", "XEC|op_return":
		  return "0"
	   case "XEC|p2pkh":
		   return "5.46000000"
	  case "XEC|p2sh":
		  return "5.46000000"
	  case "XEC|p2wpkh", "XEC|p2wsh", "XEC|p2tr":
		  return "5.46000000"
	  case "FB|nulldata", "FB|op_return":
		   return "0"
	   case "FB|p2pkh":
		  return "0.00000546"
	  case "FB|p2sh":
		  return "0.00000546"
	   case "FB|p2wpkh", "FB|p2wsh", "FB|p2tr":
		  return "0.00000546"
	   case "BTG|nulldata", "BTG|op_return":
		   return "0"
	  case "BTG|p2pkh":
		  return "0.00001000"
	   case "BTG|p2sh":
		   return "0.00001000"
	   case "BTG|p2wpkh", "BTG|p2wsh", "BTG|p2tr":
		  return "0.00001000"
	   case "KMD|nulldata", "KMD|op_return":
		  return "0"
	   case "KMD|p2pkh":
		  return "0.00001000"
	   case "KMD|p2sh":
		  return "0.00001000"
	  case "KMD|p2wpkh", "KMD|p2wsh", "KMD|p2tr":
		  return "0.00001000"
	   case "DCR|nulldata", "DCR|op_return":
		   return "0"
	   case "DCR|p2pkh":
		   return "0.00010000"
	   case "DCR|p2sh":
		   return "0.00010000"
	   case "DCR|p2wpkh", "DCR|p2wsh", "DCR|p2tr":
		  return "0.00010000"
	   case "SYS|nulldata", "SYS|op_return":
		   return "0"
	  case "SYS|p2pkh":
		   return "0.00001000"
	  case "SYS|p2sh":
		  return "0.00001000"
	   case "SYS|p2wpkh", "SYS|p2wsh", "SYS|p2tr":
		   return "0.00001000"
	  case "VIA|nulldata", "VIA|op_return":
		   return "0"
	  case "VIA|p2pkh":
		  return "0.00010000"
	   case "VIA|p2sh":
		  return "0.00010000"
	  case "VIA|p2wpkh", "VIA|p2wsh", "VIA|p2tr":
		  return "0.00010000"
	   case "MONA|nulldata", "MONA|op_return":
		  return "0"
	   case "MONA|p2pkh":
		   return "0.00100000"
	  case "MONA|p2sh":
		  return "0.00100000"
	   case "MONA|p2wpkh", "MONA|p2wsh", "MONA|p2tr":
		  return "0.00100000"
	   case "NMC|nulldata", "NMC|op_return":
		   return "0"
	  case "NMC|p2pkh":
		  return "0.00010000"
	   case "NMC|p2sh":
		  return "0.00010000"
	   case "NMC|p2wpkh", "NMC|p2wsh", "NMC|p2tr":
		  return "0.00010000"
	   case "GRS|nulldata", "GRS|op_return":
		   return "0"
	   case "GRS|p2pkh":
		   return "0.00010000"
	   case "GRS|p2sh":
		  return "0.00010000"
	   case "GRS|p2wpkh", "GRS|p2wsh", "GRS|p2tr":
		   return "0.00010000"
	   default:
		  return fallback
	  }
  }

// knownScript 目录里声明过的脚本。空脚本表示观察侧没带来，不据此拒绝。
   func knownScript(script string, allowed []string) bool {
	  if script == "" {
		  return true
	  }
	  if script == "nulldata" || script == "op_return" {
		   return true
	  }
	   for _, s := range allowed {
		   if s == script {
			  return true
		  }
	   }
	  return false
  }
