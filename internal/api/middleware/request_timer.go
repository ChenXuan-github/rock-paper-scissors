package middleware

import (
	"log"
	"time"

	"github.com/gin-gonic/gin"
)

// RequestTimer 记录每次 HTTP 请求的处理耗时。
func RequestTimer() gin.HandlerFunc {
	// 中间件工厂返回 gin.HandlerFunc，Router 通过 Use 把它挂到请求链上。
	return func(c *gin.Context) {
		// c.Next() 之前属于前置处理，先记录请求开始时间。
		start := time.Now()

		// 暂停当前中间件，执行后续中间件和最终 Handler。
		// Handler 完成后会回到下一行，因此形成类似环绕 AOP 的结构。
		c.Next()

		// c.Next() 之后属于后置处理，此时状态码和总耗时都已确定。
		duration := time.Since(start)
		// 记录方法、路径、状态码和耗时，便于观察接口行为。
		log.Printf(
			"method=%s path=%s status=%d duration=%s",
			c.Request.Method,
			c.Request.URL.Path,
			c.Writer.Status(),
			duration,
		)
	}
}
