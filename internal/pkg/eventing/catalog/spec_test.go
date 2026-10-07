package eventcatalog

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func loadDefaultRegistry(t *testing.T) *EffectiveRegistry {
	t.Helper()
	cfg, err := Load("../../../../configs/events.yaml")
	if err != nil {
		t.Fatalf("Load events.yaml: %v", err)
	}
	registry, err := NewEffectiveRegistry(NewCatalog(cfg), DefaultSpecs())
	if err != nil {
		t.Fatalf("NewEffectiveRegistry: %v", err)
	}
	return registry
}

func TestEffectiveRegistryAndContractMatrixStayInSync(t *testing.T) {
	registry := loadDefaultRegistry(t)
	stores := loadMatrixStoreTokens(t)
	matrixBytes, err := os.ReadFile("../../../../docs/03-基础设施/event/20-事件契约与演进.md")
	if err != nil {
		t.Fatalf("read event matrix: %v", err)
	}
	matrix := string(matrixBytes)
	eventRows := parseMarkdownTable(t, matrix, []string{
		"Event type", "Owner", "Producer", "Delivery", "Profile", "Store", "Immediate", "Priority",
		"Handler", "Idempotency policy", "Settlement policy", "Handler failure behavior",
	})
	rows := indexMarkdownRows(t, eventRows, "Event type")
	if len(rows) != len(registry.Snapshot()) {
		t.Fatalf("matrix event rows = %d, effective events = %d", len(rows), len(registry.Snapshot()))
	}
	for _, evt := range registry.Snapshot() {
		row := rows[evt.Type]
		if row == nil {
			t.Fatalf("matrix is missing event %q", evt.Type)
		}
		want := map[string]string{
			"Owner":              evt.Owner,
			"Delivery":           string(evt.Delivery),
			"Profile":            string(evt.OutboxProfile),
			"Store":              matrixStoreToken(t, evt.OutboxProfile, stores),
			"Immediate":          strconv.FormatBool(evt.Immediate),
			"Priority":           string(evt.Priority),
			"Handler":            evt.PrimaryHandler,
			"Idempotency policy": evt.IdempotencyPolicy,
			"Settlement policy":  string(evt.SettlementPolicy),
		}
		for field, expected := range want {
			if got := row[field]; got != expected {
				t.Fatalf("matrix %s for %q = %q, want %q", field, evt.Type, got, expected)
			}
		}
		if row["Producer"] == "" || row["Handler failure behavior"] == "" {
			t.Fatalf("matrix row for %q must include readable producer and failure behavior", evt.Type)
		}
	}

	consumerRows := parseMarkdownTable(t, matrix, []string{
		"Consumer ID", "Event", "Runtime", "Topic", "Channel", "Idempotency policy", "Settlement policy",
	})
	actualConsumers := indexMarkdownRows(t, consumerRows, "Consumer ID")
	wantConsumerCount := 0
	for _, evt := range registry.Snapshot() {
		for _, consumer := range evt.AdditionalConsumers {
			wantConsumerCount++
			row := actualConsumers[consumer.ID]
			if row == nil {
				t.Fatalf("additional consumer table is missing %q", consumer.ID)
			}
			want := map[string]string{
				"Event": evt.Type, "Runtime": consumer.Runtime, "Topic": evt.Topic,
				"Channel": consumer.Channel, "Idempotency policy": consumer.IdempotencyPolicy,
				"Settlement policy": string(consumer.SettlementPolicy),
			}
			for field, expected := range want {
				if got := row[field]; got != expected {
					t.Fatalf("additional consumer %s for %q = %q, want %q", field, consumer.ID, got, expected)
				}
			}
		}
	}
	if len(actualConsumers) != wantConsumerCount {
		t.Fatalf("additional consumer rows = %d, registry consumers = %d", len(actualConsumers), wantConsumerCount)
	}
}

func matrixStoreToken(t *testing.T, profile OutboxProfile, stores map[OutboxProfile]string) string {
	t.Helper()
	if profile == "" {
		return "none"
	}
	store, ok := stores[profile]
	if !ok {
		t.Fatalf("no host storage evidence for profile %q", profile)
	}
	return store
}

func loadMatrixStoreTokens(t *testing.T) map[OutboxProfile]string {
	t.Helper()
	read := func(path string) string {
		t.Helper()
		data, err := os.ReadFile("../../../../" + path)
		if err != nil {
			t.Fatalf("read host storage evidence %s: %v", path, err)
		}
		return string(data)
	}
	stores, err := deriveMatrixStoreTokens(
		read("internal/apiserver/process/standard_event_subsystem_m4.go"),
		read("internal/apiserver/infra/mysql/standardoutbox/status.go"),
		read("internal/pkg/migration/migrations/mysql/000084_standard_reliable_outbox.up.sql"),
	)
	if err != nil {
		t.Fatalf("derive current host stores: %v", err)
	}
	return stores
}

// Read the actual composition and query rather than copy a second store-name
// catalog into the test. Fail closed when a source shape changes: that change
// requires reviewing the documentation contract, not silently skipping a cell.
func deriveMatrixStoreTokens(mongoComposition, mysqlStatus, migration string) (map[OutboxProfile]string, error) {
	mongoBody, err := contractFunction(mongoComposition, "buildM4StandardEventSubsystem")
	if err != nil {
		return nil, err
	}
	var collections []string
	ast.Inspect(mongoBody, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		method, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || method.Sel.Name != "Collection" {
			return true
		}
		db, ok := method.X.(*ast.SelectorExpr)
		if !ok || db.Sel.Name != "MongoDB" {
			return true
		}
		owner, ok := db.X.(*ast.Ident)
		if !ok || owner.Name != "opts" {
			return true
		}
		value := ""
		if len(call.Args) == 1 {
			value, _ = contractStringLiteral(call.Args[0])
		}
		collections = append(collections, value)
		return true
	})
	if len(collections) != 1 || collections[0] == "" {
		return nil, fmt.Errorf("expected one literal host Mongo Collection, found %v", collections)
	}
	mysqlBody, err := contractFunction(mysqlStatus, "OutboxStatusSnapshot")
	if err != nil {
		return nil, err
	}
	var queries []string
	ast.Inspect(mysqlBody, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		method, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || method.Sel.Name != "QueryContext" {
			return true
		}
		query := ""
		if len(call.Args) >= 2 {
			query, _ = contractStringLiteral(call.Args[1])
		}
		queries = append(queries, query)
		return true
	})
	if len(queries) != 1 || !regexp.MustCompile(`(?i)^\s*SELECT\b`).MatchString(queries[0]) {
		return nil, fmt.Errorf("expected one literal SELECT in MySQL status reader, found %v", queries)
	}
	from := regexp.MustCompile("(?i)\\bFROM\\s+`?([A-Za-z_][A-Za-z0-9_]*)`?\\b").FindAllStringSubmatch(queries[0], -1)
	if len(from) != 1 || regexp.MustCompile(`(?i)\b(JOIN|UNION)\b`).MatchString(queries[0]) {
		return nil, fmt.Errorf("expected one unambiguous MySQL status FROM object")
	}
	table := from[0][1]
	if collections[0] != table {
		return nil, fmt.Errorf("Mongo collection %q and MySQL status table %q disagree", collections[0], table)
	}
	create := regexp.MustCompile("(?im)^\\s*CREATE\\s+TABLE(?:\\s+IF\\s+NOT\\s+EXISTS)?\\s+`?([A-Za-z_][A-Za-z0-9_]*)`?\\s*\\(").FindAllStringSubmatch(migration, -1)
	declarations := 0
	for _, match := range create {
		if match[1] == table {
			declarations++
		}
	}
	if declarations != 1 {
		return nil, fmt.Errorf("MySQL status table %q has %d standard migration declarations", table, declarations)
	}
	return map[OutboxProfile]string{
		OutboxProfileMongoDomain:     "Mongo " + collections[0],
		OutboxProfileAssessmentMySQL: "MySQL " + table,
	}, nil
}

func contractFunction(source, name string) (*ast.BlockStmt, error) {
	file, err := parser.ParseFile(token.NewFileSet(), "host.go", source, 0)
	if err != nil {
		return nil, err
	}
	var bodies []*ast.BlockStmt
	for _, declaration := range file.Decls {
		if fn, ok := declaration.(*ast.FuncDecl); ok && fn.Name.Name == name && fn.Body != nil {
			bodies = append(bodies, fn.Body)
		}
	}
	if len(bodies) != 1 {
		return nil, fmt.Errorf("expected one host function %s, found %d", name, len(bodies))
	}
	return bodies[0], nil
}

func contractStringLiteral(expr ast.Expr) (string, error) {
	literal, ok := expr.(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return "", fmt.Errorf("host contract is not a literal string")
	}
	return strconv.Unquote(literal.Value)
}

func TestMatrixStoreEvidenceRejectsAmbiguousAndRetiredSources(t *testing.T) {
	// A different coherent current name succeeds: no rm_outbox hard-coded oracle.
	mongo := `package host; func buildM4StandardEventSubsystem() { opts.MongoDB.Collection("current_store") }`
	mysql := `package host; func OutboxStatusSnapshot() { r.db.QueryContext(ctx, "SELECT state FROM current_store WHERE state <> 'published'") }`
	migration := "CREATE TABLE current_store (id BIGINT);\nCREATE TABLE replay_ledger (id BIGINT);"
	stores, err := deriveMatrixStoreTokens(mongo, mysql, migration)
	if err != nil || stores[OutboxProfileMongoDomain] != "Mongo current_store" || stores[OutboxProfileAssessmentMySQL] != "MySQL current_store" {
		t.Fatalf("coherent host stores = %v, error = %v", stores, err)
	}
	for _, tc := range []struct {
		name, mongo, mysql, migration string
	}{
		{"retired-mongo", strings.ReplaceAll(mongo, "current_store", "domain_event_outbox"), mysql, migration},
		{"retired-mysql", mongo, strings.ReplaceAll(mysql, "current_store", "domain_event_outbox"), migration},
		{"retired-sources-vs-migration", strings.ReplaceAll(mongo, "current_store", "domain_event_outbox"), strings.ReplaceAll(mysql, "current_store", "domain_event_outbox"), migration},
		{"missing-mongo", "package host; func buildM4StandardEventSubsystem() {}", mysql, migration},
		{"multiple-mongo", strings.Replace(mongo, " }", `; opts.MongoDB.Collection("current_store") }`, 1), mysql, migration},
		{"dynamic-mongo", strings.ReplaceAll(mongo, `"current_store"`, "collectionName"), mysql, migration},
		{"missing-mysql", mongo, "package host; func OutboxStatusSnapshot() {}", migration},
		{"multiple-mysql", mongo, strings.Replace(mysql, " }", `; r.db.QueryContext(ctx, "SELECT state FROM current_store") }`, 1), migration},
		{"multiple-from", mongo, strings.ReplaceAll(mysql, "WHERE state", "WHERE id IN (SELECT id FROM current_store) AND state"), migration},
		{"dynamic-mysql", mongo, "package host; func OutboxStatusSnapshot() { r.db.QueryContext(ctx, query) }", migration},
		{"missing-migration", mongo, mysql, "CREATE TABLE unrelated (id BIGINT);"},
		{"duplicate-migration", mongo, mysql, migration + "\nCREATE TABLE current_store (id BIGINT);"},
		{"malformed-go", "not a Go file", mysql, migration},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := deriveMatrixStoreTokens(tc.mongo, tc.mysql, tc.migration); err == nil {
				t.Fatalf("inconsistent/ambiguous source accepted: %v", got)
			}
		})
	}
}

func parseMarkdownTable(t *testing.T, document string, headers []string) []map[string]string {
	t.Helper()
	lines := strings.Split(document, "\n")
	for index, line := range lines {
		cells := markdownCells(line)
		if !slices.Equal(cells, headers) {
			continue
		}
		var rows []map[string]string
		for _, rowLine := range lines[index+2:] {
			rowCells := markdownCells(rowLine)
			if len(rowCells) == 0 {
				break
			}
			if len(rowCells) != len(headers) {
				t.Fatalf("matrix row has %d cells, want %d: %s", len(rowCells), len(headers), rowLine)
			}
			row := make(map[string]string, len(headers))
			for i, header := range headers {
				row[header] = rowCells[i]
			}
			rows = append(rows, row)
		}
		return rows
	}
	t.Fatalf("matrix table with headers %v not found", headers)
	return nil
}

func markdownCells(line string) []string {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "|") || !strings.HasSuffix(line, "|") {
		return nil
	}
	parts := strings.Split(strings.Trim(line, "|"), "|")
	cells := make([]string, len(parts))
	for i, part := range parts {
		cells[i] = strings.Trim(strings.TrimSpace(part), "`")
	}
	return cells
}

func indexMarkdownRows(t *testing.T, rows []map[string]string, key string) map[string]map[string]string {
	t.Helper()
	indexed := make(map[string]map[string]string, len(rows))
	for _, row := range rows {
		value := row[key]
		if value == "" {
			t.Fatalf("matrix row has empty %s: %#v", key, row)
		}
		if _, exists := indexed[value]; exists {
			t.Fatalf("matrix has duplicate %s %q", key, value)
		}
		indexed[value] = row
	}
	return indexed
}

func TestEffectiveRegistryCoversWireCatalog(t *testing.T) {
	registry := loadDefaultRegistry(t)
	if len(registry.Snapshot()) != len(EventTypes()) {
		t.Fatalf("effective events = %d, code events = %d", len(registry.Snapshot()), len(EventTypes()))
	}
	for _, eventType := range EventTypes() {
		if _, ok := registry.Lookup(eventType); !ok {
			t.Fatalf("event %q missing from effective registry", eventType)
		}
	}
}

func TestEffectiveRegistryDerivesProfilePolicy(t *testing.T) {
	registry := loadDefaultRegistry(t)

	if got := registry.ImmediateTypes(OutboxProfileMongoDomain); !slices.Equal(got, []string{AnswerSheetSubmitted}) {
		t.Fatalf("mongo immediate types = %#v", got)
	}
	if got := registry.ImmediateTypes(OutboxProfileAssessmentMySQL); !slices.Equal(got, []string{EvaluationOutcomeCommitted, EvaluationRequested, TaskOpenedReminderRequested}) {
		t.Fatalf("mysql immediate types = %#v", got)
	}
	if got := registry.PriorityBucket(AnswerSheetSubmitted); got != string(PriorityP0) {
		t.Fatalf("answersheet bucket = %q", got)
	}
	if got := registry.PriorityBucket(TaskOpened); got != string(PriorityP2) {
		t.Fatalf("best-effort fallback bucket = %q", got)
	}

	tiers := registry.PriorityTiers(OutboxProfileMongoDomain)
	if len(tiers) != 3 || !slices.Contains(tiers[0], AnswerSheetSubmitted) || tiers[2] != nil {
		t.Fatalf("mongo priority tiers = %#v", tiers)
	}
}

func TestEffectiveRegistryDeclaresHotRankSecondaryConsumer(t *testing.T) {
	consumers := loadDefaultRegistry(t).Consumers(AnswerSheetSubmitted)
	if len(consumers) != 1 {
		t.Fatalf("consumers = %#v", consumers)
	}
	if consumers[0].ID != "modelcatalog.hot_rank_projection" || consumers[0].Channel != "qs-apiserver-modelcatalog-hot-rank-v1" {
		t.Fatalf("consumer = %#v", consumers[0])
	}
}

func TestEffectiveRegistryRejectsDeliveryPolicyMismatch(t *testing.T) {
	cfg, err := Load("../../../../configs/events.yaml")
	if err != nil {
		t.Fatal(err)
	}
	specs := DefaultSpecs()
	for i := range specs {
		if specs[i].Type == TaskOpened {
			specs[i].Immediate = true
			break
		}
	}
	if _, err := NewEffectiveRegistry(NewCatalog(cfg), specs); err == nil {
		t.Fatal("best-effort immediate policy must be rejected")
	}
}
