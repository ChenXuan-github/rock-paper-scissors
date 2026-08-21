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

-- 好友关系表：把用户视为图上的节点，一行数据表示两个节点之间的一条无向边。
CREATE TABLE IF NOT EXISTS friendships (
    -- 始终把较小的用户 ID 放在 low、较大的放在 high，避免同时保存 A-B 和 B-A。
    user_id_low BIGINT NOT NULL,
    user_id_high BIGINT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- 联合主键既是关系主键，也从数据库层保证同一条无向边只能出现一次。
    PRIMARY KEY (user_id_low, user_id_high),
    -- 主键适合按 low 查询；反向索引让“当前用户位于 high”时也能快速找到好友。
    KEY idx_friendships_high_low (user_id_high, user_id_low),
    CONSTRAINT fk_friendships_user_low
        FOREIGN KEY (user_id_low) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT fk_friendships_user_high
        FOREIGN KEY (user_id_high) REFERENCES users (id) ON DELETE CASCADE,
    -- 同时禁止自己加自己，并强制所有调用方遵守“小 ID 在前”的规范。
    CONSTRAINT chk_friendships_canonical_pair
        CHECK (user_id_low < user_id_high)
) ENGINE = InnoDB
  DEFAULT CHARACTER SET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;

-- 好友申请表：保存有方向的申请流程；WebSocket 只负责实时通知，不替代这里的持久化。
CREATE TABLE IF NOT EXISTS friend_requests (
    id BIGINT NOT NULL AUTO_INCREMENT,
    requester_id BIGINT NOT NULL,
    receiver_id BIGINT NOT NULL,
    -- 生成列把有方向的申请规范化成无方向用户对，用于阻止 A→B 与 B→A 重复存在。
    pair_user_id_low BIGINT GENERATED ALWAYS AS (LEAST(requester_id, receiver_id)) STORED,
    pair_user_id_high BIGINT GENERATED ALWAYS AS (GREATEST(requester_id, receiver_id)) STORED,
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    -- 申请尚未处理时为 NULL；接受或拒绝时由业务层写入处理时间。
    responded_at TIMESTAMP NULL DEFAULT NULL,
    PRIMARY KEY (id),
    -- 一对用户只维护一条申请生命周期；拒绝后重新申请时更新这条记录，而不是无限插入。
    UNIQUE KEY uk_friend_requests_pair (pair_user_id_low, pair_user_id_high),
    -- 收件箱和发件箱都按状态、时间查询，因此分别建立联合索引。
    KEY idx_friend_requests_receiver_status_created (receiver_id, status, created_at),
    KEY idx_friend_requests_requester_status_created (requester_id, status, created_at),
    -- requester/receiver 是生成列的基础列，MySQL 不允许这里使用 ON DELETE CASCADE。
    -- 当前项目不硬删除用户；默认 RESTRICT 还能避免申请记录变成没有用户的孤儿数据。
    CONSTRAINT fk_friend_requests_requester
        FOREIGN KEY (requester_id) REFERENCES users (id),
    CONSTRAINT fk_friend_requests_receiver
        FOREIGN KEY (receiver_id) REFERENCES users (id),
    CONSTRAINT chk_friend_requests_different_users
        CHECK (requester_id <> receiver_id),
    CONSTRAINT chk_friend_requests_status
        CHECK (status IN ('pending', 'accepted', 'rejected', 'cancelled'))
) ENGINE = InnoDB
  DEFAULT CHARACTER SET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;
