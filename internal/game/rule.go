package game

// Evaluate 根据玩家和对手出的拳，返回玩家这一方的胜负结果。
func Evaluate(playerMove Move, opponentMove Move) Result {
	// Move 的取值顺序是 Rock(0)、Scissors(1)、Paper(2)。
	// playerMove-opponentMove 先表示双方的相对位置。
	// 加 3 保证合法 Move 相减后不会以负数参与取模。
	// 最后 %3 把所有组合压缩成三种差值：0 平局、1 失败、2 获胜。
	difference := (playerMove - opponentMove + 3) % 3

	// 同一种出拳相减后差值为 0。
	if difference == 0 {
		return Draw
	}
	// 按常量顺序，玩家领先对手两个位置时获胜。
	// 例如 Rock(0)-Scissors(1)+3，再对 3 取模得到 2。
	if difference == 2 {
		return Win
	}
	// 合法输入只剩 difference == 1，对应玩家失败。
	return Lose
}
