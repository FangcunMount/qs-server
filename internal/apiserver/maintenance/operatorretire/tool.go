// Package operatorretire coordinates the one-off doctor backend retirement.
// SQL inventory and evidence live here; each exit uses the shared Actor application service.
package operatorretire

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"time"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/operatorretirement"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operatorretirement"
	repo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor/operatorretirement"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	"gorm.io/gorm"
)

type Candidate struct {
	ClinicianID uint64                     `json:"clinician_id"`
	OperatorID  uint64                     `json:"operator_id"`
	OrgID       int64                      `json:"org_id"`
	UserID      int64                      `json:"user_id"`
	Version     uint32                     `json:"version"`
	Roles       []authz.AssignmentRoleFact `json:"roles"`
}
type Report struct {
	MigrationID  string      `json:"migration_id"`
	State        string      `json:"state"`
	Fingerprint  string      `json:"fingerprint"`
	AfterHash    string      `json:"after_hash,omitempty"`
	CurrentHash  string      `json:"current_hash,omitempty"`
	BusinessHash string      `json:"business_hash"`
	BindingHash  string      `json:"binding_hash"`
	BindingCount int64       `json:"binding_count"`
	ActorID      int64       `json:"actor_id"`
	Candidates   []Candidate `json:"candidates"`
	Issues       []string    `json:"issues"`
	LastError    string      `json:"last_error,omitempty"`
	CreatedAt    time.Time   `json:"created_at"`
}
type manifest struct {
	MigrationID          string `gorm:"primaryKey"`
	Stage                string
	Payload              string
	PayloadSHA256        string `gorm:"column:payload_sha256"`
	CreatedAt, UpdatedAt time.Time
}

func (manifest) TableName() string { return "operator_retirement_manifests" }

type SnapshotReader interface {
	LoadFresh(context.Context, string) (*authz.Snapshot, error)
	LoadAssignmentFacts(context.Context, string) (*authz.Snapshot, error)
}

type Tool struct {
	db         *gorm.DB
	loader     SnapshotReader
	gateway    iambridge.OperatorAuthzGateway
	table      string
	repository *repo.Repository
}

func New(db *gorm.DB, loader SnapshotReader, gateway iambridge.OperatorAuthzGateway, layout string) (*Tool, error) {
	t := &Tool{db: db, loader: loader, gateway: gateway}
	switch layout {
	case "legacy":
		t.table = "staff"
		t.repository = repo.NewLegacyRepository(db)
	case "final":
		t.table = "operators"
		t.repository = repo.NewRepository(db)
	default:
		return nil, fmt.Errorf("explicit legacy or final layout is required")
	}
	return t, nil
}
func hash(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func rawHash(raw string) string { sum := sha256.Sum256([]byte(raw)); return hex.EncodeToString(sum[:]) }
func validID(id string) bool    { return regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`).MatchString(id) }
func (t *Tool) readManifest(ctx context.Context, id string) (*Report, string, error) {
	var m manifest
	err := t.db.WithContext(ctx).Where("migration_id=?", id).Take(&m).Error
	if err == gorm.ErrRecordNotFound {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	if rawHash(m.Payload) != m.PayloadSHA256 {
		return nil, "", fmt.Errorf("manifest checksum mismatch")
	}
	var r Report
	if err = json.Unmarshal([]byte(m.Payload), &r); err != nil {
		return nil, "", err
	}
	if r.MigrationID != id || r.State != m.Stage || r.Fingerprint == "" || r.BusinessHash == "" || r.BindingHash == "" {
		return nil, "", fmt.Errorf("invalid manifest identity or baseline")
	}
	return &r, m.PayloadSHA256, nil
}
func (t *Tool) save(ctx context.Context, r *Report, previous string) error {
	raw, err := json.Marshal(r)
	if err != nil {
		return err
	}
	m := manifest{MigrationID: r.MigrationID, Stage: r.State, Payload: string(raw), PayloadSHA256: rawHash(string(raw)), CreatedAt: r.CreatedAt, UpdatedAt: time.Now().UTC()}
	if previous == "" {
		return t.db.WithContext(ctx).Create(&m).Error
	}
	result := t.db.WithContext(ctx).Model(&manifest{}).Where("migration_id=? AND payload_sha256=?", r.MigrationID, previous).Updates(map[string]any{"stage": m.Stage, "payload": m.Payload, "payload_sha256": m.PayloadSHA256, "updated_at": m.UpdatedAt})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("manifest changed concurrently")
	}
	return nil
}

// databaseDigest streams a canonical projection. The binding column is separately archived;
// its removal must not make preserved business data appear changed.
func (t *Tool) databaseDigest(ctx context.Context, table string, exclude ...string) (string, error) {
	rows, err := t.db.WithContext(ctx).Table(table).Order("id").Rows()
	if err != nil {
		return "", err
	}
	defer func() { _ = rows.Close() }()
	cols, err := rows.Columns()
	if err != nil {
		return "", err
	}
	skip := map[string]bool{}
	for _, k := range exclude {
		skip[k] = true
	}
	digest := sha256.New()
	enc := json.NewEncoder(digest)
	for rows.Next() {
		values := make([]any, len(cols))
		pointers := make([]any, len(cols))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err = rows.Scan(pointers...); err != nil {
			return "", err
		}
		row := map[string]any{}
		for i, c := range cols {
			if skip[c] {
				continue
			}
			value := values[i]
			if b, ok := value.([]byte); ok {
				value = string(b)
			}
			row[c] = value
		}
		if err = enc.Encode(row); err != nil {
			return "", err
		}
	}
	if err = rows.Err(); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}
func (t *Tool) businessDigest(ctx context.Context) (string, error) {
	facts := map[string]string{}
	for _, table := range []string{"clinician", "actor_stores", "clinician_relation", "assessment_entry", "testee", "assessment", "assessment_plan", "assessment_task", "plan_enrollment"} {
		var excluded []string
		if table == "clinician" {
			excluded = []string{"operator_id"}
		}
		digest, err := t.databaseDigest(ctx, table, excluded...)
		if err != nil {
			return "", err
		}
		facts[table] = digest
	}
	return hash(facts), nil
}
func allowed(facts []authz.AssignmentRoleFact) error {
	for _, f := range facts {
		if f.RoleID == "" || f.ManagementProtection != "standard" {
			return fmt.Errorf("protected or invalid role %s", f.RoleName)
		}
		switch f.RoleName {
		case "user", "qs:assessment_operator", "qs:result_reviewer", "qs:content_manager", "qs:evaluation_plan_manager":
		default:
			return fmt.Errorf("unexpected role %s", f.RoleName)
		}
	}
	return nil
}
func retained(facts []authz.AssignmentRoleFact) []authz.AssignmentRoleFact {
	out := []authz.AssignmentRoleFact{}
	for _, f := range facts {
		if f.RoleName == "user" {
			out = append(out, f)
		}
	}
	return out
}
func normalized(facts []authz.AssignmentRoleFact) []authz.AssignmentRoleFact {
	out := append([]authz.AssignmentRoleFact{}, facts...)
	sort.Slice(out, func(i, j int) bool { return out[i].RoleID < out[j].RoleID })
	return out
}
func (t *Tool) currentHash(ctx context.Context, r *Report) (string, error) {
	bindings, err := t.bindingDigest(ctx, r.MigrationID)
	if err != nil {
		return "", err
	}
	business, err := t.businessDigest(ctx)
	if err != nil {
		return "", err
	}
	// IAM facts and policy versions below are authoritative. Projection refresh
	// timestamps/versions are expected to converge after cutover and are not a
	// change to the original Operator identity. Target projections are verified
	// independently by verifyExits.
	operators, err := t.databaseDigest(ctx, t.table, "roles", "effective_roles", "authz_policy_version", "authz_projected_at", "authz_projection_pending")
	if err != nil {
		return "", err
	}
	snapshots := map[int64]*authz.Snapshot{}
	for _, c := range r.Candidates {
		snap, e := t.loader.LoadAssignmentFacts(ctx, fmt.Sprint(c.UserID))
		if e != nil {
			return "", e
		}
		snapshots[c.UserID] = snap
	}
	return hash(struct {
		Business, Operators, Bindings string
		Snapshots                     map[int64]*authz.Snapshot
	}{business, operators, bindings, snapshots}), nil
}
func (t *Tool) Preflight(ctx context.Context, id string, actor int64) (*Report, error) {
	if !validID(id) || actor <= 0 {
		return nil, fmt.Errorf("migration id and actor id are required")
	}
	existing, _, err := t.readManifest(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		r, e := t.Status(ctx, id)
		if e == nil && r.State != "applied_unchanged" {
			e = fmt.Errorf("preflight cannot execute state %s", r.State)
		}
		return r, e
	}
	if t.table != "staff" {
		return nil, fmt.Errorf("first preflight requires legacy layout")
	}
	r := &Report{MigrationID: id, State: "pending", ActorID: actor, CreatedAt: time.Now().UTC(), Candidates: []Candidate{}, Issues: []string{}}
	administrator, err := t.loader.LoadFresh(ctx, fmt.Sprint(actor))
	if err != nil {
		return r, err
	}
	if administrator == nil || !administrator.IsQSAdmin() {
		return r, fmt.Errorf("actor lacks headquarters management permission")
	}
	var bindings []struct {
		ClinicianID, OperatorID uint64
		OrgID                   int64
		ClinicianType           string
	}
	err = t.db.WithContext(ctx).Table("clinician").Select("id AS clinician_id,org_id,operator_id,clinician_type").Where("operator_id IS NOT NULL AND deleted_at IS NULL").Order("id").Find(&bindings).Error
	if err != nil {
		return r, err
	}
	seen := map[uint64]bool{}
	for _, b := range bindings {
		var op struct {
			ID            uint64
			OrgID, UserID int64
			Version       uint32
			DeletedAt     *time.Time
		}
		e := t.db.WithContext(ctx).Table(t.table).Where("id=?", b.OperatorID).Take(&op).Error
		if e != nil || op.OrgID != b.OrgID || b.ClinicianType != "doctor" || seen[b.OperatorID] {
			r.Issues = append(r.Issues, fmt.Sprintf("clinician %d has invalid or duplicate binding", b.ClinicianID))
			continue
		}
		seen[b.OperatorID] = true
		var actorCount int64
		if e = t.db.WithContext(ctx).Table(t.table).Where("org_id=? AND user_id=? AND is_active=TRUE AND deleted_at IS NULL", b.OrgID, actor).Count(&actorCount).Error; e != nil {
			return r, e
		}
		if actorCount != 1 {
			r.Issues = append(r.Issues, fmt.Sprintf("actor has no active operator in company %d", b.OrgID))
		}
		snap, e := t.loader.LoadAssignmentFacts(ctx, fmt.Sprint(op.UserID))
		if e != nil {
			return r, e
		}
		if op.DeletedAt != nil {
			if len(retained(snap.AssignmentFacts)) != len(snap.AssignmentFacts) {
				r.Issues = append(r.Issues, fmt.Sprintf("deleted operator %d retains backend grants", op.ID))
			}
			continue
		}
		if e = allowed(snap.AssignmentFacts); e != nil {
			r.Issues = append(r.Issues, fmt.Sprintf("operator %d: %v", op.ID, e))
		}
		count, e := t.repository.OtherMemberships(ctx, op.UserID, op.ID)
		if e != nil {
			return r, e
		}
		if count != 0 {
			r.Issues = append(r.Issues, fmt.Sprintf("user %d has another company/operator membership", op.UserID))
		}
		if op.UserID == actor {
			r.Issues = append(r.Issues, "actor is in the retirement set")
		}
		r.Candidates = append(r.Candidates, Candidate{ClinicianID: b.ClinicianID, OperatorID: op.ID, OrgID: op.OrgID, UserID: op.UserID, Version: op.Version, Roles: normalized(snap.AssignmentFacts)})
	}
	r.BusinessHash, err = t.businessDigest(ctx)
	if err != nil {
		return r, err
	}
	r.BindingHash, err = t.bindingDigest(ctx, id)
	if err != nil {
		return r, err
	}
	if err = t.db.WithContext(ctx).Table("clinician").Where("operator_id IS NOT NULL").Count(&r.BindingCount).Error; err != nil {
		return r, err
	}
	r.Fingerprint, err = t.currentHash(ctx, r)
	if err != nil {
		return r, err
	}
	if len(r.Issues) != 0 {
		return r, fmt.Errorf("preflight found %d blocking issues", len(r.Issues))
	}
	return r, nil
}
func (t *Tool) Status(ctx context.Context, id string) (*Report, error) {
	r, _, err := t.readManifest(ctx, id)
	if err != nil {
		return &Report{MigrationID: id, State: "invalid"}, err
	}
	if r == nil {
		return &Report{MigrationID: id, State: "pending"}, nil
	}
	if r.State == "applied_unchanged" {
		if err = t.verifyArchive(ctx, r); err != nil {
			r.State = "invalid"
			return r, err
		}
	}
	current, err := t.currentHash(ctx, r)
	if err != nil {
		return r, err
	}
	r.CurrentHash = current
	if r.State == "applied_unchanged" && current != r.AfterHash {
		r.State = "applied_drifted"
	}
	return r, nil
}
func (t *Tool) verifyExits(ctx context.Context, r *Report) error {
	bindings, err := t.bindingDigest(ctx, r.MigrationID)
	if err != nil {
		return err
	}
	if bindings != r.BindingHash {
		return fmt.Errorf("clinician bindings changed")
	}
	digest, err := t.businessDigest(ctx)
	if err != nil {
		return err
	}
	if digest != r.BusinessHash {
		return fmt.Errorf("business data changed; automatic cutover refused")
	}
	for _, c := range r.Candidates {
		task, e := t.repository.FindTask(ctx, c.OrgID, c.OperatorID)
		if e != nil {
			return e
		}
		if task == nil || task.Stage != domain.Completed {
			return fmt.Errorf("operator %d has not completed retirement", c.OperatorID)
		}
		var count int64
		if e = t.db.WithContext(ctx).Table(t.table).Where("id=? AND user_id=? AND org_id=? AND deleted_at IS NOT NULL AND is_active=FALSE AND version=? AND authz_policy_version>=? AND authz_projection_pending=FALSE AND JSON_LENGTH(roles)=0 AND JSON_LENGTH(effective_roles)=0", c.OperatorID, c.UserID, c.OrgID, c.Version+2, task.PolicyVersion).Count(&count).Error; e != nil {
			return e
		}
		if count != 1 {
			return fmt.Errorf("operator %d is not disabled and deleted", c.OperatorID)
		}
		snap, e := t.loader.LoadAssignmentFacts(ctx, fmt.Sprint(c.UserID))
		if e != nil {
			return e
		}
		if hash(normalized(snap.AssignmentFacts)) != hash(retained(c.Roles)) || snap.AuthzVersion < task.PolicyVersion {
			return fmt.Errorf("operator %d authorization did not converge", c.OperatorID)
		}
	}
	return nil
}
func (t *Tool) Verify(ctx context.Context, id string) (*Report, error) {
	r, err := t.Status(ctx, id)
	if err != nil {
		return r, err
	}
	if r.State != "applied_unchanged" {
		return r, fmt.Errorf("retirement state is %s, not verified", r.State)
	}
	return r, t.verifyExits(ctx, r)
}
func (t *Tool) archiveBindings(ctx context.Context, id string) error {
	if t.table != "staff" {
		return fmt.Errorf("binding archival requires legacy layout")
	}
	var records []map[string]any
	if err := t.db.WithContext(ctx).Table("clinician").Where("operator_id IS NOT NULL").Order("id").Find(&records).Error; err != nil {
		return err
	}
	return t.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, record := range records {
			raw, err := json.Marshal(record)
			if err != nil {
				return err
			}
			var count int64
			if err = tx.Table("clinician_operator_binding_archive").Where("clinician_id=?", record["id"]).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return fmt.Errorf("binding archive already exists before manifest completion")
			}
			if err = tx.Table("clinician_operator_binding_archive").Create(map[string]any{"clinician_id": record["id"], "org_id": record["org_id"], "operator_id": record["operator_id"], "migration_id": id, "before_record": string(raw), "record_sha256": rawHash(string(raw)), "archived_at": time.Now().UTC()}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
func (t *Tool) Apply(ctx context.Context, id, fingerprint string, actor int64, maintenance bool) (*Report, error) {
	if !maintenance {
		return nil, fmt.Errorf("authorization and backend writes must be paused")
	}
	var result *Report
	// A named connection owns the lock for the whole batch; all domain transactions use their own user locks.
	err := t.db.WithContext(ctx).Connection(func(lock *gorm.DB) error {
		var got int
		if e := lock.Raw("SELECT GET_LOCK('qs:operator-retire:cutover',0)").Scan(&got).Error; e != nil {
			return e
		}
		if got != 1 {
			return fmt.Errorf("another retirement is executing")
		}
		defer func() {
			release, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			lock.WithContext(release).Exec("SELECT RELEASE_LOCK('qs:operator-retire:cutover')")
		}()
		r, checksum, e := t.readManifest(ctx, id)
		if e != nil {
			return e
		}
		if r != nil && r.State == "applied_unchanged" {
			if r.Fingerprint != fingerprint || r.ActorID != actor {
				return fmt.Errorf("completed migration identity or fingerprint mismatch")
			}
			result, e = t.Verify(ctx, id)
			return e
		}
		if r == nil {
			r, e = t.Preflight(ctx, id, actor)
			result = r
			if e != nil {
				return e
			}
			if r.Fingerprint != fingerprint {
				return fmt.Errorf("preflight fingerprint changed")
			}
			r.State = "executing"
			if e = t.save(ctx, r, ""); e != nil {
				return e
			}
			_, checksum, e = t.readManifest(ctx, id)
			if e != nil {
				return e
			}
		}
		result = r
		if r.Fingerprint != fingerprint || r.ActorID != actor || r.State != "executing" {
			return fmt.Errorf("manifest cannot be reused with changed identity or fingerprint")
		}
		// Business tables are immutable during the maintenance window. Compare once
		// before the batch and again in verifyExits; do not scan the entire business
		// database for every retiring person. Binding and assignment checks remain
		// per person because they govern each independent exit.
		digest, e := t.businessDigest(ctx)
		if e != nil {
			return e
		}
		if digest != r.BusinessHash {
			return fmt.Errorf("business facts drifted before retirement")
		}
		for _, c := range r.Candidates {
			bindings, e := t.bindingDigest(ctx, id)
			if e != nil {
				return e
			}
			if bindings != r.BindingHash {
				return fmt.Errorf("clinician bindings changed during retirement")
			}
			administrator, e := t.loader.LoadFresh(ctx, fmt.Sprint(actor))
			if e != nil {
				return e
			}
			if !administrator.IsQSAdmin() {
				return fmt.Errorf("actor lacks headquarters management permission")
			}
			var count int64
			if e = t.db.WithContext(ctx).Table(t.table).Where("org_id=? AND user_id=? AND is_active=TRUE AND deleted_at IS NULL", c.OrgID, actor).Count(&count).Error; e != nil {
				return e
			}
			if count != 1 {
				return fmt.Errorf("actor has no active operator in company %d", c.OrgID)
			}
			snap, e := t.loader.LoadAssignmentFacts(ctx, fmt.Sprint(c.UserID))
			if e != nil {
				return e
			}
			if e = allowed(snap.AssignmentFacts); e != nil {
				return e
			}
			task, e := t.repository.FindTask(ctx, c.OrgID, c.OperatorID)
			if e != nil {
				return e
			}
			if task == nil && hash(normalized(snap.AssignmentFacts)) != hash(c.Roles) {
				return fmt.Errorf("operator %d assignments changed", c.OperatorID)
			}
			if task != nil {
				expected := c.Version + 1
				if task.Stage == domain.Completed {
					expected++
				}
				var currentCount int64
				if e = t.db.WithContext(ctx).Table(t.table).Where("id=? AND org_id=? AND user_id=? AND version=? AND is_active=FALSE", c.OperatorID, c.OrgID, c.UserID, expected).Count(&currentCount).Error; e != nil {
					return e
				}
				if currentCount != 1 {
					return fmt.Errorf("retiring operator %d facts changed", c.OperatorID)
				}
				for _, f := range snap.AssignmentFacts {
					found := false
					for _, old := range c.Roles {
						if f == old {
							found = true
						}
					}
					if !found {
						return fmt.Errorf("new assignment on retiring operator %d", c.OperatorID)
					}
				}
			}
			_, e = app.NewMaintenanceService(t.repository, t.gateway).Execute(authz.WithSnapshot(ctx, administrator), app.Command{OrgID: c.OrgID, ActorID: actor, OperatorID: c.OperatorID, ExpectedVersion: c.Version, RequestID: id + "-" + fmt.Sprint(c.OperatorID), Reason: "retire clinician backend identity"})
			if e != nil {
				r.LastError = fmt.Sprintf("operator %d: %v", c.OperatorID, e)
				_ = t.save(ctx, r, checksum)
				return e
			}
		}
		if e = t.verifyExits(ctx, r); e != nil {
			return e
		}
		// Archive and completion are one database transaction, so a restart cannot strand an archive-only state.
		return t.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			original := t.db
			t.db = tx
			defer func() { t.db = original }()
			if e = t.archiveBindings(ctx, id); e != nil {
				return e
			}
			r.AfterHash, e = t.currentHash(ctx, r)
			if e != nil {
				return e
			}
			r.State = "applied_unchanged"
			r.LastError = ""
			return t.save(ctx, r, checksum)
		})
	})
	return result, err
}
