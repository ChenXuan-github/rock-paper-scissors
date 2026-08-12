package game

type Move int

const (
	Rock     Move = iota // 石头
	Scissors             // 剪刀
	Paper                // 布
)

/**
实际上近似于：
const (
    Rock     Move = 0
    Paper    Move = 1
    Scissors Move = 2
)

也就是说：
Rock     == 0
Paper    == 1
Scissors == 2

为什么 Paper 和 Scissors 什么都没写？

这是 Go const 的另一个语法特性。

在同一个 const 块里，如果后面的常量省略表达式：

它会重复上一行的类型和表达式。
*/