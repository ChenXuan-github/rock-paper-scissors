package game

// Round 表示一局完整的剪刀石头布对战。
type Round struct {
	PlayerMove   Move
	OpponentMove Move
	Result       Result
}

// Evaluate 根据双方出拳计算并记录本局结果。
func (r *Round) Evaluate() {
	r.Result = Evaluate(r.PlayerMove, r.OpponentMove)
}

/**
就是有这么个类， 里面三个成员变量， 前两个是依据，第三个是结果，
就是一个内部成员方法利用两个成员变量的状态更新第三个成员变量的值
*/