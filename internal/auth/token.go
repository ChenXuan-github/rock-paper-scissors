package auth

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/config"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/user"
	"github.com/golang-jwt/jwt/v5"
)

// ErrInvalidToken 是统一的 JWT 校验失败错误。
// 不向客户端区分“签名错误”“已经过期”等细节，避免暴露鉴权信息。
var ErrInvalidToken = errors.New("invalid token")

// Claims 表示 JWT Payload 中保存的数据。
type Claims struct {
	// UserID 是项目内部真正用来确定当前用户身份的数据库主键。
	UserID int64 `json:"userId"`
	// Username 是签发令牌时的用户名快照。
	Username string `json:"username"`
	// RegisteredClaims 嵌入 JWT 标准声明，例如 iss、sub、iat 和 exp。
	jwt.RegisteredClaims
}

// TokenService 负责签发和校验 JWT。
type TokenService struct {
	// secret 是 HS256 的对称密钥，签发和校验必须使用同一份密钥。
	secret []byte
	// issuer 是预期的 JWT 签发者。
	issuer string
	// lifetime 表示 JWT 从签发到过期的有效时长。
	lifetime time.Duration
}

// NewTokenService 根据配置创建 JWT 服务。
func NewTokenService(config config.JWTConfig) *TokenService {
	return &TokenService{
		// JWT 库签名时需要 []byte，所以把配置字符串转换成字节切片。
		secret: []byte(config.Secret),
		// Generate 写入该签发者，Parse 也只接受该签发者。
		issuer: config.Issuer,
		// 把配置中的分钟数转换成 Go 的 time.Duration。
		lifetime: time.Duration(config.ExpiresInMinutes) * time.Minute,
	}
}

// Generate 为登录成功的用户生成有过期时间的 JWT。
func (s *TokenService) Generate(loginUser user.User) (string, error) {
	// 只获取一次当前时间，让 iat 和 exp 使用完全相同的时间基准。
	now := time.Now()

	// claims 将成为 JWT 中间一段 Payload 的内容。
	claims := Claims{
		// 自定义声明：保存登录用户的数据库主键。
		UserID: loginUser.ID,
		// 自定义声明：保存签发令牌时的用户名。
		Username: loginUser.Username,
		// RegisteredClaims 是 JWT 规范定义的标准声明。
		RegisteredClaims: jwt.RegisteredClaims{
			// iss：该令牌由当前猜拳服务签发。
			Issuer: s.issuer,
			// sub：令牌代表的主体；标准要求字符串，所以转换用户 ID。
			Subject: strconv.FormatInt(loginUser.ID, 10),
			// iat：令牌签发时间。
			IssuedAt: jwt.NewNumericDate(now),
			// exp：签发时间加有效时长，超过它以后令牌失效。
			ExpiresAt: jwt.NewNumericDate(now.Add(s.lifetime)),
		},
	}

	// 创建尚未签名的 Token，并明确使用 HMAC-SHA256 算法。
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	// 使用服务器密钥签名，生成 Header.Payload.Signature 三段式字符串。
	signedToken, err := token.SignedString(s.secret)
	if err != nil {
		// 包装底层错误，为日志补充“JWT 签名失败”的上下文。
		return "", fmt.Errorf("sign JWT: %w", err)
	}

	// 签名成功，把可以交给客户端的 JWT 返回给登录 Handler。
	return signedToken, nil
}

// Parse 校验 JWT 的算法、签名、签发者和过期时间，并返回其中的 Claims。
func (s *TokenService) Parse(signedToken string) (Claims, error) {
	// JWT 库会把解析出的 Payload 字段写进这个 Claims 对象。
	claims := &Claims{}

	// ParseWithClaims 同时完成字符串解析、签名验证和标准声明校验。
	parsedToken, err := jwt.ParseWithClaims(
		// 客户端在 Authorization Header 中携带的三段式 JWT。
		signedToken,
		// 指定 Payload 应当解析成项目自定义的 Claims 类型。
		claims,
		// JWT 库通过该回调取得校验签名所需的密钥。
		func(token *jwt.Token) (any, error) {
			// 只接受 HS256，防止攻击者替换 Header 中声明的 alg。
			if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
				return nil, ErrInvalidToken
			}
			// HS256 是对称算法，验证签名使用签发时的同一份 secret。
			return s.secret, nil
		},
		// 再用白名单限制允许的签名算法，形成双重约束。
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		// 检查 Payload 中的 iss 是否等于当前服务配置的 issuer。
		jwt.WithIssuer(s.issuer),
		// 要求 JWT 必须包含 exp，并校验当前时间是否已经超过 exp。
		jwt.WithExpirationRequired(),
		// 同时校验 iat，防止签发时间明显处于未来。
		jwt.WithIssuedAt(),
	)
	if err != nil {
		// 对外统一包装成 ErrInvalidToken，不暴露具体校验失败原因。
		return Claims{}, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	// 除库校验外，再检查 Token 状态、用户 ID，以及 sub 与 userId 是否一致。
	if !parsedToken.Valid || claims.UserID <= 0 ||
		claims.Subject != strconv.FormatInt(claims.UserID, 10) {
		return Claims{}, ErrInvalidToken
	}

	// 所有检查通过，返回已经可以信任的当前用户身份声明。
	return *claims, nil
}
