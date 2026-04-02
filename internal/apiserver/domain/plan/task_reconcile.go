package plan

import "sort"

// EnrollmentTasksResult 描述 enrollment 计算结果。
type EnrollmentTasksResult struct {
	Tasks       []*AssessmentTask
	TasksToSave []*AssessmentTask
	Idempotent  bool
}

// ResumeTasksResult 描述 resume 需要落库的任务集合。
type ResumeTasksResult struct {
	TasksToSave []*AssessmentTask
}

func groupTasksBySeq(tasks []*AssessmentTask) map[int][]*AssessmentTask {
	grouped := make(map[int][]*AssessmentTask)
	for _, task := range tasks {
		grouped[task.GetSeq()] = append(grouped[task.GetSeq()], task)
	}
	return grouped
}

func preferredTask(tasks []*AssessmentTask) *AssessmentTask {
	var best *AssessmentTask
	for _, task := range tasks {
		if task == nil {
			continue
		}
		if best == nil || compareTaskPriority(task, best) > 0 {
			best = task
		}
	}
	return best
}

func compareTaskPriority(left, right *AssessmentTask) int {
	if left == nil && right == nil {
		return 0
	}
	if left == nil {
		return -1
	}
	if right == nil {
		return 1
	}

	leftRank := taskStatusRank(left.GetStatus())
	rightRank := taskStatusRank(right.GetStatus())
	if leftRank != rightRank {
		return leftRank - rightRank
	}

	switch {
	case left.GetID() > right.GetID():
		return 1
	case left.GetID() < right.GetID():
		return -1
	default:
		return 0
	}
}

func taskStatusRank(status TaskStatus) int {
	switch status {
	case TaskStatusCompleted:
		return 5
	case TaskStatusOpened:
		return 4
	case TaskStatusPending:
		return 3
	case TaskStatusExpired:
		return 2
	case TaskStatusCanceled:
		return 1
	default:
		return 0
	}
}

func taskMatchesExpectedSchedule(actual, expected *AssessmentTask) bool {
	if actual == nil || expected == nil {
		return false
	}
	if actual.GetPlanID() != expected.GetPlanID() {
		return false
	}
	if actual.GetTesteeID() != expected.GetTesteeID() {
		return false
	}
	if actual.GetSeq() != expected.GetSeq() {
		return false
	}
	if actual.GetOrgID() != expected.GetOrgID() {
		return false
	}
	if actual.GetScaleCode() != expected.GetScaleCode() {
		return false
	}
	return actual.GetPlannedAt().Equal(expected.GetPlannedAt())
}

func sortTasksBySeq(tasks []*AssessmentTask) {
	sort.Slice(tasks, func(i, j int) bool {
		if tasks[i].GetSeq() != tasks[j].GetSeq() {
			return tasks[i].GetSeq() < tasks[j].GetSeq()
		}
		return tasks[i].GetID() < tasks[j].GetID()
	})
}
