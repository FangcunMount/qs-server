// audit_norm_usage is a read-only reverse-reference inventory for Norm assets
// (MC-R020 slice A). Published AssessmentSnapshot NormRefs are the usage
// source of truth; Norm aggregates are never mutated.
//
// Exit 0: scan OK and no dangling refs.
// Exit 1: connect/scan failure.
// Exit 2: scan OK but dangling NormRefs found.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	mongomodelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
)

type config struct {
	mongoURI    string
	mongoDB     string
	normVersion string
	jsonOut     bool
	timeout     time.Duration
}

type snapshotRef struct {
	Code      string
	Version   string
	Kind      string
	Algorithm string
	NormRefs  []normRef
}

type normRef struct {
	FactorCode       string
	NormTableVersion string
}

type modelUsage struct {
	Code        string   `json:"code"`
	Version     string   `json:"version"`
	Kind        string   `json:"kind"`
	Algorithm   string   `json:"algorithm"`
	FactorCodes []string `json:"factor_codes"`
}

type usageEntry struct {
	NormTableVersion string       `json:"norm_table_version"`
	Models           []modelUsage `json:"models"`
}

type danglingRef struct {
	Code             string `json:"code"`
	Version          string `json:"version"`
	Kind             string `json:"kind"`
	Algorithm        string `json:"algorithm"`
	FactorCode       string `json:"factor_code"`
	NormTableVersion string `json:"norm_table_version"`
}

type multiVersionSnapshot struct {
	Code              string   `json:"code"`
	Version           string   `json:"version"`
	Kind              string   `json:"kind"`
	Algorithm         string   `json:"algorithm"`
	NormTableVersions []string `json:"norm_table_versions"`
}

type report struct {
	PublishedScanned      int                    `json:"published_scanned"`
	PublishedWithRefs     int                    `json:"published_with_refs"`
	NormsTotal            int                    `json:"norms_total"`
	UsageCount            int                    `json:"usage_count"`
	DanglingCount         int                    `json:"dangling_count"`
	UnreferencedCount     int                    `json:"unreferenced_count"`
	MultiVersionCount     int                    `json:"multi_version_count"`
	Usages                []usageEntry           `json:"usages"`
	DanglingRefs          []danglingRef          `json:"dangling_refs"`
	UnreferencedNorms     []string               `json:"unreferenced_norms"`
	MultiVersionSnapshots []multiVersionSnapshot `json:"multi_version_snapshots"`
}

type snapshotDoc struct {
	Code           string `bson:"code"`
	ReleaseVersion string `bson:"release_version"`
	Kind           string `bson:"kind"`
	Algorithm      string `bson:"algorithm"`
	DefinitionV2   *struct {
		Calibration struct {
			NormRefs []struct {
				FactorCode       string `bson:"factor_code"`
				NormTableVersion string `bson:"norm_table_version"`
			} `bson:"norm_refs"`
		} `bson:"calibration"`
	} `bson:"definition_v2"`
}

type normDoc struct {
	TableVersion string `bson:"table_version"`
}

func main() {
	cfg := parseFlags()
	if cfg.mongoURI == "" {
		fmt.Fprintln(os.Stderr, "audit norm usage failed: --mongo-uri is required (or set MONGO_URI)")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.mongoURI))
	if err != nil {
		fmt.Fprintln(os.Stderr, "audit norm usage failed: connect mongo:", err)
		os.Exit(1)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	if err := client.Ping(ctx, nil); err != nil {
		fmt.Fprintln(os.Stderr, "audit norm usage failed: ping mongo:", err)
		os.Exit(1)
	}
	norms, snaps, err := loadInputs(ctx, client.Database(cfg.mongoDB))
	if err != nil {
		fmt.Fprintln(os.Stderr, "audit norm usage failed:", err)
		os.Exit(1)
	}
	result := buildReport(norms, snaps, cfg.normVersion)
	if cfg.jsonOut {
		_ = json.NewEncoder(os.Stdout).Encode(result)
	} else {
		printReport(result)
	}
	if result.DanglingCount > 0 {
		os.Exit(2)
	}
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.mongoURI, "mongo-uri", os.Getenv("MONGO_URI"), "MongoDB URI")
	flag.StringVar(&cfg.mongoDB, "mongo-db", envOr("MONGO_DB", "qs_server"), "MongoDB database")
	flag.StringVar(&cfg.normVersion, "norm-version", "", "only report usage for this NormTableVersion")
	flag.BoolVar(&cfg.jsonOut, "json", false, "emit JSON report")
	flag.DurationVar(&cfg.timeout, "timeout", 5*time.Minute, "operation timeout")
	flag.Parse()
	return cfg
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func loadInputs(ctx context.Context, db *mongo.Database) ([]string, []snapshotRef, error) {
	norms, err := loadNormVersions(ctx, db)
	if err != nil {
		return nil, nil, err
	}
	snaps, err := loadPublishedSnapshots(ctx, db)
	if err != nil {
		return nil, nil, err
	}
	return norms, snaps, nil
}

func loadNormVersions(ctx context.Context, db *mongo.Database) ([]string, error) {
	collName := (&mongomodelcatalog.NormPO{}).CollectionName()
	cur, err := db.Collection(collName).Find(ctx, bson.M{"deleted_at": nil}, options.Find().SetProjection(bson.M{
		"table_version": 1,
	}))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cur.Close(ctx) }()
	var out []string
	for cur.Next(ctx) {
		var doc normDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		if doc.TableVersion == "" {
			continue
		}
		out = append(out, doc.TableVersion)
	}
	return out, cur.Err()
}

func loadPublishedSnapshots(ctx context.Context, db *mongo.Database) ([]snapshotRef, error) {
	collName := (&mongomodelcatalog.PublishedAssessmentModelPO{}).CollectionName()
	cur, err := db.Collection(collName).Find(ctx, bson.M{
		"deleted_at":  nil,
		"record_role": "published_snapshot",
	}, options.Find().SetProjection(bson.M{
		"code":                                1,
		"release_version":                     1,
		"kind":                                1,
		"algorithm":                           1,
		"definition_v2.calibration.norm_refs": 1,
	}))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cur.Close(ctx) }()
	var out []snapshotRef
	for cur.Next(ctx) {
		var doc snapshotDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		ref := snapshotRef{
			Code:      doc.Code,
			Version:   doc.ReleaseVersion,
			Kind:      doc.Kind,
			Algorithm: doc.Algorithm,
		}
		if doc.DefinitionV2 != nil {
			for _, nr := range doc.DefinitionV2.Calibration.NormRefs {
				if nr.NormTableVersion == "" {
					continue
				}
				ref.NormRefs = append(ref.NormRefs, normRef{
					FactorCode:       nr.FactorCode,
					NormTableVersion: nr.NormTableVersion,
				})
			}
		}
		out = append(out, ref)
	}
	return out, cur.Err()
}

// buildReport is the pure audit core: Norm catalog + published snapshot refs → report.
func buildReport(normVersions []string, snaps []snapshotRef, filterVersion string) report {
	normSet := make(map[string]struct{}, len(normVersions))
	for _, v := range normVersions {
		if v == "" {
			continue
		}
		normSet[v] = struct{}{}
	}

	type modelKey struct {
		code, version, kind, algorithm string
	}
	type usageAccum struct {
		meta    modelKey
		factors map[string]struct{}
	}
	usagesByNorm := map[string]map[modelKey]*usageAccum{}
	var dangling []danglingRef
	var multi []multiVersionSnapshot
	publishedWithRefs := 0

	for _, snap := range snaps {
		if len(snap.NormRefs) == 0 {
			continue
		}
		publishedWithRefs++
		versions := map[string]struct{}{}
		for _, nr := range snap.NormRefs {
			versions[nr.NormTableVersion] = struct{}{}
			if filterVersion != "" && nr.NormTableVersion != filterVersion {
				continue
			}
			if _, ok := normSet[nr.NormTableVersion]; !ok {
				dangling = append(dangling, danglingRef{
					Code: snap.Code, Version: snap.Version, Kind: snap.Kind, Algorithm: snap.Algorithm,
					FactorCode: nr.FactorCode, NormTableVersion: nr.NormTableVersion,
				})
			}
			mk := modelKey{snap.Code, snap.Version, snap.Kind, snap.Algorithm}
			byModel, ok := usagesByNorm[nr.NormTableVersion]
			if !ok {
				byModel = map[modelKey]*usageAccum{}
				usagesByNorm[nr.NormTableVersion] = byModel
			}
			acc, ok := byModel[mk]
			if !ok {
				acc = &usageAccum{meta: mk, factors: map[string]struct{}{}}
				byModel[mk] = acc
			}
			if nr.FactorCode != "" {
				acc.factors[nr.FactorCode] = struct{}{}
			}
		}
		if len(versions) > 1 {
			if filterVersion != "" {
				if _, hit := versions[filterVersion]; !hit {
					continue
				}
			}
			list := sortedKeys(versions)
			multi = append(multi, multiVersionSnapshot{
				Code: snap.Code, Version: snap.Version, Kind: snap.Kind, Algorithm: snap.Algorithm,
				NormTableVersions: list,
			})
		}
	}

	usages := make([]usageEntry, 0, len(usagesByNorm))
	referenced := map[string]struct{}{}
	for version, byModel := range usagesByNorm {
		referenced[version] = struct{}{}
		models := make([]modelUsage, 0, len(byModel))
		for _, acc := range byModel {
			models = append(models, modelUsage{
				Code: acc.meta.code, Version: acc.meta.version,
				Kind: acc.meta.kind, Algorithm: acc.meta.algorithm,
				FactorCodes: sortedKeys(acc.factors),
			})
		}
		sort.Slice(models, func(i, j int) bool {
			if models[i].Code != models[j].Code {
				return models[i].Code < models[j].Code
			}
			return models[i].Version < models[j].Version
		})
		usages = append(usages, usageEntry{NormTableVersion: version, Models: models})
	}
	sort.Slice(usages, func(i, j int) bool {
		return usages[i].NormTableVersion < usages[j].NormTableVersion
	})

	var unreferenced []string
	for v := range normSet {
		if filterVersion != "" && v != filterVersion {
			continue
		}
		if _, ok := referenced[v]; ok {
			continue
		}
		unreferenced = append(unreferenced, v)
	}
	sort.Strings(unreferenced)

	sort.Slice(dangling, func(i, j int) bool {
		if dangling[i].NormTableVersion != dangling[j].NormTableVersion {
			return dangling[i].NormTableVersion < dangling[j].NormTableVersion
		}
		if dangling[i].Code != dangling[j].Code {
			return dangling[i].Code < dangling[j].Code
		}
		return dangling[i].FactorCode < dangling[j].FactorCode
	})
	sort.Slice(multi, func(i, j int) bool {
		if multi[i].Code != multi[j].Code {
			return multi[i].Code < multi[j].Code
		}
		return multi[i].Version < multi[j].Version
	})

	return report{
		PublishedScanned:      len(snaps),
		PublishedWithRefs:     publishedWithRefs,
		NormsTotal:            len(normSet),
		UsageCount:            len(usages),
		DanglingCount:         len(dangling),
		UnreferencedCount:     len(unreferenced),
		MultiVersionCount:     len(multi),
		Usages:                usages,
		DanglingRefs:          dangling,
		UnreferencedNorms:     unreferenced,
		MultiVersionSnapshots: multi,
	}
}

func sortedKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func printReport(r report) {
	fmt.Printf("published_scanned=%d published_with_refs=%d norms_total=%d usages=%d dangling=%d unreferenced=%d multi_version=%d\n",
		r.PublishedScanned, r.PublishedWithRefs, r.NormsTotal, r.UsageCount, r.DanglingCount, r.UnreferencedCount, r.MultiVersionCount)
	for _, u := range r.Usages {
		codes := make([]string, 0, len(u.Models))
		for _, m := range u.Models {
			codes = append(codes, fmt.Sprintf("%s@%s", m.Code, m.Version))
		}
		fmt.Printf("  usage %-40s models=%d [%s]\n", u.NormTableVersion, len(u.Models), strings.Join(codes, ", "))
	}
	for _, d := range r.DanglingRefs {
		fmt.Printf("  dangling %s@%s factor=%s missing_norm=%s\n", d.Code, d.Version, d.FactorCode, d.NormTableVersion)
	}
	for _, v := range r.UnreferencedNorms {
		fmt.Printf("  unreferenced %s\n", v)
	}
	for _, m := range r.MultiVersionSnapshots {
		fmt.Printf("  multi_version %s@%s versions=[%s]\n", m.Code, m.Version, strings.Join(m.NormTableVersions, ", "))
	}
}
