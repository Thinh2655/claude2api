package main

import (
	"claude2api/config"
	"claude2api/core"
	"claude2api/logger"
	"claude2api/router"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()
	// Load configuration

	// Refresh the model list from claude.ai at startup and every 6h, so new
	// models appear in /v1/models without a code change.
	go refreshModelsLoop()

	// Setup all routes
	router.SetupRoutes(r)

	// Run the server on 0.0.0.0:8080
	r.Run(config.ConfigInstance.Address)
}

// refreshModelsLoop fetches the web-ui model list from a healthy session at
// startup and then every 6h. It only needs one working account; the model list
// is per-account-tier, and sessions share the tier here.
func refreshModelsLoop() {
	fetch := func() {
		config.ConfigInstance.RwMutx.RLock()
		sessions := make([]config.SessionInfo, 0, len(config.ConfigInstance.Sessions))
		sessions = append(sessions, config.ConfigInstance.Sessions...)
		config.ConfigInstance.RwMutx.RUnlock()
		for _, s := range sessions {
			if core.FetchBootstrapModels(s.SessionKey, s.ExtraCookie, s.DeviceID) != nil {
				return
			}
		}
		logger.Info("No session could supply the model list; using the built-in list")
	}
	fetch()
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		fetch()
	}
}
