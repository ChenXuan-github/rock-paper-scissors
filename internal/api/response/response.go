package response

const (
	// 当前项目让业务 code 与主要 HTTP 状态码保持一致，客户端更容易统一处理。
	CodeSuccess        = 200
	CodeInvalidRequest = 400
	CodeInternalError  = 500
)

// Response 是所有 HTTP 接口统一使用的响应结构。
// [T any] 表示 Data 可以是任意具体类型，同时仍保留编译期类型检查。
type Response[T any] struct {
	Code    int    `json:"code"`    // 业务结果码
	Message string `json:"message"` // 面向调用方的简要消息
	Data    T      `json:"data"`    // 接口真正返回的业务数据
}

// Success 创建成功响应。
func Success[T any](data T) Response[T] {
	// Go 会根据 data 推断 T，例如传入 loginResponse 时返回 Response[loginResponse]。
	return Response[T]{
		Code:    CodeSuccess,
		Message: "success",
		Data:    data,
	}
}

// Error 创建不包含业务数据的失败响应。
func Error(code int, message string) Response[any] {
	// 失败时没有固定数据类型，因此使用 any，并明确让 JSON data 为 null。
	return Response[any]{
		Code:    code,
		Message: message,
		Data:    nil,
	}
}
