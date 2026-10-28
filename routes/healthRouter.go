package routes

import (
	"github.com/gin-gonic/gin"
	controller "github.com/ishanbagra18/ecommerce-using-go/controllers"
)

// HealthRoutes registers health checks, liveness, readiness, and runtime telemetry routes
func HealthRoutes(router *gin.Engine) {
	// Root-level health probes (standard for cloud orchestrators like K8s, Docker, AWS ALB)
	router.GET("/health", controller.HealthCheck())
	router.GET("/healthz", controller.HealthCheck())
	router.GET("/live", controller.LivenessCheck())
	router.GET("/livez", controller.LivenessCheck())
	router.GET("/ready", controller.ReadinessCheck())
	router.GET("/readyz", controller.ReadinessCheck())
	router.GET("/metrics", controller.RuntimeMetrics())

	// API grouped routes
	api := router.Group("/api/observability")
	{
		api.GET("/health", controller.HealthCheck())
		api.GET("/metrics", controller.RuntimeMetrics())
	}
}
