package settlement

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"strings"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/game"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/record"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/score"
)

// Command 是一局已经收齐双方出拳后，交给持久化结算服务的数据。
// Player1 和 Player2 只是这一条记录的两个方向，不代表房主或出拳先后顺序。
type Command struct {
	RoomID      string
	Player1ID   int64
	Player1Move game.Move
	Player2ID   int64
	Player2Move game.Move
}

// Outcome 是事务提交成功后的完整持久化结果。
type Outcome struct {
	Record       record.GameRecord
	Player1Score score.PlayerScore
	Player2Score score.PlayerScore
	Points       int
}

// pointGenerator 抽象随机积分生成，测试可以注入固定值，避免测试结果随机变化。
type pointGenerator interface {
	Next() int
}

type randomPointGenerator struct{}

// Next 返回包含左右边界的 [11, 19] 随机整数。
func (randomPointGenerator) Next() int {
	count := score.MaxWinPoints - score.MinWinPoints + 1
	return rand.IntN(count) + score.MinWinPoints
}

// Service 统一编排“一局记录 + 双方积分”的 MySQL 事务。
// 它持有 *sql.DB 是因为事务必须从数据库连接池开始。
type Service struct {
	db             *sql.DB
	pointGenerator pointGenerator
	rankingUpdater scoreRankingUpdater
}

// scoreRankingUpdater 是结算服务提交 MySQL 后需要的 Redis 排行榜更新能力。
type scoreRankingUpdater interface {
	UpdateScores(ctx context.Context, playerScores ...score.PlayerScore) error
}

// NewService 创建使用真实随机积分生成器的结算服务。
func NewService(db *sql.DB, rankingUpdaters ...scoreRankingUpdater) *Service {
	service := &Service{
		db:             db,
		pointGenerator: randomPointGenerator{},
	}
	if len(rankingUpdaters) > 0 {
		service.rankingUpdater = rankingUpdaters[0]
	}
	return service
}

// Settle 在同一个 MySQL 事务中更新双方积分并写入一条对战记录。
func (s *Service) Settle(ctx context.Context, command Command) (Outcome, error) {
	command.RoomID = strings.TrimSpace(command.RoomID)
	if s.db == nil || !validCommand(command) {
		return Outcome{}, ErrInvalidCommand
	}

	// 胜负属于纯内存规则，进入数据库事务前即可计算。
	player1Result := game.Evaluate(command.Player1Move, command.Player2Move)
	player2Result := game.Evaluate(command.Player2Move, command.Player1Move)

	// 平局固定为 0；非平局全局只随机一次，确保赢家 +N 与输家 -N 完全对应。
	points := 0
	if player1Result != game.Draw {
		points = s.pointGenerator.Next()
		if points < score.MinWinPoints || points > score.MaxWinPoints {
			return Outcome{}, ErrInvalidGeneratedPoints
		}
	}

	// BeginTx 相当于声明式事务开始；nil 表示使用数据库驱动的默认事务选项。
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Outcome{}, fmt.Errorf("begin settlement transaction: %w", err)
	}

	// 只要后面的 Commit 没有成功，任何提前 return 都会触发回滚。
	// Commit 成功后再 Rollback 会得到 sql.ErrTxDone，因此这里安全地忽略返回值。
	defer func() {
		_ = tx.Rollback()
	}()

	// 两个 Repository 都持有同一个 *sql.Tx，下面所有 SQL 才真正属于同一事务。
	txScoreService := score.NewService(score.NewMySQLRepository(tx))
	txRecordService := record.NewService(record.NewMySQLRepository(tx))

	// 兼容迁移后才注册、暂时还没有 player_scores 行的用户。
	if err := ensurePlayerScore(ctx, txScoreService, command.Player1ID); err != nil {
		return Outcome{}, fmt.Errorf("ensure player1 score: %w", err)
	}
	if err := ensurePlayerScore(ctx, txScoreService, command.Player2ID); err != nil {
		return Outcome{}, fmt.Errorf("ensure player2 score: %w", err)
	}

	// ApplyResult 使用 score = score + delta，因此负分会被正常保留。
	player1Score, err := txScoreService.ApplyResult(ctx, command.Player1ID, player1Result, points)
	if err != nil {
		return Outcome{}, fmt.Errorf("update player1 score: %w", err)
	}
	player2Score, err := txScoreService.ApplyResult(ctx, command.Player2ID, player2Result, points)
	if err != nil {
		return Outcome{}, fmt.Errorf("update player2 score: %w", err)
	}

	gameRecord := record.GameRecord{
		RoomID:             command.RoomID,
		Player1ID:          command.Player1ID,
		Player1Move:        command.Player1Move,
		Player2ID:          command.Player2ID,
		Player2Move:        command.Player2Move,
		Player1ScoreChange: scoreChange(player1Result, points),
		Player1ScoreAfter:  player1Score.Score,
		Player2ScoreChange: scoreChange(player2Result, points),
		Player2ScoreAfter:  player2Score.Score,
	}
	if player1Result == game.Win {
		winnerID := command.Player1ID
		gameRecord.WinnerID = &winnerID
	} else if player2Result == game.Win {
		winnerID := command.Player2ID
		gameRecord.WinnerID = &winnerID
	}

	createdRecord, err := txRecordService.Create(ctx, gameRecord)
	if err != nil {
		return Outcome{}, fmt.Errorf("save settlement record: %w", err)
	}

	// 只有三次写入和相关查询全部成功，才让 MySQL 永久保存这次结算。
	if err := tx.Commit(); err != nil {
		return Outcome{}, fmt.Errorf("commit settlement transaction: %w", err)
	}

	outcome := Outcome{
		Record:       createdRecord,
		Player1Score: player1Score,
		Player2Score: player2Score,
		Points:       points,
	}

	// Redis 不参与 MySQL 事务：提交成功后再更新排行榜。
	// Redis 短暂失败不能把已经提交的对局伪装成失败；下次启动会从 MySQL 全量重建修复。
	if s.rankingUpdater != nil {
		if err := s.rankingUpdater.UpdateScores(ctx, player1Score, player2Score); err != nil {
			log.Printf("update leaderboard after settlement: %v", err)
		}
	}
	return outcome, nil
}

func ensurePlayerScore(ctx context.Context, service *score.Service, userID int64) error {
	_, err := service.GetByUserID(ctx, userID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, score.ErrScoreNotFound) {
		return err
	}

	_, err = service.CreateForUser(ctx, userID)
	return err
}

func validCommand(command Command) bool {
	if command.RoomID == "" || len(command.RoomID) > 16 {
		return false
	}
	if command.Player1ID <= 0 || command.Player2ID <= 0 || command.Player1ID == command.Player2ID {
		return false
	}
	if command.Player1Move.String() == "unknown" || command.Player2Move.String() == "unknown" {
		return false
	}
	return true
}

func scoreChange(result game.Result, points int) int {
	switch result {
	case game.Win:
		return points
	case game.Lose:
		return -points
	default:
		return 0
	}
}
