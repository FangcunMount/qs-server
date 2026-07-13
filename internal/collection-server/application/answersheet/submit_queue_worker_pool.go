package answersheet

import "github.com/FangcunMount/component-base/pkg/logger"

type submitQueueStatusWriter func(requestID, status, answerSheetID string)

// submitQueueWorkerPool owns the goroutines that drain submit jobs.
// SubmitQueue remains responsible for admission, status lookup, and observation.
type submitQueueWorkerPool struct {
	workerCount int
	jobs        <-chan submitJob
	submit      submitFunc
	writeStatus submitQueueStatusWriter
}

func newSubmitQueueWorkerPool(workerCount int, jobs <-chan submitJob, submit submitFunc, writeStatus submitQueueStatusWriter) *submitQueueWorkerPool {
	if workerCount <= 0 || jobs == nil || submit == nil || writeStatus == nil {
		return nil
	}
	return &submitQueueWorkerPool{
		workerCount: workerCount,
		jobs:        jobs,
		submit:      submit,
		writeStatus: writeStatus,
	}
}

func (p *submitQueueWorkerPool) Start() {
	if p == nil {
		return
	}
	for i := 0; i < p.workerCount; i++ {
		go p.worker()
	}
}

func (p *submitQueueWorkerPool) worker() {
	for job := range p.jobs {
		p.writeStatus(job.requestID, SubmitStatusProcessing, "")
		startFields := []interface{}{
			"action", "process_answersheet_submit",
			"request_id", job.requestID,
			"writer_id", job.writerID,
		}
		if job.req != nil {
			startFields = append(startFields,
				"testee_id", job.req.TesteeID,
				"questionnaire_code", job.req.QuestionnaireCode,
			)
		}
		logger.L(job.ctx).Infow("答卷提交队列开始处理", startFields...)
		resp, err := p.submit(job.ctx, job.requestID, job.writerID, job.req)
		if err != nil {
			p.writeStatus(job.requestID, SubmitStatusFailed, "")
			failureFields := []interface{}{
				"action", "process_answersheet_submit",
				"request_id", job.requestID,
				"error", err.Error(),
			}
			if job.req != nil {
				failureFields = append(failureFields,
					"testee_id", job.req.TesteeID,
					"questionnaire_code", job.req.QuestionnaireCode,
				)
			}
			logger.L(job.ctx).Errorw("答卷提交队列处理失败", failureFields...)
			continue
		}
		if resp != nil {
			p.writeStatus(job.requestID, SubmitStatusDone, resp.ID)
			logger.L(job.ctx).Infow("答卷提交队列处理完成",
				"action", "process_answersheet_submit",
				"request_id", job.requestID,
				"answersheet_id", resp.ID,
				"assessment_id", resp.AssessmentID,
				"result", "success",
			)
		}
	}
}
