package game

// Round 表示一局完整的剪刀石头布对战。
// PlayerMove 和 OpponentMove 是判定依据，Result 是计算后保存的结果。
type Round struct {
	PlayerMove   Move
	OpponentMove Move
	Result       Result
}

// Evaluate 根据双方出拳计算并记录本局结果。
func (r *Round) Evaluate() {
	// 指针接收者 *Round 让方法可以修改原对象，而不是只修改结构体副本。
	// 纯规则函数 Evaluate 负责计算；Round 方法负责把计算结果写回自身状态。
	r.Result = Evaluate(r.PlayerMove, r.OpponentMove)
}
