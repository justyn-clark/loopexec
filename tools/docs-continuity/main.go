// docs-continuity binds public documentation to the exact source being released.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
)

const officialSite = "https://loopexec.dev"

type fileDigest struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type manifest struct {
	SchemaVersion       int          `json:"schema_version"`
	Version             string       `json:"version"`
	SourceSHA256        string       `json:"source_sha256"`
	DocumentationSHA256 string       `json:"documentation_sha256"`
	Files               []fileDigest `json:"files"`
}

func decode(data []byte, value any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("expected exactly one JSON document")
	}
	return nil
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func safeRead(root, path string) ([]byte, error) {
	if !fs.ValidPath(path) || strings.Contains(path, "\\") {
		return nil, fmt.Errorf("unsafe path %q", path)
	}
	current := root
	for _, part := range strings.Split(path, "/") {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("symlink forbidden: %s", path)
		}
	}
	info, err := os.Stat(current)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 4<<20 {
		return nil, fmt.Errorf("not a bounded regular file: %s", path)
	}
	return os.ReadFile(current)
}

func sourceVersion(root string) (string, error) {
	data, err := safeRead(root, "cmd/loopexec/main.go")
	if err != nil {
		return "", err
	}
	f, err := parser.ParseFile(token.NewFileSet(), "main.go", data, 0)
	if err != nil {
		return "", err
	}
	for _, decl := range f.Decls {
		g, ok := decl.(*ast.GenDecl)
		if !ok || g.Tok != token.CONST {
			continue
		}
		for _, spec := range g.Specs {
			v := spec.(*ast.ValueSpec)
			for i, name := range v.Names {
				if name.Name == "toolVersion" && i < len(v.Values) {
					lit, ok := v.Values[i].(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						return "", errors.New("toolVersion must be a string constant")
					}
					return strconv.Unquote(lit.Value)
				}
			}
		}
	}
	return "", errors.New("missing toolVersion")
}

func hashFiles(root string, paths []string) ([]fileDigest, string, error) {
	sort.Strings(paths)
	files := make([]fileDigest, 0, len(paths))
	var binding bytes.Buffer
	for i, path := range paths {
		if i > 0 && path == paths[i-1] {
			return nil, "", fmt.Errorf("duplicate file: %s", path)
		}
		data, err := safeRead(root, path)
		if err != nil {
			return nil, "", err
		}
		hash := digest(data)
		files = append(files, fileDigest{path, hash})
		fmt.Fprintf(&binding, "%s\x00%s\n", path, hash)
	}
	return files, digest(binding.Bytes()), nil
}

func sourcePaths(root string) ([]string, error) {
	paths := []string{"go.mod", "go.sum", "AGENTS.md"}
	for _, dir := range []string{"cmd", "internal", "examples", "scripts", "tools", ".github/workflows"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				switch entry.Name() {
				case "vendor", "node_modules", "build", ".loopexec", ".small", ".small-cache":
					return filepath.SkipDir
				}
				return nil
			}
			switch filepath.Ext(path) {
			case ".go", ".sh", ".json", ".yaml", ".yml", ".toml":
				relative, err := filepath.Rel(root, path)
				if err != nil {
					return err
				}
				paths = append(paths, filepath.ToSlash(relative))
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return paths, nil
}

func makeManifest(root string) (manifest, error) {
	var m manifest
	registry, err := safeRead(root, "docs/documentation-files.json")
	if err != nil {
		return m, err
	}
	var allowlist struct {
		Files []string `json:"files"`
	}
	if err := decode(registry, &allowlist); err != nil {
		return m, err
	}
	m.SchemaVersion = 1
	m.Version, err = sourceVersion(root)
	if err != nil {
		return m, err
	}
	required := map[string]bool{"README.md": false, "SPEC.md": false, "CHANGELOG.md": false, "docs/documentation-files.json": false, "docs/cli.md": false, "docs/cli-reference.generated.md": false, "docs/documentation-continuity.md": false, "docs/releases/v" + m.Version + ".md": false, "examples/scoped-budget/README.md": false}
	for _, path := range allowlist.Files {
		if !publicPath(path) {
			return m, fmt.Errorf("non-public documentation path: %s", path)
		}
		if _, ok := required[path]; ok {
			required[path] = true
		}
	}
	for path, present := range required {
		if !present {
			return m, fmt.Errorf("required public documentation missing from allowlist: %s", path)
		}
	}
	m.Files, m.DocumentationSHA256, err = hashFiles(root, allowlist.Files)
	if err != nil {
		return m, err
	}
	sources, err := sourcePaths(root)
	if err != nil {
		return m, err
	}
	_, m.SourceSHA256, err = hashFiles(root, sources)
	return m, err
}

func publicPath(path string) bool {
	switch path {
	case "LICENSE", "README.md", "SPEC.md", "CHANGELOG.md", "docs/documentation-files.json", "examples/scoped-budget/README.md":
		return true
	}
	return fs.ValidPath(path) && strings.HasPrefix(path, "docs/") &&
		strings.HasSuffix(path, ".md") && !strings.Contains(path, "handoff") &&
		!strings.Contains(path, "/.") && !strings.Contains(path, "\\")
}

func verifyBundle(root, out string) (manifest, error) {
	var saved manifest
	data, err := safeRead(out, "manifest.json")
	if err != nil {
		return saved, err
	}
	if err := decode(data, &saved); err != nil {
		return saved, err
	}
	expected, err := makeManifest(root)
	if err != nil {
		return saved, err
	}
	if !reflect.DeepEqual(saved, expected) {
		return saved, errors.New("local documentation manifest differs from source")
	}
	for _, file := range saved.Files {
		data, err := safeRead(out, file.Path)
		if err != nil {
			return saved, err
		}
		if digest(data) != file.SHA256 {
			return saved, fmt.Errorf("bundled documentation differs: %s", file.Path)
		}
	}
	return saved, nil
}

func writeBundle(root, out string) error {
	m, err := makeManifest(root)
	if err != nil {
		return err
	}
	if err := os.Mkdir(out, 0755); err != nil {
		return fmt.Errorf("bundle output must not exist: %w", err)
	}
	for _, file := range m.Files {
		data, err := safeRead(root, file.Path)
		if err != nil {
			return err
		}
		if digest(data) != file.SHA256 {
			return fmt.Errorf("documentation changed during bundling: %s", file.Path)
		}
		path := filepath.Join(out, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, "manifest.json"), append(data, '\n'), 0644)
}

func fetch(client *http.Client, url string) ([]byte, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
	if err == nil && len(data) > 4<<20 {
		err = errors.New("site response exceeds limit")
	}
	return data, err
}

func siteMarker(m manifest) string {
	return fmt.Sprintf("<!-- loopexec-docs version=%s source=%s documentation=%s -->", m.Version, m.SourceSHA256, m.DocumentationSHA256)
}

func verifySite(client *http.Client, base string, expected manifest) error {
	data, err := fetch(client, base+"/loopexec-documentation.json")
	if err != nil {
		return err
	}
	var actual manifest
	if err := decode(data, &actual); err != nil {
		return err
	}
	if !reflect.DeepEqual(actual, expected) {
		return errors.New("deployed documentation manifest does not match release source and docs")
	}
	home, err := fetch(client, base+"/")
	if err != nil {
		return err
	}
	if !bytes.Contains(home, []byte(siteMarker(expected))) {
		return errors.New("deployed homepage lacks the matching documentation build marker")
	}
	for _, file := range expected.Files {
		data, err := fetch(client, base+"/loopexec-documentation/"+file.Path)
		if err != nil {
			return err
		}
		if digest(data) != file.SHA256 {
			return fmt.Errorf("deployed documentation differs: %s", file.Path)
		}
	}
	return nil
}

func run(args []string) error {
	if len(args) != 2 {
		return errors.New("usage: docs-continuity bundle <new-output-directory> | verify-bundle <directory> | verify-site <manifest.json> | marker <manifest.json>")
	}
	if args[0] == "bundle" {
		return writeBundle(".", args[1])
	}
	if args[0] == "verify-bundle" {
		_, err := verifyBundle(".", args[1])
		return err
	}
	data, err := os.ReadFile(args[1])
	if err != nil {
		return err
	}
	var m manifest
	if err := decode(data, &m); err != nil {
		return err
	}
	if args[0] == "marker" {
		fmt.Println(siteMarker(m))
		return nil
	}
	if args[0] != "verify-site" {
		return errors.New("unknown documentation command")
	}
	if filepath.Base(args[1]) != "manifest.json" {
		return errors.New("expected bundle manifest.json")
	}
	if _, err := verifyBundle(".", filepath.Dir(args[1])); err != nil {
		return err
	}
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 4 || req.URL.Scheme != "https" || req.URL.Host != "loopexec.dev" {
			return errors.New("documentation redirect must remain on the official HTTPS origin")
		}
		return nil
	}}
	return verifySite(client, officialSite, m)
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "DOCS_CONTINUITY_FAILED:", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "DOCS_CONTINUITY_OK")
}
