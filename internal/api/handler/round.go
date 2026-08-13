package handler

import (
	"net/http"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/api/response"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/game"
	"github.com/gin-gonic/gin"
)

type evaluateRoundRequest struct {
	PlayerMove   string `json:"playerMove" binding:"required,oneof=rock scissors paper"`
	OpponentMove string `json:"opponentMove" binding:"required,oneof=rock scissors paper"`
}

type evaluateRoundResponse struct {
	PlayerMove   string `json:"playerMove"`
	OpponentMove string `json:"opponentMove"`
	Result       string `json:"result"`
}

func (r evaluateRoundRequest) toRound() (game.Round, error) {
	playerMove, err := game.ParseMove(r.PlayerMove)
	if err != nil {
		return game.Round{}, err
	}

	opponentMove, err := game.ParseMove(r.OpponentMove)
	if err != nil {
		return game.Round{}, err
	}

	return game.Round{
		PlayerMove:   playerMove,
		OpponentMove: opponentMove,
	}, nil
}

// EvaluateRound 接收双方出拳并返回本局结果。
func EvaluateRound(c *gin.Context) {
	var request evaluateRoundRequest

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(
			response.CodeInvalidRequest,
			"invalid request body",
		))
		return
	}

	round, err := request.toRound()
	if err != nil {
		c.JSON(http.StatusBadRequest, response.Error(
			response.CodeInvalidRequest,
			err.Error(),
		))
		return
	}

	round.Evaluate()

	c.JSON(http.StatusOK, response.Success(evaluateRoundResponse{
		PlayerMove:   round.PlayerMove.String(),
		OpponentMove: round.OpponentMove.String(),
		Result:       round.Result.String(),
	}))
}
