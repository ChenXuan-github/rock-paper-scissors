package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	// t.TempDir 为当前测试创建隔离临时目录，测试结束后由 Go 自动清理。
	path := filepath.Join(t.TempDir(), "application.yml")
	// 构造一份完整但不含真实生产凭证的 YAML 测试配置。
	content := []byte(
		"server:\n" +
			"  port: 9090\n" +
			"database:\n" +
			"  driver: mysql\n" +
			"  host: 127.0.0.1\n" +
			"  port: 3306\n" +
			"  name: rock_paper_scissors\n" +
			"  username: root\n" +
			"  password: test-password\n" +
			"jwt:\n" +
			"  issuer: rock-paper-scissors\n" +
			"  secret: test-jwt-secret\n" +
			"  expiresInMinutes: 60\n",
	)

	// 0600 表示只有当前用户可读写，适合可能包含凭证的配置文件。
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}

	config, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if config.Server.Port != 9090 {
		t.Errorf("server port = %d, want 9090", config.Server.Port)
	}

	if config.Database.Name != "rock_paper_scissors" {
		t.Errorf("database name = %q, want %q", config.Database.Name, "rock_paper_scissors")
	}

	if config.Database.Password != "test-password" {
		t.Errorf("database password = %q, want %q", config.Database.Password, "test-password")
	}

	if config.JWT.Issuer != "rock-paper-scissors" || config.JWT.ExpiresInMinutes != 60 {
		t.Errorf("jwt config = %+v", config.JWT)
	}
}
