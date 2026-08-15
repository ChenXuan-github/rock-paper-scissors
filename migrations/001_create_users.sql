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
