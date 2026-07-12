package ws

// ReportEventsSubscribeFrame is the inbound subscribe message on WSS /api/v1/report-events.
type ReportEventsSubscribeFrame struct {
	Op           string `json:"op" example:"subscribe"`
	AssessmentID string `json:"assessment_id" example:"8001"`
	// 测评层 kind；人格线为 personality，量表线为 medical。
	Kind     string `json:"kind" example:"personality" enums:"personality,medical"`
	TesteeID string `json:"testee_id" example:"618855887087350318"`
}

// ReportEventsStatusFrame is a server push status frame after subscribe.
type ReportEventsStatusFrame struct {
	Op   string `json:"op" example:"status"`
	Data struct {
		Status string `json:"status" example:"interpreted" enums:"processing,interpreted,failed"`
	} `json:"data"`
}

// ReportEventsWebSocket documents WSS /api/v1/report-events for OpenAPI consumers.
// @Summary 报告状态 WebSocket 推送
// @Description 升级 WebSocket 后发送 subscribe 帧等待测评终态。每条连接仅允许一次 subscribe。人格线 kind=personality；量表线 kind=medical。需 report_events.enabled=true。
// @Tags 报告事件
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Success 101 {object} ReportEventsStatusFrame "Switching Protocols"
// @Failure 400 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse "report_events 未开启"
// @Router /api/v1/report-events [get]
func ReportEventsWebSocket() {}
