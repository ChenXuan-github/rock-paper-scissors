package handler

import (
	"context"
	"log"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/api/middleware"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/api/response"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/leaderboard"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/record"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/score"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/user"
	"github.com/gin-gonic/gin"
)

const defaultRecordLimit = 50

// scoreQueryService 是积分相关 HTTP 接口需要的最小查询能力。
type scoreQueryService interface {
	GetOrCreate(ctx context.Context, userID int64) (score.PlayerScore, error)
	GetByUserIDs(ctx context.Context, userIDs []int64) (map[int64]score.PlayerScore, error)
}

// recordQueryService 隔离 Handler 与战绩的 MySQL 实现，测试可注入假对象。
type recordQueryService interface {
	ListByUserID(ctx context.Context, userID int64, limit, offset int) ([]record.GameRecord, error)
}

// leaderboardQueryService 描述 Handler 对 Redis 排行榜的读写需求。
type leaderboardQueryService interface {
	UpdateScores(ctx context.Context, playerScores ...score.PlayerScore) error
	Top(ctx context.Context, limit int) ([]leaderboard.Entry, error)
	Rank(ctx context.Context, userID int64) (*int, error)
}

// leaderboardUserQueryService 用 UserID 补齐排行榜展示所需的用户名。
type leaderboardUserQueryService interface {
	GetByIDs(ctx context.Context, ids []int64) (map[int64]user.User, error)
}

// CompetitionHandler 负责当前玩家的积分汇总与历史战绩读取接口。
type CompetitionHandler struct {
	scoreService  scoreQueryService
	recordService recordQueryService
	leaderboard   leaderboardQueryService
	userService   leaderboardUserQueryService
}

// NewCompetitionHandler 组装积分、战绩、排行榜和用户资料四个查询依赖。
func NewCompetitionHandler(
	scoreService scoreQueryService,
	recordService recordQueryService,
	leaderboardService leaderboardQueryService,
	userService leaderboardUserQueryService,
) *CompetitionHandler {
	return &CompetitionHandler{
		scoreService:  scoreService,
		recordService: recordService,
		leaderboard:   leaderboardService,
		userService:   userService,
	}
}

// myScoreResponse 是“我的积分”接口的 VO，不向客户端暴露持久层时间等内部字段。
type myScoreResponse struct {
	Score   int     `json:"score"`
	Rank    *int    `json:"rank"`
	Wins    uint    `json:"wins"`
	Losses  uint    `json:"losses"`
	Draws   uint    `json:"draws"`
	WinRate float64 `json:"winRate"`
}

// battleRecordResponse 把数据库中固定的 Player1/Player2 方向转换成当前用户视角。
type battleRecordResponse struct {
	ID           string    `json:"id"`
	RoomID       string    `json:"roomId"`
	PlayedAt     time.Time `json:"playedAt"`
	OpponentName string    `json:"opponentName"`
	MyMove       string    `json:"myMove"`
	OpponentMove string    `json:"opponentMove"`
	Result       string    `json:"result"`
	ScoreDelta   int       `json:"scoreDelta"`
	ScoreAfter   int       `json:"scoreAfter"`
}

// rankingEntryResponse 在 Redis 排序结果上补充 MySQL 中的用户名和胜率。
type rankingEntryResponse struct {
	Rank     int     `json:"rank"`
	UserID   int64   `json:"userId"`
	Username string  `json:"username"`
	Score    int     `json:"score"`
	Wins     uint    `json:"wins"`
	WinRate  float64 `json:"winRate"`
}

// MyScore 返回当前 JWT 用户的永久积分、胜负统计和 Redis ZSet 实时名次。
func (h *CompetitionHandler) MyScore(c *gin.Context) {
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, response.Error(http.StatusUnauthorized, "invalid or missing access token"))
		return
	}

	playerScore, err := h.scoreService.GetOrCreate(c.Request.Context(), userID)
	if err != nil {
		log.Printf("get current player score: %v", err)
		c.JSON(http.StatusInternalServerError, response.Error(response.CodeInternalError, "internal server error"))
		return
	}

	var rank *int
	if h.leaderboard != nil {
		// 确保迁移后新注册、尚未经历对局的零分用户也进入 ZSet。
		if err := h.leaderboard.UpdateScores(c.Request.Context(), playerScore); err != nil {
			log.Printf("sync current player leaderboard score: %v", err)
		} else {
			rank, err = h.leaderboard.Rank(c.Request.Context(), userID)
			if err != nil {
				log.Printf("get current player rank: %v", err)
			}
		}
	}

	c.JSON(http.StatusOK, response.Success(myScoreResponse{
		Score:   playerScore.Score,
		Rank:    rank,
		Wins:    playerScore.Wins,
		Losses:  playerScore.Losses,
		Draws:   playerScore.Draws,
		WinRate: calculateWinRate(playerScore),
	}))
}

// Ranking 返回 Redis ZSet 中按积分倒序排列的真实排行榜。
func (h *CompetitionHandler) Ranking(c *gin.Context) {
	limit, ok := positiveQueryInt(c, "limit", 50)
	if !ok || limit > 100 {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "invalid ranking limit"))
		return
	}
	if h.leaderboard == nil || h.userService == nil {
		c.JSON(http.StatusInternalServerError, response.Error(response.CodeInternalError, "leaderboard is unavailable"))
		return
	}

	entries, err := h.leaderboard.Top(c.Request.Context(), limit)
	if err != nil {
		log.Printf("read leaderboard: %v", err)
		c.JSON(http.StatusInternalServerError, response.Error(response.CodeInternalError, "internal server error"))
		return
	}

	// Redis 只提供顺序、UserID 和积分。先收集全部 ID，再分别批量读取两个 MySQL 表。
	userIDs := make([]int64, 0, len(entries))
	for _, entry := range entries {
		userIDs = append(userIDs, entry.UserID)
	}
	usersByID, err := h.userService.GetByIDs(c.Request.Context(), userIDs)
	if err != nil {
		log.Printf("batch load leaderboard users: %v", err)
		c.JSON(http.StatusInternalServerError, response.Error(response.CodeInternalError, "internal server error"))
		return
	}
	scoresByUserID, err := h.scoreService.GetByUserIDs(c.Request.Context(), userIDs)
	if err != nil {
		log.Printf("batch load leaderboard scores: %v", err)
		c.JSON(http.StatusInternalServerError, response.Error(response.CodeInternalError, "internal server error"))
		return
	}

	// 两个 Map 把按 ID 找实体的成本降为 O(1)，循环只负责保持 Redis 给出的名次顺序。
	result := make([]rankingEntryResponse, 0, len(entries))
	for _, entry := range entries {
		player, userExists := usersByID[entry.UserID]
		playerScore, scoreExists := scoresByUserID[entry.UserID]
		if !userExists || !scoreExists {
			// 理论上不会发生；若 Redis 与 MySQL 短暂不一致，跳过脏成员并记录日志。
			log.Printf(
				"skip incomplete leaderboard entry user=%d userExists=%t scoreExists=%t",
				entry.UserID,
				userExists,
				scoreExists,
			)
			continue
		}
		result = append(result, rankingEntryResponse{
			Rank:     entry.Rank,
			UserID:   entry.UserID,
			Username: player.Username,
			Score:    entry.Score,
			Wins:     playerScore.Wins,
			WinRate:  calculateWinRate(playerScore),
		})
	}

	c.JSON(http.StatusOK, response.Success(result))
}

// MyRecords 返回当前 JWT 用户最近的历史对局，并统一转换成“我的视角”。
func (h *CompetitionHandler) MyRecords(c *gin.Context) {
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, response.Error(http.StatusUnauthorized, "invalid or missing access token"))
		return
	}

	limit, ok := positiveQueryInt(c, "limit", defaultRecordLimit)
	if !ok || limit > 100 {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "invalid pagination"))
		return
	}
	offset, ok := nonNegativeQueryInt(c, "offset", 0)
	if !ok {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "invalid pagination"))
		return
	}

	records, err := h.recordService.ListByUserID(c.Request.Context(), userID, limit, offset)
	if err != nil {
		log.Printf("list current player records: %v", err)
		c.JSON(http.StatusInternalServerError, response.Error(response.CodeInternalError, "internal server error"))
		return
	}

	result := make([]battleRecordResponse, 0, len(records))
	for _, gameRecord := range records {
		result = append(result, toBattleRecordResponse(gameRecord, userID))
	}
	c.JSON(http.StatusOK, response.Success(result))
}

// toBattleRecordResponse 根据当前用户位于 Player1 还是 Player2，交换双方展示方向。
func toBattleRecordResponse(gameRecord record.GameRecord, userID int64) battleRecordResponse {
	responseValue := battleRecordResponse{
		ID:       strconv.FormatInt(gameRecord.ID, 10),
		RoomID:   gameRecord.RoomID,
		PlayedAt: gameRecord.CreatedAt,
		Result:   "draw",
	}

	if gameRecord.Player1ID == userID {
		responseValue.OpponentName = gameRecord.Player2Username
		responseValue.MyMove = gameRecord.Player1Move.String()
		responseValue.OpponentMove = gameRecord.Player2Move.String()
		responseValue.ScoreDelta = gameRecord.Player1ScoreChange
		responseValue.ScoreAfter = gameRecord.Player1ScoreAfter
	} else {
		responseValue.OpponentName = gameRecord.Player1Username
		responseValue.MyMove = gameRecord.Player2Move.String()
		responseValue.OpponentMove = gameRecord.Player1Move.String()
		responseValue.ScoreDelta = gameRecord.Player2ScoreChange
		responseValue.ScoreAfter = gameRecord.Player2ScoreAfter
	}

	// winner_id 为 NULL 代表平局；非空时再判断当前用户是赢家还是输家。
	if gameRecord.WinnerID != nil {
		if *gameRecord.WinnerID == userID {
			responseValue.Result = "win"
		} else {
			responseValue.Result = "lose"
		}
	}
	return responseValue
}

// positiveQueryInt 解析必须大于零的查询参数；未传时使用默认值。
func positiveQueryInt(c *gin.Context, key string, fallback int) (int, bool) {
	raw := c.Query(key)
	if raw == "" {
		return fallback, true
	}
	value, err := strconv.Atoi(raw)
	return value, err == nil && value > 0
}

// nonNegativeQueryInt 用于 offset 一类允许为零、但不能为负数的查询参数。
func nonNegativeQueryInt(c *gin.Context, key string, fallback int) (int, bool) {
	raw := c.Query(key)
	if raw == "" {
		return fallback, true
	}
	value, err := strconv.Atoi(raw)
	return value, err == nil && value >= 0
}

// calculateWinRate 返回保留一位小数的百分比；尚无对局时避免除零并返回 0。
func calculateWinRate(playerScore score.PlayerScore) float64 {
	total := playerScore.Wins + playerScore.Losses + playerScore.Draws
	if total == 0 {
		return 0
	}
	return math.Round(float64(playerScore.Wins)/float64(total)*1000) / 10
}
