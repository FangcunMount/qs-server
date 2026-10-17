package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"github.com/FangcunMount/qs-server/pkg/version"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var definitionWriteTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "qs_model_definition_write_total",
	Help: "REST Definition writes classified from original input before application materialization.",
}, []string{"input", "result"})
var definitionInputFieldTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "qs_model_definition_input_field_total",
	Help: "Original REST Definition derived field presence including null and empty values.",
}, []string{"field", "presence", "result"})

func init() {
	for _, result := range []string{"accepted", "rejected"} {
		for _, input := range []string{"canonical", "derived", "unreadable"} {
			definitionWriteTotal.WithLabelValues(input, result)
		}
		for _, field := range []string{"DecisionSpec", "InterpretationAssets"} {
			for _, presence := range []string{"present", "absent", "unreadable"} {
				definitionInputFieldTotal.WithLabelValues(field, presence, result)
			}
		}
	}
	promauto.NewGauge(prometheus.GaugeOpts{
		Name:        "qs_definition_compatibility_observation_info",
		Help:        "REST Definition input observation capability version 1 and compiled source SHA.",
		ConstLabels: prometheus.Labels{"source_sha": version.Get().GitCommit, "capability": "1"},
	}).Set(1)
}

type definitionObservationBody struct {
	io.Reader
	io.Closer
}

// Tee the existing binder's reads, preserving its errors and consumed bytes. Only
// this REST write observes presence; domain decoding and persistence reads do not.
func observeDefinitionWrite(c *gin.Context) func() {
	original := c.Request.Body
	var captured bytes.Buffer
	if original != nil {
		c.Request.Body = definitionObservationBody{Reader: io.TeeReader(original, &captured), Closer: original}
	}
	return func() {
		c.Request.Body = original
		presence := [2]string{"unreadable", "unreadable"}
		input := "unreadable"
		var fields map[string]json.RawMessage
		// Match the binder's first-value decode, including its existing tolerance
		// of trailing bytes, rather than changing the input acceptance contract.
		if json.NewDecoder(bytes.NewReader(captured.Bytes())).Decode(&fields) == nil {
			presence = [2]string{"absent", "absent"}
			input = "canonical"
			for key := range fields {
				for index, field := range []string{"DecisionSpec", "InterpretationAssets"} {
					if strings.EqualFold(key, field) {
						presence[index] = "present"
						input = "derived"
					}
				}
			}
		}
		result := "rejected"
		if c.Writer.Status() >= 200 && c.Writer.Status() < 300 {
			result = "accepted"
		}
		definitionWriteTotal.WithLabelValues(input, result).Inc()
		for index, field := range []string{"DecisionSpec", "InterpretationAssets"} {
			definitionInputFieldTotal.WithLabelValues(field, presence[index], result).Inc()
		}
	}
}
