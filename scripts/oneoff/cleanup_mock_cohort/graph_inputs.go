package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"syscall"
)

type graphOriginalInput struct {
	path, sha  string
	max        int64
	info       os.FileInfo
	changeTime any
}
type graphInputGuard struct{ files []graphOriginalInput }

func graphChangeTime(info os.FileInfo) (any, error) {
	v := reflect.ValueOf(info.Sys())
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return nil, errors.New("private input kernel metadata unavailable")
	}
	v = v.Elem()
	for _, name := range []string{"Ctim", "Ctimespec"} {
		field := v.FieldByName(name)
		if field.IsValid() && field.CanInterface() {
			return field.Interface(), nil
		}
	}
	return nil, errors.New("private input change time unavailable")
}
func graphPrivateInfo(path string) (os.FileInfo, any, error) {
	info, e := os.Lstat(path)
	if e != nil {
		return nil, nil, e
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || stat.Nlink != 1 || int(stat.Uid) != os.Geteuid() {
		return nil, nil, errors.New("private original input ownership changed")
	}
	change, e := graphChangeTime(info)
	return info, change, e
}
func newGraphInputGuard(c config, g group) (*graphInputGuard, error) {
	guard := &graphInputGuard{files: []graphOriginalInput{
		{path: c.group, sha: c.groupSHA, max: 256 * 1024 * 1024},
		{path: c.owner, sha: g.OwnerSHA, max: 32 * 1024},
		{path: c.protection, sha: g.ProtectedSHA, max: 32 * 1024 * 1024},
		{path: c.selector, sha: g.QS.Graph.SelectorSHA, max: 16 * 1024 * 1024},
		{path: filepath.Join(c.scopeDir, "graph-index.json"), sha: g.QS.Graph.IndexSHA, max: 16 * 1024 * 1024},
	}}
	if c.action != "capture" && c.action != "plan" {
		guard.files = append(guard.files, graphOriginalInput{path: filepath.Join(c.archive, "material.json"), sha: c.materialSHA, max: 16 * 1024 * 1024})
	}
	for i := range guard.files {
		var e error
		guard.files[i].info, guard.files[i].changeTime, e = graphPrivateInfo(guard.files[i].path)
		if e != nil {
			return nil, e
		}
	}
	return guard, guard.full()
}
func (g *graphInputGuard) check() error {
	for _, file := range g.files {
		current, change, e := graphPrivateInfo(file.path)
		if e != nil {
			return e
		}
		if !os.SameFile(file.info, current) || file.info.Size() != current.Size() || file.info.Mode() != current.Mode() || !file.info.ModTime().Equal(current.ModTime()) || !reflect.DeepEqual(file.changeTime, change) {
			return errors.New("immutable original input changed during action")
		}
	}
	return nil
}
func (g *graphInputGuard) full() error {
	if e := g.check(); e != nil {
		return e
	}
	for _, file := range g.files {
		raw, e := privateRead(file.path, file.max)
		if e != nil {
			return e
		}
		if digest(raw) != file.sha {
			return errors.New("immutable original input raw SHA changed")
		}
	}
	return g.check()
}
func graphFreezeAuthority(c config, g group) error {
	raw, e := privateRead(c.freeze, 32*1024)
	if e != nil {
		return e
	}
	if digest(raw) != c.freezeSHA {
		return errors.New("freeze original changed")
	}
	return validateFreeze(raw, g, c.groupSHA)
}
