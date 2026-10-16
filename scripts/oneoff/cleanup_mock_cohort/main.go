// cleanup_mock_cohort is a private exact-object coordinator. It never invokes
// migrations, application containers, IAM deletion, topic purge or cache flush.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var toolSource = "development" // Set to the exact source commit with -ldflags.

type config struct {
	action, group, groupSHA, protection, protectionSHA, archive, materialSHA, approval, approvalSHA, output string
	owner, freeze, freezeSHA                                                                                string
	selector, selectorSHA, scopeDir                                                                         string
	captureEvidence, captureEvidenceSHA                                                                     string
	execute, original                                                                                       bool
	timeout                                                                                                 time.Duration
}

func flags(args []string) (config, error) {
	var c config
	f := flag.NewFlagSet("cleanup_mock_cohort", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.StringVar(&c.action, "action", "plan", "plan/capture/apply/verify/restore/plan-cohort")
	f.StringVar(&c.selector, "selector-file", "", "fixed whole-cohort readonly selector")
	f.StringVar(&c.selectorSHA, "selector-sha256", "", "selector raw SHA256")
	f.StringVar(&c.scopeDir, "scope-dir", "", "fresh owned 0700 scope-shard directory")
	f.StringVar(&c.captureEvidence, "capture-evidence-file", "", "external capture executor original stdout/exit evidence")
	f.StringVar(&c.captureEvidenceSHA, "capture-evidence-sha256", "", "capture executor evidence raw SHA256")
	f.StringVar(&c.group, "group-file", "", "private exact group JSON")
	f.StringVar(&c.groupSHA, "group-sha256", "", "group raw SHA256")
	f.StringVar(&c.protection, "protection-file", "", "private system/clack protection JSON")
	f.StringVar(&c.protectionSHA, "protection-sha256", "", "protection raw SHA256")
	f.StringVar(&c.archive, "archive-dir", "", "owned 0700 archive outside Git")
	f.StringVar(&c.materialSHA, "material-sha256", "", "material.json raw SHA256")
	f.StringVar(&c.approval, "approval-file", "", "independent exact action approval")
	f.StringVar(&c.approvalSHA, "approval-sha256", "", "approval raw SHA256")
	f.StringVar(&c.output, "output-file", "", "fresh private planned manifest output")
	f.StringVar(&c.owner, "owner-confirmation-file", "", "original system(10001) whole-cohort scope confirmation")
	f.StringVar(&c.freeze, "frozen-receipt-file", "", "independent writer/replay freeze receipt")
	f.StringVar(&c.freezeSHA, "frozen-receipt-sha256", "", "freeze receipt raw SHA256")
	f.BoolVar(&c.execute, "execute", false, "enable approved apply/restore; default is dry-run")
	f.BoolVar(&c.original, "expect-original", false, "verify exact restored preimages instead of absence")
	f.DurationVar(&c.timeout, "timeout", 20*time.Minute, "bounded CLI budget; cannot extend approval expiry")
	if e := f.Parse(args); e != nil {
		return c, e
	}
	evidenceProvided := false
	f.Visit(func(v *flag.Flag) {
		if v.Name == "capture-evidence-file" || v.Name == "capture-evidence-sha256" {
			evidenceProvided = true
		}
	})
	if evidenceProvided && (c.action == "plan" || c.action == "capture" || c.action == "plan-cohort") {
		return c, errors.New("discovery/capture cannot accept capture execution evidence")
	}
	if f.NArg() != 0 {
		return c, errors.New("positional inputs rejected")
	}
	switch c.action {
	case "plan", "capture", "apply", "verify", "restore", "plan-cohort":
	default:
		return c, errors.New("unknown action")
	}
	if c.execute && c.action != "apply" && c.action != "restore" {
		return c, errors.New("execute only belongs to approved data actions")
	}
	if c.timeout <= 0 || c.timeout > 25*time.Minute {
		return c, errors.New("CLI timeout must leave the five-minute maintenance reserve")
	}
	return c, nil
}

func connect(ctx context.Context, w binding) (*liveStore, func() error, error) {
	dsn, e := privateRead(os.Getenv("QS_MOCK_MYSQL_DSN_FILE"), 64*1024)
	if e != nil {
		return nil, nil, safePhase("mysql_configuration", errors.New("private MySQL DSN file required"))
	}
	uri, e := privateRead(os.Getenv("QS_MOCK_MONGO_URI_FILE"), 64*1024)
	if e != nil {
		return nil, nil, safePhase("mongo_configuration", errors.New("private Mongo URI file required"))
	}
	cfg, e := mysqldriver.ParseDSN(strings.TrimSpace(string(dsn)))
	if e != nil {
		return nil, nil, errors.New("invalid private MySQL configuration")
	}
	if cfg.DBName != w.SQLDatabase || cfg.MultiStatements {
		return nil, nil, errors.New("DSN database/multi-statement mismatch")
	}
	cfg.ParseTime = false
	cfg.InterpolateParams = false
	cfg.Timeout = 10 * time.Second
	cfg.ReadTimeout = 30 * time.Second
	cfg.WriteTimeout = 30 * time.Second
	db, e := sql.Open("mysql", cfg.FormatDSN())
	if e != nil {
		return nil, nil, safePhase("mysql_open", e)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	conn, e := db.Conn(ctx)
	if e != nil {
		_ = db.Close()
		return nil, nil, safePhase("mysql_connection", e)
	}
	client, e := mongo.Connect(ctx, options.Client().ApplyURI(strings.TrimSpace(string(uri))).SetMaxPoolSize(1).SetMinPoolSize(0).SetConnectTimeout(10*time.Second))
	if e != nil {
		_ = conn.Close()
		_ = db.Close()
		return nil, nil, safePhase("mongo_connection", e)
	}
	store := &liveStore{sql: conn, mongo: client, mongoDB: w.MongoDatabase}
	held := false
	lock := "qs_mock_cohort_" + digest(encode([]string{w.SQLUUID, w.SQLDatabase}))[:32]
	closeAll := func() error {
		q, c := context.WithTimeout(context.Background(), 15*time.Second)
		defer c()
		var release error
		if held {
			var ok sql.NullInt64
			if e := conn.QueryRowContext(q, "SELECT RELEASE_LOCK(?)", lock).Scan(&ok); e != nil || !ok.Valid || ok.Int64 != 1 {
				release = errors.New("SQL serialization gate release unknown")
			}
		}
		return errors.Join(release, conn.Close(), db.Close(), client.Disconnect(q))
	}
	if e = store.identity(ctx, w); e != nil {
		return nil, nil, errors.Join(safePhase("initial_identity", e), closeAll())
	}
	var ok sql.NullInt64
	if e = conn.QueryRowContext(ctx, "SELECT GET_LOCK(?,0)", lock).Scan(&ok); e != nil || !ok.Valid || ok.Int64 != 1 {
		return nil, nil, errors.Join(safePhase("mysql_gate", errors.Join(errors.New("same-database cleanup executor gate unavailable"), e)), closeAll())
	}
	held = true
	if e = store.identity(ctx, w); e != nil {
		return nil, nil, errors.Join(safePhase("locked_identity", e), closeAll())
	}
	return store, closeAll, nil
}

func materialScope(g group, p protected, m material, groupSHA string) error {
	if m.Format != 1 || m.Kind != "mock_cohort_archive" || m.Operation != g.Operation || m.GroupSHA != groupSHA || m.ProtectedSHA != g.ProtectedSHA || !reflect.DeepEqual(m.Binding, g.QS.Binding) || len(m.Records) != len(g.QS.Objects) {
		return errors.New("archive/group authority mismatch")
	}
	for i, r := range m.Records {
		if !reflect.DeepEqual(r.Object, g.QS.Objects[i]) || !hashPattern.MatchString(r.SHA) {
			return errors.New("archive exact physical scope mismatch")
		}
	}
	guards := sortedObjects(append(append(append([]object{}, p.QS.System...), p.QS.Clack...), p.QS.Other...))
	if len(guards) != len(m.Protected) {
		return errors.New("archive protection scope incomplete")
	}
	for i, o := range guards {
		if objectID(o) != objectID(m.Protected[i].Object) {
			return errors.New("archive protection identity mismatch")
		}
	}
	return nil
}

func run(args []string, out io.Writer) error {
	c, e := flags(args)
	if e != nil {
		return e
	}
	if c.action == "plan-cohort" {
		return safePhase("graph_plan", runGraphPlan(c, out))
	}
	var g group
	var p protected
	if e = readExact(c.group, c.groupSHA, 256*1024*1024, &g); e != nil {
		return e
	}
	if e = readExact(c.protection, c.protectionSHA, 32*1024*1024, &p); e != nil {
		return e
	}
	if g.ProtectedSHA != c.protectionSHA {
		return errors.New("group protection hash mismatch")
	}
	if g.QS.Graph != nil {
		return safePhase("graph_action", runGraphAction(c, g, p, out))
	}
	if c.captureEvidence != "" || c.captureEvidenceSHA != "" {
		return errors.New("capture evidence only belongs to full graph consumers")
	}
	if e = validateGroup(g, p); e != nil {
		return e
	}
	if c.action != "plan" {
		raw, e := privateRead(c.group, 256*1024*1024)
		if e != nil || digest(raw) != c.groupSHA {
			return errors.New("group original changed")
		}
		owner, e := privateRead(c.owner, 32*1024)
		if e != nil {
			return e
		}
		if e = validateOwner(raw, owner, g); e != nil {
			return e
		}
	}
	var m material
	var a approval
	if c.action != "plan" && c.action != "capture" {
		if e = privateDirectory(c.archive); e != nil {
			return e
		}
		if e = readExact(filepath.Join(c.archive, "material.json"), c.materialSHA, 256*1024*1024, &m); e != nil {
			return e
		}
		if e = materialScope(g, p, m, c.groupSHA); e != nil {
			return e
		}
	}
	if c.execute {
		if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(toolSource) {
			return errors.New("mutating tool must bind an exact compiled source commit")
		}
		if e = readExact(c.approval, c.approvalSHA, 32*1024, &a); e != nil {
			return e
		}
		if e = checkApproval(a, m, g, c.materialSHA, c.action, toolSource, time.Now()); e != nil {
			return e
		}
		if a.FreezeSHA != c.freezeSHA || a.SettlementSHA != c.freezeSHA {
			return errors.New("action must bind the exact shared frozen/replay receipt")
		}
	}
	if c.action == "capture" || c.execute {
		raw, e := privateRead(c.freeze, 32*1024)
		if e != nil {
			return e
		}
		if digest(raw) != c.freezeSHA {
			return errors.New("freeze original changed")
		}
		if e = validateFreeze(raw, g, c.groupSHA); e != nil {
			return e
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	if c.execute {
		var stop context.CancelFunc
		ctx, stop = context.WithDeadline(ctx, a.Expires)
		defer stop()
	}
	s, closeAll, e := connect(ctx, g.QS.Binding)
	if e != nil {
		return e
	}
	var planned group
	var captured material
	switch c.action {
	case "plan":
		planned, e = plan(ctx, s, g, p)
	case "capture":
		captured, e = capture(ctx, s, g, p, c.archive, c.groupSHA, toolSource)
	case "verify":
		e = verify(ctx, s, m, c.archive, c.original)
	case "apply", "restore":
		if c.execute {
			e = mutate(ctx, s, g, m, c.archive, a, c.approvalSHA, c.action == "restore")
		} else {
			e = protect(ctx, s, m, c.archive)
			if e == nil {
				e = precheckTargets(ctx, s, m, c.archive, c.action == "restore")
			}
		}
	}
	// A failed/unknown release is never converted into a successful receipt.
	e = errors.Join(e, closeAll(), ctx.Err())
	if e != nil {
		return e
	}
	if c.action == "plan" {
		if e = privateDirectory(filepath.Dir(c.output)); e != nil {
			return e
		}
		if e = writeNew(c.output, encode(planned)); e != nil {
			return e
		}
	}
	if c.action == "capture" {
		if e = writeNew(filepath.Join(c.archive, "material.json"), encode(captured)); e != nil {
			return e
		}
		c.materialSHA = digest(encode(captured))
	}
	r := actionReceipt{Format: 1, Kind: "mock_cohort_qs_action_result", Action: c.action, Operation: g.Operation, GroupSHA: c.groupSHA, Source: toolSource, MaterialSHA: c.materialSHA, Complete: true, DryRun: c.action == "plan" || ((c.action == "apply" || c.action == "restore") && !c.execute), Whole: false, Pending: true, Exclusions: len(g.QS.Exclusions), Released: true}
	_, e = out.Write(append(encode(r), '\n'))
	return e
}

func main() {
	if e := run(os.Args[1:], os.Stdout); e != nil {
		_, _ = os.Stderr.Write(append(encode(safeFailure(e)), '\n'))
		os.Exit(1)
	}
}
