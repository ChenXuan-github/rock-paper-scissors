package record

import (
	"context"
	"fmt"
	"strings"
)

const (
	maxRoomIDLength = 16
	maxPageSize     = 100
)

// Service 负责对战记录的输入校验、保存和历史查询。
// 它暂时不负责更新积分；跨表原子操作留给后续 SettlementService。
type Service struct {
	repository Repository
}

// NewService 注入对战记录 Repository；结算场景会注入持有同一个 *sql.Tx 的实现。
func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

// Create 校验一局记录的基本一致性后交给 Repository 保存。
func (s *Service) Create(ctx context.Context, gameRecord GameRecord) (GameRecord, error) {
	gameRecord.RoomID = strings.TrimSpace(gameRecord.RoomID)
	if !validRecord(gameRecord) {
		return GameRecord{}, ErrInvalidRecord
	}

	created, err := s.repository.Create(ctx, gameRecord)
	if err != nil {
		return GameRecord{}, fmt.Errorf("create game record: %w", err)
	}
	return created, nil
}

// GetByID 查询指定对局记录。
func (s *Service) GetByID(ctx context.Context, id int64) (GameRecord, error) {
	if id <= 0 {
		return GameRecord{}, ErrInvalidRecord
	}
	gameRecord, err := s.repository.FindByID(ctx, id)
	if err != nil {
		return GameRecord{}, fmt.Errorf("get game record by id: %w", err)
	}
	return gameRecord, nil
}

// ListByUserID 分页查询某名玩家参与的对战记录。
func (s *Service) ListByUserID(ctx context.Context, userID int64, limit, offset int) ([]GameRecord, error) {
	if userID <= 0 || limit <= 0 || limit > maxPageSize || offset < 0 {
		return nil, ErrInvalidPage
	}
	records, err := s.repository.ListByUserID(ctx, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list game records by user id: %w", err)
	}
	return records, nil
}

// validRecord 检查一条战绩内部的关联关系，防止明显矛盾的数据进入持久层。
func validRecord(gameRecord GameRecord) bool {
	if gameRecord.RoomID == "" || len(gameRecord.RoomID) > maxRoomIDLength {
		return false
	}
	if gameRecord.Player1ID <= 0 || gameRecord.Player2ID <= 0 || gameRecord.Player1ID == gameRecord.Player2ID {
		return false
	}
	if gameRecord.Player1Move.String() == "unknown" || gameRecord.Player2Move.String() == "unknown" {
		return false
	}
	if gameRecord.WinnerID != nil && *gameRecord.WinnerID != gameRecord.Player1ID && *gameRecord.WinnerID != gameRecord.Player2ID {
		return false
	}
	return true
}
