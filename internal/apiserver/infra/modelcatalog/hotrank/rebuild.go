package hotrank

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/hotrank"
	"github.com/google/uuid"
	redis "github.com/redis/go-redis/v9"
)

const MaxRebuildFacts = 1000
const rebuildLeaseTTL = 60 * time.Second

var (
	ErrRebuildInProgress     = errors.New("hot rank day rebuild is in progress; retry original event")
	ErrRebuildAlreadyApplied = errors.New("hot rank rebuild request was already applied; inspect original receipt")
	ErrRebuildLeaseLost      = errors.New("hot rank rebuild lease expired or changed")
	ErrRebuildConflict       = errors.New("hot rank rebuild identity or Redis state conflict")
)
var releaseRebuildScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
 return redis.call("DEL", KEYS[1])
end
return 0
`)
var acquireRebuildScript = redis.NewScript(`
if redis.call("EXISTS", KEYS[2]) == 1 then return 2 end
if redis.call("SET", KEYS[1], ARGV[1], "NX", "PX", ARGV[2]) then return 1 end
return 0
`)

// All inputs, types and the receipt conflict are checked before the first write.
// Lua provides isolation, not rollback of arbitrary Redis runtime errors.
var applyRebuildScript = redis.NewScript(`
local previous = redis.call("GET", KEYS[3])
if previous then
 local ok, receipt = pcall(cjson.decode, previous)
 if not ok or receipt.fingerprint ~= ARGV[2] or receipt.operator ~= ARGV[6] then return -2 end
 return 0
end
if redis.call("GET", KEYS[2]) ~= ARGV[1] then return -1 end
local kind = redis.call("TYPE", KEYS[1]).ok
if kind ~= "none" and kind ~= "zset" then return -2 end
local ttl = tonumber(ARGV[3])
if not ttl or ttl < 1 or ttl > 34560000 then return -2 end
local counts = cjson.decode(ARGV[4])
if not next(counts) and kind == "zset" and redis.call("ZCARD", KEYS[1]) > 0 then return -2 end
for member, count in pairs(counts) do
 if type(member) ~= "string" or member == "" or type(count) ~= "number" or count < 1 or count > 1000 or count ~= math.floor(count) then return -2 end
end
for index = 4, #KEYS do
 local t = redis.call("TYPE", KEYS[index]).ok
 if t ~= "none" and t ~= "string" then return -2 end
 local v = redis.call("GET", KEYS[index])
 if v and v ~= "1" then return -2 end
end
redis.call("DEL", KEYS[1])
for member, count in pairs(counts) do redis.call("ZADD", KEYS[1], count, member) end
if next(counts) then redis.call("EXPIRE", KEYS[1], ttl) end
for index = 4, #KEYS do redis.call("SET", KEYS[index], "1", "EX", ttl) end
redis.call("SET", KEYS[3], ARGV[5], "EX", ttl)
redis.call("DEL", KEYS[2])
return 1
`)

// RebuildSnapshot must contain a complete day, never a selected account or ID page.
// Only the original durable acceptance and matching original Outbox can supply it.
type RebuildSnapshot struct {
	Day                  string                   `json:"day"`
	CapturedAt           time.Time                `json:"captured_at"`
	Facts                []hotrank.SubmissionFact `json:"-"`
	OriginalFingerprints map[string]string        `json:"-"`
	Complete             bool                     `json:"complete"`
}
type RebuildLease struct {
	Day, RequestID, Token string
	acquiredAt            time.Time
}
type RebuildReceipt struct {
	Day              string         `json:"day"`
	RequestID        string         `json:"request_id"`
	Operator         string         `json:"operator"`
	Fingerprint      string         `json:"fingerprint"`
	SourceCapturedAt time.Time      `json:"source_captured_at"`
	FactCount        int            `json:"fact_count"`
	Counts           map[string]int `json:"counts"`
	AppliedAt        time.Time      `json:"applied_at"`
}

func rebuildDay(day string, now time.Time) (time.Time, error) {
	location := hotRankTimezone
	start, err := time.ParseInLocation("20060102", day, location)
	current := now.In(location)
	today := time.Date(current.Year(), current.Month(), current.Day(), 0, 0, 0, 0, location)
	if err != nil || start.Format("20060102") != day || start.After(now) || start.Before(today.AddDate(0, 0, -399)) {
		return time.Time{}, fmt.Errorf("rebuild requires a valid UTC+8 day within the retained window")
	}
	return start, nil
}
func (r *RedisScaleHotRankProjection) AcquireRebuild(ctx context.Context, day, requestID string) (RebuildLease, error) {
	if r == nil || r.client == nil || r.keys == nil {
		return RebuildLease{}, fmt.Errorf("hot rank rebuild store is unavailable")
	}
	if _, err := rebuildDay(day, r.now()); err != nil {
		return RebuildLease{}, err
	}
	if id, err := uuid.Parse(requestID); err != nil || id.String() != requestID {
		return RebuildLease{}, fmt.Errorf("canonical rebuild request UUID required")
	}
	lease := RebuildLease{Day: day, RequestID: requestID, Token: uuid.NewString(), acquiredAt: r.now()}
	daily := r.keys.BuildScaleHotDailyKey(day)
	result, err := acquireRebuildScript.Run(ctx, r.client, []string{daily + ":rebuild-lock", daily + ":rebuild-receipt:" + requestID}, lease.Token, rebuildLeaseTTL.Milliseconds()).Int64()
	if err != nil {
		return RebuildLease{}, err
	}
	switch result {
	case 1:
		return lease, nil
	case 2:
		return RebuildLease{}, ErrRebuildAlreadyApplied
	default:
		return RebuildLease{}, ErrRebuildInProgress
	}
}
func (r *RedisScaleHotRankProjection) ReleaseRebuild(ctx context.Context, lease RebuildLease) error {
	if lease.Token == "" {
		return fmt.Errorf("rebuild lease token required")
	}
	return releaseRebuildScript.Run(ctx, r.client, []string{r.keys.BuildScaleHotDailyKey(lease.Day) + ":rebuild-lock"}, lease.Token).Err()
}
func (r *RedisScaleHotRankProjection) RebuildReceipt(ctx context.Context, day, requestID string) (*RebuildReceipt, error) {
	if _, err := rebuildDay(day, r.now()); err != nil {
		return nil, err
	}
	if id, err := uuid.Parse(requestID); err != nil || id.String() != requestID {
		return nil, fmt.Errorf("canonical rebuild request UUID required")
	}
	value, err := r.client.Get(ctx, r.keys.BuildScaleHotDailyKey(day)+":rebuild-receipt:"+requestID).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var receipt RebuildReceipt
	if err := json.Unmarshal(value, &receipt); err != nil || receipt.Day != day || receipt.RequestID != requestID || len(receipt.Fingerprint) != 64 {
		return nil, ErrRebuildConflict
	}
	return &receipt, nil
}

// ApplyDayRebuild sets absolute source counts and represented original IDs together.
// A lost response is resolved from the original request receipt; repeating an
// applied snapshot never overwrites submissions that arrived after the repair.
func (r *RedisScaleHotRankProjection) ApplyDayRebuild(ctx context.Context, lease RebuildLease, snapshot RebuildSnapshot, operator string) (*RebuildReceipt, error) {
	if lease.Token == "" || lease.acquiredAt.IsZero() || snapshot.CapturedAt.Before(lease.acquiredAt) || lease.Day != snapshot.Day || strings.TrimSpace(operator) == "" || len(operator) > 128 {
		return nil, fmt.Errorf("rebuild lease, source day and operator required")
	}
	if _, err := rebuildDay(lease.Day, r.now()); err != nil {
		return nil, err
	}
	if !snapshot.Complete || snapshot.CapturedAt.IsZero() || snapshot.CapturedAt.After(r.now()) || len(snapshot.Facts) > MaxRebuildFacts || len(snapshot.OriginalFingerprints) != len(snapshot.Facts) {
		return nil, fmt.Errorf("complete bounded original source snapshot required")
	}
	fingerprint, counts, facts, err := snapshotManifest(snapshot)
	if err != nil {
		return nil, err
	}
	daily := r.keys.BuildScaleHotDailyKey(lease.Day)
	keys := []string{daily, daily + ":rebuild-lock", daily + ":rebuild-receipt:" + lease.RequestID}
	for _, fact := range facts {
		keys = append(keys, r.keys.BuildScaleHotProjectedKey(fact.EventID))
	}
	receipt := RebuildReceipt{Day: lease.Day, RequestID: lease.RequestID, Operator: operator, Fingerprint: fingerprint, SourceCapturedAt: snapshot.CapturedAt, FactCount: len(facts), Counts: counts, AppliedAt: r.now().UTC()}
	encoded, err := json.Marshal(receipt)
	if err != nil {
		return nil, err
	}
	encodedCounts, err := json.Marshal(counts)
	if err != nil {
		return nil, err
	}
	result, err := applyRebuildScript.Run(ctx, r.client, keys, lease.Token, fingerprint, int64(defaultScaleHotRankRetention.Seconds()), string(encodedCounts), string(encoded), operator).Int64()
	if err != nil {
		return nil, err
	}
	switch result {
	case -1:
		return nil, ErrRebuildLeaseLost
	case -2:
		return nil, ErrRebuildConflict
	case 0:
		return r.RebuildReceipt(ctx, lease.Day, lease.RequestID)
	case 1:
		return &receipt, nil
	default:
		return nil, ErrRebuildConflict
	}
}

// Manifest is the approval identity for a complete source snapshot. Capture time
// does not change its fingerprint; original event content and business day do.
func (s RebuildSnapshot) Manifest() (string, map[string]int, error) {
	fingerprint, counts, _, err := snapshotManifest(s)
	return fingerprint, counts, err
}
func snapshotManifest(snapshot RebuildSnapshot) (string, map[string]int, []hotrank.SubmissionFact, error) {
	if !snapshot.Complete || len(snapshot.Facts) > MaxRebuildFacts || len(snapshot.OriginalFingerprints) != len(snapshot.Facts) {
		return "", nil, nil, fmt.Errorf("complete bounded source required")
	}
	facts := append([]hotrank.SubmissionFact(nil), snapshot.Facts...)
	sort.Slice(facts, func(i, j int) bool { return facts[i].EventID < facts[j].EventID })
	type identity struct {
		EventID, QuestionnaireCode string
		SubmittedAt                time.Time
		OriginalFingerprint        string
	}
	identities := make([]identity, 0, len(facts))
	counts := map[string]int{}
	for i, f := range facts {
		if id, err := uuid.Parse(f.EventID); err != nil || id.String() != f.EventID {
			return "", nil, nil, ErrRebuildConflict
		}
		fingerprint, ok := snapshot.OriginalFingerprints[f.EventID]
		decoded, err := hex.DecodeString(fingerprint)
		if !ok || err != nil || len(decoded) != 32 || strings.TrimSpace(f.QuestionnaireCode) == "" || f.QuestionnaireCode != strings.TrimSpace(f.QuestionnaireCode) || f.SubmittedAt.IsZero() || f.SubmittedAt.In(hotRankTimezone).Format("20060102") != snapshot.Day || (i > 0 && facts[i-1].EventID == f.EventID) {
			return "", nil, nil, ErrRebuildConflict
		}
		counts[f.QuestionnaireCode]++
		identities = append(identities, identity{f.EventID, f.QuestionnaireCode, f.SubmittedAt.UTC(), fingerprint})
	}
	canonical, err := json.Marshal(struct {
		Day   string
		Facts []identity
	}{snapshot.Day, identities})
	if err != nil {
		return "", nil, nil, err
	}
	hash := sha256.Sum256(canonical)
	fingerprint := hex.EncodeToString(hash[:])
	return fingerprint, counts, facts, nil
}
