package compose

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/vazra/simpledeploy/internal/fsutil"
)

// maxExtendsDepth limits how many extends files deep the raw check follows.
const maxExtendsDepth = 8

const includeViolation = `"include" is not supported: put all services in docker-compose.yml`

// checkRawFileRefs refuses compose content that would make the loader read
// files on its own before the validator runs: a top-level include, and
// label_file and extends.file values of services and jobs (and label_file
// of services' pre_start hooks) that are not regular files inside appDir.
// Extends files are checked the same way, recursively.
func checkRawFileRefs(content []byte, appDir string) error {
	c := &rawRefChecker{appDir: appDir, seen: map[string]bool{}}
	if real, err := filepath.EvalSymlinks(appDir); err == nil {
		c.appReal = real
	}
	if err := c.checkContent(content, appDir, true, 0); err != nil {
		return fmt.Errorf("parse compose: %w", err)
	}
	if len(c.violations) > 0 {
		return &ViolationError{Violations: c.violations}
	}
	return nil
}

type rawRefChecker struct {
	appDir     string
	appReal    string // appDir with symlinks resolved; "" when it does not exist
	seen       map[string]bool
	violations []string
}

func (c *rawRefChecker) add(format string, args ...any) {
	c.violations = append(c.violations, fmt.Sprintf(format, args...))
}

// checkContent checks every YAML document in content. dir is the folder
// relative paths resolve from; top is true for docker-compose.yml itself.
func (c *rawRefChecker) checkContent(content []byte, dir string, top bool, depth int) error {
	dec := yaml.NewDecoder(bytes.NewReader(content))
	for {
		var doc any
		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		c.checkDoc(doc, dir, top, depth)
	}
}

func (c *rawRefChecker) checkDoc(doc any, dir string, top bool, depth int) {
	root := rawMap(doc)
	if root == nil {
		return
	}
	if _, ok := root["include"]; ok && top {
		c.add(includeViolation)
	}
	for _, section := range rawContainerSections {
		entries := rawMap(root[section.key])
		for _, name := range sortedKeys(entries) {
			entry := rawMap(entries[name])
			if entry == nil {
				continue
			}
			who := fmt.Sprintf("%s %q", section.kind, name)
			c.checkLabelFiles(who, entry["label_file"], dir, depth)
			// extends: <service> and extends without file stay in this file.
			if ext := rawMap(entry["extends"]); ext != nil {
				if f, ok := ext["file"]; ok && f != nil {
					c.checkFile(who, "extends file", f, dir, true, depth)
				}
			}
			if hooks, ok := entry["pre_start"].([]any); ok {
				for i, hook := range hooks {
					if hm := rawMap(hook); hm != nil {
						c.checkLabelFiles(fmt.Sprintf("%s pre_start hook %d", who, i+1), hm["label_file"], dir, depth)
					}
				}
			}
		}
	}
}

// rawContainerSections are the top-level sections whose entries run
// containers and may name label_file and extends files.
var rawContainerSections = []struct{ key, kind string }{
	{"services", "service"},
	{"jobs", "job"},
}

// checkLabelFiles checks a label_file value: one path or a list of paths.
func (c *rawRefChecker) checkLabelFiles(who string, v any, dir string, depth int) {
	switch lf := v.(type) {
	case nil:
	case []any:
		for _, item := range lf {
			c.checkFile(who, "label_file", item, dir, false, depth)
		}
	default:
		c.checkFile(who, "label_file", lf, dir, false, depth)
	}
}

// checkFile refuses a file reference unless it is a plain relative path to
// a regular file inside the app folder (after resolving symlinks). who
// names the owner for messages, e.g. `service "web"`.
func (c *rawRefChecker) checkFile(who, kind string, v any, dir string, extends bool, depth int) {
	p, ok := rawScalar(v)
	if !ok {
		c.add("%s: %s must be a path", who, kind)
		return
	}
	bad := func() { c.add("%s: %s %q must be a file inside the app folder", who, kind, p) }
	if p == "" || filepath.IsAbs(p) || strings.Contains(p, "..") || strings.ContainsAny(p, "~$") || c.appReal == "" {
		bad()
		return
	}
	full := filepath.Join(dir, p)
	real, err := filepath.EvalSymlinks(full)
	if err != nil || !within(real, c.appReal) {
		bad()
		return
	}
	if fi, err := os.Stat(real); err != nil || !fi.Mode().IsRegular() {
		bad()
		return
	}
	if !extends || c.seen[real] {
		return
	}
	c.seen[real] = true
	if depth >= maxExtendsDepth {
		c.add("%s: extends files nest too deeply", who)
		return
	}
	data, err := fsutil.ReadRegularFile(real)
	if err != nil {
		bad()
		return
	}
	if err := c.checkContent(data, filepath.Dir(full), false, depth+1); err != nil {
		c.add("%s: extends file %q is not valid YAML", who, p)
	}
}

// stripFileRefs returns content without the keys checkRawFileRefs checks:
// a top-level include, label_file and extends with a file in services and
// jobs (extends within the same file is kept), and pre_start hooks'
// label_file.
func stripFileRefs(content []byte) ([]byte, error) {
	dec := yaml.NewDecoder(bytes.NewReader(content))
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	for {
		var doc any
		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if doc == nil {
			continue
		}
		rawDelete(doc, "include")
		for _, section := range rawContainerSections {
			entries := rawMap(rawMap(doc)[section.key])
			for _, name := range sortedKeys(entries) {
				entry := entries[name]
				rawDelete(entry, "label_file")
				if ext := rawMap(rawMap(entry)["extends"]); ext != nil {
					if f, ok := ext["file"]; ok && f != nil {
						rawDelete(entry, "extends")
					}
				}
				if hooks, ok := rawMap(entry)["pre_start"].([]any); ok {
					for _, hook := range hooks {
						rawDelete(hook, "label_file")
					}
				}
			}
		}
		if err := enc.Encode(doc); err != nil {
			return nil, err
		}
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// rawDelete removes key from v when v is a map.
func rawDelete(v any, key string) {
	switch m := v.(type) {
	case map[string]any:
		delete(m, key)
	case map[any]any:
		for k := range m {
			if s, ok := rawScalar(k); ok && s == key {
				delete(m, k)
			}
		}
	}
}

// rawMap returns v as a string-keyed map, or nil.
func rawMap(v any) map[string]any {
	switch m := v.(type) {
	case map[string]any:
		return m
	case map[any]any:
		out := make(map[string]any, len(m))
		for k, val := range m {
			if s, ok := rawScalar(k); ok {
				out[s] = val
			}
		}
		return out
	}
	return nil
}

func rawScalar(v any) (string, bool) {
	switch s := v.(type) {
	case string:
		return s, true
	case int:
		return strconv.Itoa(s), true
	case int64:
		return strconv.FormatInt(s, 10), true
	case uint64:
		return strconv.FormatUint(s, 10), true
	case float64:
		return strconv.FormatFloat(s, 'g', -1, 64), true
	case bool:
		return strconv.FormatBool(s), true
	}
	return "", false
}
