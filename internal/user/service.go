package user

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const (
	maxUsernameLength = 64 // 与 Handler 和数据库字段的用户名上限保持一致。
	minPasswordLength = 6  // 当前学习项目采用的最低密码字符数。
	maxPasswordBytes  = 72 // BCrypt 只处理前 72 字节，因此业务层提前拒绝超长密码。
)

// Service 负责用户注册和登录业务。
type Service struct {
	// 依赖接口而不是 *MySQLRepository，测试可以注入内存实现。
	repository Repository
}

// NewService 创建用户业务服务。
func NewService(repository Repository) *Service {
	// 构造函数注入让依赖关系明确，并避免 Service 内部自行连接数据库。
	return &Service{repository: repository}
}

// Register 校验注册信息，生成密码哈希并保存用户。
func (s *Service) Register(ctx context.Context, username, password string) (User, error) {
	// 用户名允许调用方误带首尾空白，进入业务规则前先规范化。
	username = strings.TrimSpace(username)
	// RuneCountInString 按 Unicode 字符计数，不会把一个中文字符当成多个字节。
	if username == "" || utf8.RuneCountInString(username) > maxUsernameLength {
		return User{}, ErrInvalidUsername
	}

	// 最小长度按字符判断；最大长度按字节判断，以符合 BCrypt 的 72 字节限制。
	if utf8.RuneCountInString(password) < minPasswordLength || len([]byte(password)) > maxPasswordBytes {
		return User{}, ErrInvalidPassword
	}

	// 保存前主动查重，能够返回更清晰的业务错误。
	// 数据库唯一索引仍然保留，用于防住并发注册造成的竞态条件。
	_, err := s.repository.FindByUsername(ctx, username)
	if err == nil {
		return User{}, ErrUsernameExists
	}
	// “没查到用户”正是允许继续注册的情况；其他数据库错误必须向上返回。
	if !errors.Is(err, ErrUserNotFound) {
		return User{}, fmt.Errorf("check username: %w", err)
	}

	// GenerateFromPassword 内部生成随机盐并执行有成本的 BCrypt 哈希。
	// 数据库只保存结果字符串，不保存明文密码。
	passwordHash, err := bcrypt.GenerateFromPassword(
		[]byte(password),
		bcrypt.DefaultCost,
	)
	if err != nil {
		return User{}, fmt.Errorf("hash password: %w", err)
	}

	// 业务校验和密码哈希全部完成后，才构造 User 交给持久层。
	createdUser, err := s.repository.Create(ctx, User{
		Username:     username,
		PasswordHash: string(passwordHash),
	})
	if err != nil {
		return User{}, err
	}

	return createdUser, nil
}

// Login 验证用户名和密码，成功时返回对应用户。
func (s *Service) Login(ctx context.Context, username, password string) (User, error) {
	// 与注册时保持相同的用户名规范化规则。
	username = strings.TrimSpace(username)

	// 登录必须取回 PasswordHash，供后续 BCrypt 比对。
	foundUser, err := s.repository.FindByUsername(ctx, username)
	if err != nil {
		// 用户不存在与密码错误统一返回 ErrInvalidCredentials，避免账号枚举。
		if errors.Is(err, ErrUserNotFound) {
			return User{}, ErrInvalidCredentials
		}
		return User{}, fmt.Errorf("find login user: %w", err)
	}

	// CompareHashAndPassword 从哈希字符串读取算法参数和盐，验证待登录密码。
	// 它比较的是哈希结果，不会也不能把数据库哈希“解密”为原密码。
	if err := bcrypt.CompareHashAndPassword(
		[]byte(foundUser.PasswordHash),
		[]byte(password),
	); err != nil {
		return User{}, ErrInvalidCredentials
	}

	// 密码验证通过后返回用户，Handler 才允许为其签发 JWT。
	return foundUser, nil
}

// GetByID 获取数据库中最新的用户信息。
func (s *Service) GetByID(ctx context.Context, id int64) (User, error) {
	// /me 使用 JWT 中已验证的用户 ID 查询数据库中的最新资料。
	foundUser, err := s.repository.FindByID(ctx, id)
	if err != nil {
		return User{}, fmt.Errorf("get user by id: %w", err)
	}

	return foundUser, nil
}

// GetByIDs 批量查询用户并按主键建立 Map，便于调用方以 O(1) 时间组装集合结果。
func (s *Service) GetByIDs(ctx context.Context, ids []int64) (map[int64]User, error) {
	for _, id := range ids {
		if id <= 0 {
			return nil, ErrUserNotFound
		}
	}

	foundUsers, err := s.repository.FindByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("get users by ids: %w", err)
	}
	usersByID := make(map[int64]User, len(foundUsers))
	for _, foundUser := range foundUsers {
		usersByID[foundUser.ID] = foundUser
	}
	return usersByID, nil
}
