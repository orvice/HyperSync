package http

import (
	"github.com/gin-gonic/gin"
	"go.orx.me/apps/hyper-sync/internal/handler"
	"go.orx.me/apps/hyper-sync/internal/wire"
)

func Router(r *gin.Engine) {

	// Routes
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "pong",
		})
	})

	// API routes
	api := r.Group("/api")
	{
		// Token management routes
		tokenRoutes := api.Group("/token")
		{
			// 初始化 SchedulerService 和 TokenHandler
			schedulerService, err := wire.NewSchedulerService()
			if err != nil {
				// 这里可能需要更好的错误处理，但现在简单返回
				panic(err)
			}

			tokenHandler := handler.NewTokenHandler(schedulerService)

			// GET /api/token/status/:platform - 获取平台token状态
			tokenRoutes.GET("/status/:platform", tokenHandler.GetTokenStatus)

			// POST /api/token/refresh/:platform - 手动刷新特定平台token
			tokenRoutes.POST("/refresh/:platform", tokenHandler.RefreshToken)

			// POST /api/token/refresh-all - 手动刷新所有平台token
			tokenRoutes.POST("/refresh-all", tokenHandler.RefreshAllTokens)
		}
	}
}
