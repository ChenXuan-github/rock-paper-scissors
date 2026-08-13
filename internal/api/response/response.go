package response

const (
	CodeSuccess        = 200
	CodeInvalidRequest = 400
	CodeInternalError  = 500
)

// Response 是所有 HTTP 接口统一使用的响应结构。
type Response[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

// Success 创建成功响应。
func Success[T any](data T) Response[T] {
	return Response[T]{
		Code:    CodeSuccess,
		Message: "success",
		Data:    data,
	}
}

// Error 创建不包含业务数据的失败响应。
func Error(code int, message string) Response[any] {
	return Response[any]{
		Code:    code,
		Message: message,
		Data:    nil,
	}
}
