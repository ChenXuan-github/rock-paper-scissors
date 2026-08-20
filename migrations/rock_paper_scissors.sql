-- 用户表迁移：保存登录所需的最小账号信息。
CREATE TABLE IF NOT EXISTS users (
    -- BIGINT 对应 Go 的 int64；AUTO_INCREMENT 由 MySQL 生成主键。
    id BIGINT NOT NULL AUTO_INCREMENT,
    -- 登录名最长 64 字符，并通过唯一索引防止重复注册。
    username VARCHAR(64) NOT NULL,
    -- 只保存 BCrypt 哈希字符串，绝不保存明文密码。
    password_hash VARCHAR(255) NOT NULL,
    -- 插入时由数据库记录账号创建时间。
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uk_users_username (username)
) ENGINE = InnoDB
  -- utf8mb4 支持完整 Unicode，包括中文和 Emoji。
  DEFAULT CHARACTER SET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;

-- 玩家积分表：一名注册用户最多对应一条积分汇总记录。
CREATE TABLE IF NOT EXISTS player_scores (
    -- user_id 同时作为主键和外键，天然保证“一名用户一条积分记录”。
                                             user_id BIGINT NOT NULL,
    -- 允许玩家输到负分，所以使用有符号 INT。
                                             score INT NOT NULL DEFAULT 0,
                                             wins INT UNSIGNED NOT NULL DEFAULT 0,
                                             losses INT UNSIGNED NOT NULL DEFAULT 0,
                                             draws INT UNSIGNED NOT NULL DEFAULT 0,
                                             created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
                                             updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
                                             PRIMARY KEY (user_id),
    CONSTRAINT fk_player_scores_user
    FOREIGN KEY (user_id) REFERENCES users (id)
    ) ENGINE = InnoDB
    DEFAULT CHARACTER SET = utf8mb4
    COLLATE = utf8mb4_0900_ai_ci;

-- 为迁移执行前已经注册的老用户补齐零积分记录。
-- INSERT IGNORE 让迁移重复执行时不会覆盖已有玩家的真实积分。
INSERT IGNORE INTO player_scores (user_id)
SELECT id FROM users;

-- 对战记录表：一小局 1v1 对战只保存一条记录，不为双方重复保存两份。
CREATE TABLE IF NOT EXISTS game_records (
                                            id BIGINT NOT NULL AUTO_INCREMENT,
    -- 当前房间 ID 为 6 位字符串；保留到 16 位，允许以后调整生成规则。
                                            room_id VARCHAR(16) NOT NULL,
    player1_id BIGINT NOT NULL,
    player1_move VARCHAR(16) NOT NULL,
    player2_id BIGINT NOT NULL,
    player2_move VARCHAR(16) NOT NULL,
    -- 平局没有胜者，因此 winner_id 允许为 NULL。
    winner_id BIGINT NULL,
    player1_score_change INT NOT NULL,
    player2_score_change INT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_game_records_player1_created (player1_id, created_at),
    KEY idx_game_records_player2_created (player2_id, created_at),
    KEY idx_game_records_created (created_at),
    CONSTRAINT fk_game_records_player1
    FOREIGN KEY (player1_id) REFERENCES users (id),
    CONSTRAINT fk_game_records_player2
    FOREIGN KEY (player2_id) REFERENCES users (id),
    CONSTRAINT fk_game_records_winner
    FOREIGN KEY (winner_id) REFERENCES users (id),
    CONSTRAINT chk_game_records_different_players
    CHECK (player1_id <> player2_id),
    CONSTRAINT chk_game_records_player1_move
    CHECK (player1_move IN ('rock', 'scissors', 'paper')),
    CONSTRAINT chk_game_records_player2_move
    CHECK (player2_move IN ('rock', 'scissors', 'paper')),
    CONSTRAINT chk_game_records_winner
    CHECK (winner_id IS NULL OR winner_id IN (player1_id, player2_id))
    ) ENGINE = InnoDB
    DEFAULT CHARACTER SET = utf8mb4
    COLLATE = utf8mb4_0900_ai_ci;

-- 战绩详情需要展示当局结算后的积分快照。
ALTER TABLE game_records
    ADD COLUMN player1_score_after INT NOT NULL AFTER player1_score_change,
    ADD COLUMN player2_score_after INT NOT NULL AFTER player2_score_change;