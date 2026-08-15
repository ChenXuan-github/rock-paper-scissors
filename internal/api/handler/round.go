package handler

import (
	"net/http"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/api/response"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/game"
	"github.com/gin-gonic/gin"
)

// evaluateRoundRequest 是 POST /rounds/evaluate 的请求 DTO。
// json 标签定义前端字段名；binding 标签让 Gin 完成必填和枚举校验。
type evaluateRoundRequest struct {
	PlayerMove   string `json:"playerMove" binding:"required,oneof=rock scissors paper"`
	OpponentMove string `json:"opponentMove" binding:"required,oneof=rock scissors paper"`
}

// evaluateRoundResponse 是接口的响应 DTO，只包含允许暴露给客户端的字段。
type evaluateRoundResponse struct {
	PlayerMove   string `json:"playerMove"`
	OpponentMove string `json:"opponentMove"`
	Result       string `json:"result"`
}

// toRound 把 Web 层字符串 DTO 转成 game 包认识的领域对象。
func (r evaluateRoundRequest) toRound() (game.Round, error) {
	// ParseMove 返回 (Move, error)，必须先检查错误再使用 Move。
	playerMove, err := game.ParseMove(r.PlayerMove)
	if err != nil {
		return game.Round{}, err
	}

	opponentMove, err := game.ParseMove(r.OpponentMove)
	if err != nil {
		return game.Round{}, err
	}

	// Result 未赋值时使用零值 ResultPending，随后由 Evaluate 更新。
	return game.Round{
		PlayerMove:   playerMove,
		OpponentMove: opponentMove,
	}, nil
}

// EvaluateRound 接收双方出拳并返回本局结果。
func EvaluateRound(c *gin.Context) {
	// request 是当前请求体反序列化后的临时 DTO。
	var request evaluateRoundRequest

	// ShouldBindJSON 同时完成 JSON 反序列化和 binding 标签校验。
	if err := c.ShouldBindJSON(&request); err != nil {
		// 请求格式或字段值不合法属于客户端错误，写出 400 后立即结束 Handler。
		c.JSON(http.StatusBadRequest, response.Error(
			response.CodeInvalidRequest,
			"invalid request body",
		))
		return
	}

	// Web 字符串转换为强类型领域对象，规则层不再处理 JSON 字符串。
	round, err := request.toRound()
	if err != nil {
		c.JSON(http.StatusBadRequest, response.Error(
			response.CodeInvalidRequest,
			err.Error(),
		))
		return
	}

	// 调用领域方法，根据双方 Move 更新 Result。
	round.Evaluate()

	// 把领域类型转回对外字符串，并套入统一成功响应。
	c.JSON(http.StatusOK, response.Success(evaluateRoundResponse{
		PlayerMove:   round.PlayerMove.String(),
		OpponentMove: round.OpponentMove.String(),
		Result:       round.Result.String(),
	}))
}
