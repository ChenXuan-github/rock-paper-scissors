package user

import "errors"

var (
	// 领域错误让 Service 和 Handler 不需要识别 MySQL 错误码或 SQL 库细节。
	ErrUserNotFound    = errors.New("user not found")
	ErrUsernameExists  = errors.New("username already exists")
	ErrInvalidUsername = errors.New("invalid username")
	ErrInvalidPassword = errors.New("invalid password")
	// 登录失败统一使用同一个错误，避免泄露“用户名存在但密码错误”等账号信息。
	ErrInvalidCredentials = errors.New("invalid username or password")
)
