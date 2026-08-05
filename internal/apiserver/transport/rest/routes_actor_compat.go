package rest

import (
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var deprecatedPractitionerRouteTotal = promauto.NewCounter(prometheus.CounterOpts{
	Namespace: "qs",
	Subsystem: "actor",
	Name:      "deprecated_practitioner_route_total",
	Help:      "Requests served through the deprecated /api/v1/practitioners compatibility route.",
})

func observeDeprecatedPractitionerRoute(c *gin.Context) {
	deprecatedPractitionerRouteTotal.Inc()
	c.Next()
}
