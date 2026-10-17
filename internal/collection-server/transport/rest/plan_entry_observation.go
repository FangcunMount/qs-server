package rest

import (
	"net/http"

	"github.com/FangcunMount/qs-server/pkg/version"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var planEntryRouteTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "qs_plan_entry_route_total",
	Help: "Matched GET task entry routes including authentication and authorization rejection.",
}, []string{"route", "result"})

func init() {
	for _, route := range []string{"canonical", "legacy_token"} {
		for _, result := range []string{"accepted", "rejected"} {
			planEntryRouteTotal.WithLabelValues(route, result)
		}
	}
	promauto.NewGauge(prometheus.GaugeOpts{
		Name:        "qs_plan_entry_compatibility_observation_info",
		Help:        "Task entry route observation capability version 1 and compiled source SHA.",
		ConstLabels: prometheus.Labels{"source_sha": version.Get().GitCommit, "capability": "1"},
	}).Set(1)
}

// Installed before IAM so an old link rejected by authentication is still observed.
// FullPath is the registered template; neither task IDs nor tokens enter labels.
func observePlanEntryRoutes() gin.HandlerFunc {
	return func(c *gin.Context) {
		route := ""
		if c.Request.Method == http.MethodGet {
			switch c.FullPath() {
			case "/api/v1/plan-task-entries/:task_id":
				route = "canonical"
			case "/api/v1/plan-task-entries/:task_id/:token":
				route = "legacy_token"
			}
		}
		c.Next()
		if route != "" {
			result := "rejected"
			if c.Writer.Status() >= 200 && c.Writer.Status() < 300 {
				result = "accepted"
			}
			planEntryRouteTotal.WithLabelValues(route, result).Inc()
		}
	}
}
