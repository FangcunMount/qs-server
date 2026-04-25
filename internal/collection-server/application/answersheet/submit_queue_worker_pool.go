package answersheet

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
		resp, err := p.submit(job.ctx, job.requestID, job.writerID, job.req)
		if err != nil {
			p.writeStatus(job.requestID, SubmitStatusFailed, "")
			continue
		}
		if resp != nil {
			p.writeStatus(job.requestID, SubmitStatusDone, resp.ID)
		}
	}
}
