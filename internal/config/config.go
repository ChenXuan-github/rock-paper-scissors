package config

import (
	"os"

	"github.com/goccy/go-yaml"
)

// Config 是 application.yml 的根配置对象，由 main 在启动时一次性加载。
type Config struct {
	// yaml 标签把结构体字段映射到 application.yml 中对应的配置块。
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Redis    RedisConfig    `yaml:"redis"`
	// JWT 保存令牌签发和校验共同使用的配置。
	JWT JWTConfig `yaml:"jwt"`
}

// RedisConfig 保存 Redis 单机连接参数。
// Password 为空字符串表示本机 Redis 没有配置密码。
type RedisConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Password string `yaml:"password"`
	Database int    `yaml:"database"`
}

// ServerConfig 保存 HTTP 服务启动配置。
type ServerConfig struct {
	Port int `yaml:"port"`
}

// DatabaseConfig 保存连接 MySQL 所需的基础参数。
type DatabaseConfig struct {
	Driver   string `yaml:"driver"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Name     string `yaml:"name"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// JWTConfig 对应 application.yml 中的 jwt 配置块。
type JWTConfig struct {
	// Issuer 会写入 iss，并在解析时校验签发者是否一致。
	Issuer string `yaml:"issuer"`
	// Secret 是 HS256 签发和验证共同使用的对称密钥。
	Secret string `yaml:"secret"`
	// ExpiresInMinutes 控制 JWT 从签发开始可以使用多少分钟。
	ExpiresInMinutes int `yaml:"expiresInMinutes"`
}

// Load 从 YAML 文件读取项目配置。
func Load(path string) (Config, error) {
	// ReadFile 一次性读取配置文件；路径不存在或无权限时立即返回错误。
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	// 零值 Config 作为 YAML 反序列化目标。
	var config Config
	// Unmarshal 根据 yaml 标签把文本填充到嵌套结构体。
	if err := yaml.Unmarshal(data, &config); err != nil {
		return Config{}, err
	}

	// 返回值类型为 (Config, error)，nil 表示读取和解析均成功。
	return config, nil
}
