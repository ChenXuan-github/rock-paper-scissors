package user

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// memoryRepository 是 Repository 的内存测试实现，不连接也不污染真实 MySQL。
type memoryRepository struct {
	users  map[string]User
	nextID int64
}

func newMemoryRepository() *memoryRepository {
	// 每个测试获得独立 map 和从 1 开始的模拟自增主键。
	return &memoryRepository{
		users:  make(map[string]User),
		nextID: 1,
	}
}

func (r *memoryRepository) Create(_ context.Context, user User) (User, error) {
	// 下划线忽略该测试替身当前不需要使用的 Context 参数。
	if _, exists := r.users[user.Username]; exists {
		return User{}, ErrUsernameExists
	}

	user.ID = r.nextID
	r.nextID++
	r.users[user.Username] = user
	return user, nil
}

func (r *memoryRepository) FindByUsername(_ context.Context, username string) (User, error) {
	user, exists := r.users[username]
	if !exists {
		return User{}, ErrUserNotFound
	}
	return user, nil
}

func (r *memoryRepository) FindByID(_ context.Context, id int64) (User, error) {
	// map 以用户名为键，因此测试实现通过遍历模拟按 ID 查询。
	for _, user := range r.users {
		if user.ID == id {
			return user, nil
		}
	}
	return User{}, ErrUserNotFound
}

func TestRegisterHashesPassword(t *testing.T) {
	// 通过接口注入 memoryRepository，测试只关注 Service 业务规则。
	repository := newMemoryRepository()
	service := NewService(repository)

	registeredUser, err := service.Register(context.Background(), "chenxuan", "password123")
	if err != nil {
		t.Fatal(err)
	}

	// 第一层断言：数据库模型中绝不能出现原始明文。
	if registeredUser.PasswordHash == "password123" {
		t.Fatal("password was stored as plain text")
	}

	// 第二层断言：生成的哈希必须能通过 BCrypt 验证原密码。
	if err := bcrypt.CompareHashAndPassword(
		[]byte(registeredUser.PasswordHash),
		[]byte("password123"),
	); err != nil {
		t.Fatalf("stored password hash does not match original password: %v", err)
	}
}

func TestRegisterRejectsDuplicateUsername(t *testing.T) {
	repository := newMemoryRepository()
	service := NewService(repository)

	if _, err := service.Register(context.Background(), "chenxuan", "password123"); err != nil {
		t.Fatal(err)
	}

	_, err := service.Register(context.Background(), "chenxuan", "another-password")
	if !errors.Is(err, ErrUsernameExists) {
		t.Fatalf("Register() error = %v, want %v", err, ErrUsernameExists)
	}
}

func TestLoginAcceptsCorrectPassword(t *testing.T) {
	repository := newMemoryRepository()
	service := NewService(repository)

	registeredUser, err := service.Register(context.Background(), "chenxuan", "password123")
	if err != nil {
		t.Fatal(err)
	}

	loggedInUser, err := service.Login(context.Background(), "chenxuan", "password123")
	if err != nil {
		t.Fatal(err)
	}

	if loggedInUser.ID != registeredUser.ID {
		t.Errorf("logged-in user ID = %d, want %d", loggedInUser.ID, registeredUser.ID)
	}
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	repository := newMemoryRepository()
	service := NewService(repository)

	if _, err := service.Register(context.Background(), "chenxuan", "password123"); err != nil {
		t.Fatal(err)
	}

	_, err := service.Login(context.Background(), "chenxuan", "wrong-password")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Login() error = %v, want %v", err, ErrInvalidCredentials)
	}
}

func TestLoginDoesNotRevealMissingUser(t *testing.T) {
	service := NewService(newMemoryRepository())

	_, err := service.Login(context.Background(), "missing-user", "password123")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Login() error = %v, want %v", err, ErrInvalidCredentials)
	}
}

func TestGetByIDReturnsUser(t *testing.T) {
	repository := newMemoryRepository()
	service := NewService(repository)

	registeredUser, err := service.Register(context.Background(), "chenxuan", "password123")
	if err != nil {
		t.Fatal(err)
	}

	foundUser, err := service.GetByID(context.Background(), registeredUser.ID)
	if err != nil {
		t.Fatal(err)
	}
	if foundUser.Username != registeredUser.Username {
		t.Errorf("username = %q, want %q", foundUser.Username, registeredUser.Username)
	}
}
