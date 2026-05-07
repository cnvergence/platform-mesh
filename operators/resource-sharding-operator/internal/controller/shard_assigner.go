package controller

import "sync/atomic"

type ShardAssigner struct {
	shards  atomic.Pointer[[]string]
	counter atomic.Uint64
}

func NewShardAssigner(shards []string) *ShardAssigner {
	a := &ShardAssigner{}
	a.shards.Store(&shards)
	return a
}

// Next returns the next shard in round-robin order.
// Returns "" if no shards are configured.
func (a *ShardAssigner) Next() string {
	shards := *a.shards.Load()
	if len(shards) == 0 {
		return ""
	}
	idx := a.counter.Add(1) - 1
	return shards[idx%uint64(len(shards))]
}

// UpdateShards atomically replaces the shard list.
func (a *ShardAssigner) UpdateShards(shards []string) {
	a.shards.Store(&shards)
}
