package router

import (
	"claude2api/config"
	"claude2api/core"
	"claude2api/middleware"
	"claude2api/service"

	"github.com/gin-gonic/gin"
)

func SetupRoutes(r *gin.Engine) {
	// Apply middleware
	r.Use(middleware.CORSMiddleware())

	// Health check endpoint
	r.GET("/health", service.HealthCheckHandler)

	// Key management UI + API
	r.GET("/keys", service.KeysPageHandler)
	authedKeys := r.Group("/keys/api", middleware.AuthMiddleware())
	{
		authedKeys.GET("", service.KeysListHandler)
		authedKeys.POST("/add", service.KeyAddHandler)
		authedKeys.POST("/delete", service.KeyDeleteHandler)
		authedKeys.GET("/gateway", service.GatewayKeyGetHandler)
		authedKeys.POST("/gateway", service.GatewayKeySetHandler)
		authedKeys.POST("/check", service.KeyCheckHandler)
	}

	// API routes use auth middleware per-group so the gateway can stay open
	authed := r.Group("/", middleware.AuthMiddleware())
	{
		// Chat completions endpoint (OpenAI-compatible)
		authed.POST("/v1/chat/completions", service.ChatCompletionsHandler)
		authed.GET("/v1/models", service.MoudlesHandler)

		if config.ConfigInstance.EnableMirrorApi {
			authed.POST(config.ConfigInstance.MirrorApiPrefix+"/v1/chat/completions", service.MirrorChatHandler)
			authed.GET(config.ConfigInstance.MirrorApiPrefix+"/v1/models", service.MoudlesHandler)
		}

		// HuggingFace compatible routes
		authed.POST("/hf/v1/chat/completions", service.ChatCompletionsHandler)
		authed.GET("/hf/v1/models", service.MoudlesHandler)
	}

	// Gateway: serve the real claude.ai web UI on localhost. Mounted as a
	// no-route fallback AFTER the API routes, like rust-chat2api does.
	if config.ConfigInstance.EnableGateway {
		gateway := core.NewGatewayHandler()
		r.NoRoute(func(c *gin.Context) {
			gateway.ServeHTTP(c.Writer, c.Request)
		})
	} else {
		r.NoRoute(middleware.AuthMiddleware(), func(c *gin.Context) {
			c.JSON(404, gin.H{"error": "Not found"})
		})
	}
}
