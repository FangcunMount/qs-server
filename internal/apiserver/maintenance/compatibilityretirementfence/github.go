package compatibilityretirementfence

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"sort"
	"strings"
)

// HTTPDoer is borrowed. The host provides read-only GitHub authentication and
// lifecycle. Only fixed GET endpoints are used; credentials/errors are not returned.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type githubWorkflow struct {
	ID    int64  `json:"id"`
	Path  string `json:"path"`
	State string `json:"state"`
	Name  string `json:"name"`
}
type githubRepository struct {
	ID       int64  `json:"id"`
	FullName string `json:"full_name"`
	Owner    struct {
		ID int64 `json:"id"`
	} `json:"owner"`
}
type githubRun struct {
	ID             int64            `json:"id"`
	RunAttempt     int64            `json:"run_attempt"`
	WorkflowID     int64            `json:"workflow_id"`
	HeadSHA        string           `json:"head_sha"`
	HeadBranch     string           `json:"head_branch"`
	Event          string           `json:"event"`
	Status         string           `json:"status"`
	Path           string           `json:"path"`
	Repository     githubRepository `json:"repository"`
	HeadRepository githubRepository `json:"head_repository"`
	Actor          struct {
		ID int64 `json:"id"`
	} `json:"actor"`
	TriggeringActor struct {
		ID int64 `json:"id"`
	} `json:"triggering_actor"`
}
type githubJob struct {
	ID          int64  `json:"id"`
	RunID       int64  `json:"run_id"`
	HeadSHA     string `json:"head_sha"`
	Status      string `json:"status"`
	CheckRunURL string `json:"check_run_url"`
	RunnerID    int64  `json:"runner_id"`
}
type remoteSnapshot struct {
	Digest        string
	WorkflowCount int
	QueueRows     int
}

var historicalWorkflowIDs = []int64{368629149, 372424442, 373553659, 373620355, 373624776, 373634423, 373838613, 373874329, 373875487}
var currentWorkflowPaths = []string{"cd.yml", "db-ops.yml", "authz-production-matrix-provision.yml", "authz-production-matrix.yml", "compatibility-observation.yml", "attention-reconcile-audit.yml", "m6-qs03-gap-readonly.yml", "m5-authz-outage-preflight.yml", "m5-authz-ephemeral-postcheck.yml", "ping-runner.yml", "reliable-messaging-m6-qs-image-handoff.yml", "compatibility-retirement.yml"}

func getJSON(ctx context.Context, client HTTPDoer, address string, dst any) ([]byte, error) {
	if client == nil || nilDoer(client) {
		return nil, ErrRemote
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, ErrRemote
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	response, err := client.Do(req)
	if err != nil || response == nil {
		return nil, ErrRemote
	}
	if response.Body == nil {
		return nil, ErrRemote
	}
	// Body belongs to this request, not the borrowed HTTP transport. Its cleanup
	// cannot replace the fixed read failure category or successful decoding.
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK || response.Request == nil || response.Request.URL.String() != address {
		return nil, ErrRemote
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, MaxBodyBytes+1))
	if err != nil || len(body) > MaxBodyBytes || decodeJSON(body, dst) != nil {
		return nil, ErrRemote
	}
	return body, nil
}

func readSnapshot(ctx context.Context, client HTTPDoer, p Policy) (remoteSnapshot, error) {
	var result remoteSnapshot
	var hashes []string
	record := func(address string, v any) error {
		b, e := getJSON(ctx, client, address, v)
		if e == nil {
			hashes = append(hashes, address+" "+digest(b))
		}
		return e
	}
	base := APIBase + "/repos/" + Repository
	var repo githubRepository
	if err := record(base, &repo); err != nil {
		return result, err
	}
	if canonicalInteger(repo.ID) != p.RepositoryID || repo.FullName != Repository || canonicalInteger(repo.Owner.ID) != p.OwnerID {
		return result, ErrIdentity
	}
	var commit struct {
		SHA string `json:"sha"`
	}
	if err := record(base+"/commits/main", &commit); err != nil {
		return result, err
	}
	if commit.SHA != p.SourceSHA {
		return result, ErrIdentity
	}
	var run githubRun
	if err := record(base+"/actions/runs/"+p.RunID+"/attempts/"+p.RunAttempt, &run); err != nil {
		return result, err
	}
	if canonicalInteger(run.ID) != p.RunID || canonicalInteger(run.RunAttempt) != p.RunAttempt || run.WorkflowID != p.WorkflowID || run.HeadSHA != p.SourceSHA || run.HeadBranch != "main" || run.Event != "workflow_dispatch" || run.Status != "in_progress" || run.Path != WorkflowPath || canonicalInteger(run.Repository.ID) != p.RepositoryID || canonicalInteger(run.HeadRepository.ID) != p.RepositoryID || run.Repository.FullName != Repository || run.HeadRepository.FullName != Repository || canonicalInteger(run.Actor.ID) != p.ActorID || canonicalInteger(run.TriggeringActor.ID) != p.ActorID {
		return result, ErrIdentity
	}
	workflows := map[int64]githubWorkflow{}
	paths := map[string]bool{}
	total := -1
	for page := 1; page <= MaxPages; page++ {
		var list struct {
			Total int              `json:"total_count"`
			Rows  []githubWorkflow `json:"workflows"`
		}
		if err := record(fmt.Sprintf("%s/actions/workflows?per_page=100&page=%d", base, page), &list); err != nil {
			return result, err
		}
		if total < 0 {
			total = list.Total
		}
		if total != list.Total || total < 1 || total > 1000 || len(list.Rows) > 100 {
			return result, ErrCoverage
		}
		for _, w := range list.Rows {
			if w.ID <= 0 || w.Path == "" || w.State == "" || workflows[w.ID].ID != 0 {
				return result, ErrCoverage
			}
			workflows[w.ID] = w
			paths[w.Path] = true
		}
		if len(list.Rows) < 100 {
			break
		}
		if page == MaxPages {
			return result, ErrCoverage
		}
	}
	if len(workflows) != total || total != len(p.WorkflowIDs) {
		return result, ErrCoverage
	}
	for _, id := range p.WorkflowIDs {
		if workflows[id].ID != id {
			return result, ErrCoverage
		}
	}
	if workflows[p.WorkflowID].Path != WorkflowPath || workflows[p.WorkflowID].State != "active" {
		return result, ErrIdentity
	}
	for _, path := range currentWorkflowPaths {
		if !paths[".github/workflows/"+path] {
			return result, ErrCoverage
		}
	}
	// Read every nonterminal category independently through actual EOF. A pause
	// variable or an empty first page cannot substitute for this enumeration.
	for _, status := range []string{"queued", "in_progress", "waiting", "pending", "requested"} {
		seen := map[int64]bool{}
		count := -1
		for page := 1; page <= MaxPages; page++ {
			var list struct {
				Total int         `json:"total_count"`
				Rows  []githubRun `json:"workflow_runs"`
			}
			address := fmt.Sprintf("%s/actions/runs?status=%s&per_page=100&page=%d", base, status, page)
			if err := record(address, &list); err != nil {
				return result, err
			}
			if count < 0 {
				count = list.Total
			}
			if count != list.Total || count < 0 || count > 1000 || len(list.Rows) > 100 {
				return result, ErrCoverage
			}
			for _, r := range list.Rows {
				if r.ID <= 0 || seen[r.ID] || r.Status != status || workflows[r.WorkflowID].ID == 0 || canonicalInteger(r.Repository.ID) != p.RepositoryID || r.Repository.FullName != Repository {
					return result, ErrCoverage
				}
				seen[r.ID] = true
				if canonicalInteger(r.ID) != p.RunID || canonicalInteger(r.RunAttempt) != p.RunAttempt || r.WorkflowID != p.WorkflowID || r.HeadSHA != p.SourceSHA || status != "in_progress" {
					return result, ErrActive
				}
			}
			if len(list.Rows) < 100 {
				break
			}
			if page == MaxPages {
				return result, ErrCoverage
			}
		}
		if len(seen) != count {
			return result, ErrCoverage
		}
		result.QueueRows += count
	}
	if result.QueueRows != 1 {
		return result, ErrCoverage
	}
	jobs := map[int64]bool{}
	jobFound := false
	jobTotal := -1
	for page := 1; page <= MaxPages; page++ {
		var list struct {
			Total int         `json:"total_count"`
			Rows  []githubJob `json:"jobs"`
		}
		if err := record(fmt.Sprintf("%s/actions/runs/%s/attempts/%s/jobs?per_page=100&page=%d", base, p.RunID, p.RunAttempt, page), &list); err != nil {
			return result, err
		}
		if jobTotal < 0 {
			jobTotal = list.Total
		}
		if jobTotal != list.Total || jobTotal < 1 || jobTotal > 1000 || len(list.Rows) > 100 {
			return result, ErrCoverage
		}
		for _, j := range list.Rows {
			if j.ID <= 0 || jobs[j.ID] || canonicalInteger(j.RunID) != p.RunID || j.HeadSHA != p.SourceSHA {
				return result, ErrCoverage
			}
			jobs[j.ID] = true
			if j.CheckRunURL == base+"/check-runs/"+p.CheckRunID {
				if jobFound || j.Status != "in_progress" || j.RunnerID != p.RunnerID {
					return result, ErrIdentity
				}
				jobFound = true
			} else if j.Status != "completed" {
				return result, ErrActive
			}
		}
		if len(list.Rows) < 100 {
			break
		}
		if page == MaxPages {
			return result, ErrCoverage
		}
	}
	if len(jobs) != jobTotal || !jobFound {
		return result, ErrCoverage
	}
	result.WorkflowCount = total
	sort.Strings(hashes)
	result.Digest = digest([]byte(strings.Join(hashes, "\n")))
	return result, nil
}

func nilDoer(client HTTPDoer) bool {
	v := reflect.ValueOf(client)
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Func, reflect.Slice, reflect.Chan:
		return v.IsNil()
	}
	return false
}
