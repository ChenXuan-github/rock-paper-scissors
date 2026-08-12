package game

// Evaluate 根据玩家和对手出的拳，返回玩家这一方的胜负结果。
func Evaluate(playerMove Move, opponentMove Move) Result {

	// Move 的取值顺序是 Rock(0)、Scissors(1)、Paper(2)。
	// 加 3 可以避免相减后出现负数，再对 3 取模，将结果统一为：
	// 0 表示平局，1 表示输，2 表示赢。

	difference := (playerMove - opponentMove + 3) % 3

	if difference == 0 {
		return Draw
	}
	if difference == 2 {
		return Win
	}
	return Lose
}