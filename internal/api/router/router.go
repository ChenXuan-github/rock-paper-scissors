package router

import (
	"github.com/ChenXuan-github/rock-paper-scissors/internal/api/handler"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/api/middleware"
	"github.com/gin-gonic/gin"
)

// New 创建并配置 HTTP 路由器。
func New() *gin.Engine {
	r := gin.Default()
	r.Use(middleware.RequestTimer())

	api := r.Group("/api/v1")
	rounds := api.Group("/rounds")
	rounds.POST("/evaluate", handler.EvaluateRound)

	return r
}
