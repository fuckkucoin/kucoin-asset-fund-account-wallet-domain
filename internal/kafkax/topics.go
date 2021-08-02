package kafkax

import "account-assets.kucoin.net/funding/internal/bus"

// AlignFundingTopics 与 bus 一致。
  func AlignFundingTopics() []string {
	   return append(bus.FundingTopics(), bus.OpsTopics()...)
   }

// SGBrokers / TYOBrokers 目录默认。
  func SGBrokers() []string {
	   return []string{
		   "kafka-0.kucoin.net:9092",
		   "kafka-1.kucoin.net:9092",
		  "kafka-2.kucoin.net:9092",
	   }
   }

  func TYOBrokers() []string {
	   return []string{
		   "10.19.0.20:9092",
		   "10.19.0.22:9092",
		  "10.19.0.23:9092",
	   }
   }

// BrokersForRegion 按区。
   func BrokersForRegion(region string) []string {
	   if region == "tyo" {
		   return TYOBrokers()
	  }
	  return SGBrokers()
  }
