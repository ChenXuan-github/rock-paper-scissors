package user

import "time"

// User 表示系统中的一个已注册用户。
type User struct {
	ID           int64     // MySQL 自增主键，也是 JWT 中保存的用户身份 ID。
	Username     string    // 登录名，数据库唯一索引保证不可重复。
	PasswordHash string    // BCrypt 哈希，只用于验证密码，绝不能返回客户端。
	CreatedAt    time.Time // 数据库记录的账号创建时间。
}
