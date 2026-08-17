package game

// RoundState 表示房间中当前一小局从等待出拳到完成结算的运行状态。
// 原来的 Round 保存两只拳和胜负结果；RoundState 额外记录每只拳属于哪个用户。
type RoundState struct {
	// moves 保存每名玩家本局提交的拳。
	moves map[int64]Move
	// results 保存结算后每名玩家各自视角的结果。
	results map[int64]Result
	// settled 表示双方是否都已出拳并完成胜负计算。
	settled bool
}

// RoundStateSnapshot 是当前小局状态的只读快照，避免调用方直接修改房间内部 Map。
type RoundStateSnapshot struct {
	// SubmittedCount 只表示已有几人提交，不泄露未结算时另一名玩家的具体拳型。
	SubmittedCount int
	// Settled 为 true 时，Moves 和 Results 才包含一局完整的双方数据。
	Settled bool
	Moves   map[int64]Move
	Results map[int64]Result
}

// newRoundState 为每次新开始的小局创建独立状态，防止沿用上一局的拳和结果。
func newRoundState() *RoundState {
	return &RoundState{
		// 两个 Map 都按 1v1 最大人数预分配容量，但初始长度仍然是 0。
		moves:   make(map[int64]Move, maxPlayersPerRoom),
		results: make(map[int64]Result, maxPlayersPerRoom),
	}
}

// snapshot 复制当前一小局的状态。调用它时，Room 已经持有自己的锁。
func (r *RoundState) snapshot() RoundStateSnapshot {
	// Map 是引用类型，直接返回内部 Map 会让上层绕过 Room 的锁修改当前小局。
	// 因此分别创建新 Map，并逐项复制键值。
	moves := make(map[int64]Move, len(r.moves))
	for userID, move := range r.moves {
		moves[userID] = move
	}

	results := make(map[int64]Result, len(r.results))
	for userID, result := range r.results {
		results[userID] = result
	}

	return RoundStateSnapshot{
		SubmittedCount: len(r.moves),
		Settled:        r.settled,
		Moves:          moves,
		Results:        results,
	}
}

// oppositeResult 把第一名玩家视角的结果转换成对手视角。
func oppositeResult(result Result) Result {
	// 一局只有双方视角：第一方获胜必然意味着第二方失败，平局则双方相同。
	switch result {
	case Win:
		return Lose
	case Lose:
		return Win
	case Draw:
		return Draw
	default:
		return ResultPending
	}
}
