package bus

import "strings"

// TopicSpec 主题规格（运维预建依据）。

type TopicSpec struct {
	Name       string `json:"name"`
	Partitions int    `json:"partitions"`
	ReplicationFactor int   `json:"replication_factor"`
	RetentionMS       int64 `json:"retention_ms"`
	ConsumerGroup string `json:"consumer_group,omitempty"`
	Purpose       string `json:"purpose"`
}

// FundingTopicPlan 资金面主题规划。

  func FundingTopicPlan() []TopicSpec {
	   groups := FundingConsumerGroups()
	  day := int64(24 * 60 * 60 * 1000)
	  return []TopicSpec{
		  {Name: TopicDepositCredited, Partitions: 12, ReplicationFactor: 3, RetentionMS: 7 * day, ConsumerGroup: groups["notify"], Purpose: "deposit credited notify"},
		  {Name: TopicDepositObserved, Partitions: 12, ReplicationFactor: 3, RetentionMS: 3 * day, ConsumerGroup: groups["asset"], Purpose: "deposit observed notify"},
		   {Name: TopicWithdrawRequested, Partitions: 12, ReplicationFactor: 3, RetentionMS: 7 * day, ConsumerGroup: groups["notify"], Purpose: "withdraw requested notify"},
		   {Name: TopicWithdrawBroadcast, Partitions: 12, ReplicationFactor: 3, RetentionMS: 7 * day, ConsumerGroup: groups["asset"], Purpose: "withdraw broadcast notify"},
		  {Name: TopicWithdrawConfirmed, Partitions: 12, ReplicationFactor: 3, RetentionMS: 7 * day, ConsumerGroup: groups["notify"], Purpose: "withdraw confirmed notify"},
		   {Name: TopicWithdrawCancelled, Partitions: 6, ReplicationFactor: 3, RetentionMS: 7 * day, ConsumerGroup: groups["notify"], Purpose: "withdraw cancelled notify"},
		  {Name: TopicLedgerPosted, Partitions: 24, ReplicationFactor: 3, RetentionMS: 3 * day, ConsumerGroup: groups["asset"], Purpose: "ledger posted notify"},
		  {Name: TopicReconDiff, Partitions: 6, ReplicationFactor: 3, RetentionMS: 14 * day, ConsumerGroup: groups["recon"], Purpose: "recon diff"},
		   {Name: TopicKYTHit, Partitions: 6, ReplicationFactor: 3, RetentionMS: 30 * day, ConsumerGroup: groups["risk"], Purpose: "kyt hit"},
		   {Name: TopicDriftBlocked, Partitions: 6, ReplicationFactor: 3, RetentionMS: 14 * day, ConsumerGroup: groups["risk"], Purpose: "ledger drift blocked"},
		   {Name: TopicOutboxPoison, Partitions: 3, ReplicationFactor: 3, RetentionMS: 30 * day, ConsumerGroup: groups["outbox"], Purpose: "outbox poison"},
		   {Name: TopicOutboxStuck, Partitions: 3, ReplicationFactor: 3, RetentionMS: 14 * day, ConsumerGroup: groups["outbox"], Purpose: "ops outbox lag"},
		  {Name: TopicRegistryLoss, Partitions: 3, ReplicationFactor: 3, RetentionMS: 7 * day, Purpose: "ops registry quorum loss"},
		   {Name: TopicWorkerCircuit, Partitions: 3, ReplicationFactor: 3, RetentionMS: 7 * day, Purpose: "ops worker circuit"},
	   }
   }

// TopicSpecByName 查找规格。

   func TopicSpecByName(name string) (TopicSpec, bool) {
	   name = strings.TrimSpace(name)
	  for _, s := range FundingTopicPlan() {
		  if s.Name == name {
			   return s, true
		   }
	   }
	   return TopicSpec{}, false
  }

// OpsTopics 运维主题清单。
   func OpsTopics() []string {
	   return []string{TopicOutboxPoison, TopicOutboxStuck, TopicRegistryLoss, TopicWorkerCircuit, TopicWorkerLeader}
   }

// AllManagedTopics 资金+运维主题并集。
   func AllManagedTopics() []string {
	  seen := map[string]struct{}{}
	   var out []string
	   for _, s := range FundingTopicPlan() {
		   if _, ok := seen[s.Name]; ok {
			   continue
		   }
		   seen[s.Name] = struct{}{}
		  out = append(out, s.Name)
	   }
	   for _, t := range FundingTopics() {
		   if _, ok := seen[t]; ok {
			   continue
		  }
		  seen[t] = struct{}{}
		  out = append(out, t)
	   }
	   return out
  }
