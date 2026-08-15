package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/config"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/user"
	"github.com/golang-jwt/jwt/v5"
)

func TestGenerateCreatesValidToken(t *testing.T) {
	// 使用独立测试密钥，不能依赖本机 application.yml 中的 JWT 配置。
	const secret = "test-signing-secret"
	service := NewTokenService(config.JWTConfig{
		Issuer:           "rock-paper-scissors",
		Secret:           secret,
		ExpiresInMinutes: 60,
	})

	signedToken, err := service.Generate(user.User{ID: 7, Username: "chenxuan"})
	if err != nil {
		t.Fatal(err)
	}

	// 不调用项目 Parse，而是直接使用第三方库独立验证 Generate 的产物。
	claims := &Claims{}
	parsedToken, err := jwt.ParseWithClaims(
		signedToken,
		claims,
		func(_ *jwt.Token) (any, error) { return []byte(secret), nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer("rock-paper-scissors"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !parsedToken.Valid {
		t.Fatal("generated token is invalid")
	}
	if claims.UserID != 7 || claims.Username != "chenxuan" || claims.Subject != "7" {
		t.Errorf("claims = %+v", claims)
	}
	// 允许少量测试执行耗时，但令牌有效期应接近配置的 60 分钟。
	if claims.ExpiresAt == nil || time.Until(claims.ExpiresAt.Time) < 59*time.Minute {
		t.Errorf("unexpected expiration: %v", claims.ExpiresAt)
	}
}

func TestParseReturnsClaimsForValidToken(t *testing.T) {
	// 正向验证同一服务使用同一密钥时能够还原可信 Claims。
	service := NewTokenService(config.JWTConfig{
		Issuer:           "rock-paper-scissors",
		Secret:           "test-signing-secret",
		ExpiresInMinutes: 60,
	})

	signedToken, err := service.Generate(user.User{ID: 7, Username: "chenxuan"})
	if err != nil {
		t.Fatal(err)
	}

	claims, err := service.Parse(signedToken)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != 7 || claims.Username != "chenxuan" {
		t.Errorf("claims = %+v", claims)
	}
}

func TestParseRejectsTokenSignedWithDifferentSecret(t *testing.T) {
	// 签发器与校验器密钥不同，用来模拟伪造或由其他服务签发的 JWT。
	issuer := NewTokenService(config.JWTConfig{
		Issuer:           "rock-paper-scissors",
		Secret:           "first-secret",
		ExpiresInMinutes: 60,
	})
	verifier := NewTokenService(config.JWTConfig{
		Issuer:           "rock-paper-scissors",
		Secret:           "different-secret",
		ExpiresInMinutes: 60,
	})

	signedToken, err := issuer.Generate(user.User{ID: 7, Username: "chenxuan"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = verifier.Parse(signedToken)
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("error = %v, want ErrInvalidToken", err)
	}
}
