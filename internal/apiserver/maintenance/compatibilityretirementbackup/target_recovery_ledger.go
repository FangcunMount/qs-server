package compatibilityretirementbackup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"syscall"
)

type targetJournalStamp struct {
	device, inode           uint64
	mode                    os.FileMode
	owner                   uint32
	links                   uint64
	size, modified, changed int64
}
type targetJournalAsset struct {
	hash  string
	stamp targetJournalStamp
}
type targetRecoveryJournal struct {
	self  *targetRecoveryJournal
	dir   string
	inode targetJournalStamp
	files map[string]targetJournalAsset
}

func targetStamp(st os.FileInfo) (targetJournalStamp, error) {
	if st == nil {
		return targetJournalStamp{}, ErrRecoveryJournal
	}
	s, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		return targetJournalStamp{}, ErrRecoveryJournal
	}
	changed, e := targetChangeTime(s)
	if e != nil {
		return targetJournalStamp{}, e
	}
	return targetJournalStamp{uint64(s.Dev), uint64(s.Ino), st.Mode(), s.Uid, uint64(s.Nlink), st.Size(), st.ModTime().UnixNano(), changed}, nil
}
func newTargetJournal(dir string, r TargetRecoveryRequest) (*targetRecoveryJournal, error) {
	if privateDirectory(dir) != nil {
		return nil, ErrRecoveryJournal
	}
	st, e := os.Lstat(dir)
	if e != nil {
		return nil, ErrRecoveryJournal
	}
	stamp, e := targetStamp(st)
	if e != nil || stamp.owner != uint32(os.Geteuid()) {
		return nil, ErrRecoveryJournal
	}
	entries, e := os.ReadDir(dir)
	if e != nil || len(entries) != 0 {
		return nil, ErrRecoveryJournal
	}
	j := &targetRecoveryJournal{dir: dir, inode: stamp, files: map[string]targetJournalAsset{}}
	j.self = j
	if _, e = j.write("binding", r); e != nil {
		return nil, e
	}
	return j, nil
}
func (j *targetRecoveryJournal) validate() error {
	if j == nil || j.self != j || privateDirectory(j.dir) != nil {
		return ErrRecoveryJournal
	}
	st, e := os.Lstat(j.dir)
	if e != nil {
		return ErrRecoveryJournal
	}
	now, e := targetStamp(st)
	if e != nil || now.device != j.inode.device || now.inode != j.inode.inode || now.owner != j.inode.owner || now.mode != j.inode.mode {
		return ErrRecoveryJournal
	}
	entries, e := os.ReadDir(j.dir)
	if e != nil || len(entries) != len(j.files) {
		return ErrRecoveryJournal
	}
	for _, entry := range entries {
		want, ok := j.files[entry.Name()]
		if !ok {
			return ErrRecoveryJournal
		}
		f, e := openPrivateFile(filepath.Join(j.dir, entry.Name()))
		if e != nil {
			return ErrRecoveryJournal
		}
		before, e := f.Stat()
		if e != nil {
			_ = f.Close()
			return ErrRecoveryJournal
		}
		raw, e := readPrivate(f, metadataBudget)
		after, se := f.Stat()
		ce := f.Close()
		bs, be := targetStamp(before)
		as, ae := targetStamp(after)
		if e != nil || se != nil || ce != nil || be != nil || ae != nil || bs != as || bs != want.stamp || bs.owner != uint32(os.Geteuid()) || bs.links != 1 || sha(raw) != want.hash {
			return ErrRecoveryJournal
		}
	}
	return nil
}
func (j *targetRecoveryJournal) write(name string, value any) (string, error) {
	if j == nil || j.self != j || filepath.Base(name) != name || name == "" {
		return "", ErrRecoveryJournal
	}
	if len(j.files) > 0 {
		if e := j.validate(); e != nil {
			return "", e
		}
	}
	name = "target-recovery-" + name + ".json"
	if _, ok := j.files[name]; ok {
		return "", ErrRecoveryUnknown
	}
	raw, e := json.Marshal(value)
	if e != nil || len(raw) > 64<<10 {
		return "", ErrRecoveryJournal
	}
	f, e := newPrivateFile(filepath.Join(j.dir, name))
	if e != nil {
		return "", ErrRecoveryUnknown
	}
	n, we := f.Write(raw)
	sy := f.Sync()
	st, se := f.Stat()
	ce := f.Close()
	if we != nil || n != len(raw) || sy != nil || se != nil || ce != nil || syncDirectory(j.dir) != nil {
		return "", ErrRecoveryUnknown
	}
	stamp, e := targetStamp(st)
	if e != nil || stamp.owner != uint32(os.Geteuid()) || stamp.links != 1 || !stamp.mode.IsRegular() || stamp.mode.Perm() != 0600 {
		return "", ErrRecoveryUnknown
	}
	digest := sha(raw)
	j.files[name] = targetJournalAsset{digest, stamp}
	if j.validate() != nil {
		return "", ErrRecoveryUnknown
	}
	return digest, nil
}
func (j *targetRecoveryJournal) has(name, hash string) bool {
	if j == nil || j.self != j {
		return false
	}
	a, ok := j.files["target-recovery-"+name+".json"]
	return ok && a.hash == hash
}

// Journal JSON is reconciliation material, never a reconstructed capability.
type targetStatementRecord struct {
	RequestSHA256   string `json:"request_sha256"`
	Index           int    `json:"target_index"`
	Database        string `json:"database"`
	Name            string `json:"name"`
	Phase           string `json:"phase"`
	StatementSHA256 string `json:"statement_sha256"`
	SQLConnectionID string `json:"sql_connection_id,omitempty"`
	Result          string `json:"result"`
	Records         uint64 `json:"records"`
}

func targetRecord(p *TargetRecoveryPlan, i int, phase, statement, result string, n uint64) targetStatementRecord {
	return targetStatementRecord{jsonSHA(p.request), i, p.observations[i].Database, targetNames[i], phase, sha([]byte(statement)), p.sqlConnection, result, n}
}

// Linux/Darwin expose different Stat_t timestamp names. Missing native
// metadata is an error, never replaced with mtime or wall-clock observations.
func targetChangeTime(st *syscall.Stat_t) (int64, error) {
	value := reflect.ValueOf(st).Elem()
	for _, name := range []string{"Ctim", "Ctimespec"} {
		field := value.FieldByName(name)
		if !field.IsValid() {
			continue
		}
		sec, nano := field.FieldByName("Sec"), field.FieldByName("Nsec")
		if !sec.IsValid() || !nano.IsValid() || !sec.CanInt() || !nano.CanInt() {
			return 0, ErrRecoveryJournal
		}
		if sec.Int() < 0 || nano.Int() < 0 || nano.Int() >= 1000000000 || sec.Int() > 9223372036 {
			return 0, ErrRecoveryJournal
		}
		return sec.Int()*1000000000 + nano.Int(), nil
	}
	return 0, ErrRecoveryJournal
}
