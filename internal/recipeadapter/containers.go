package recipeadapter

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func docker(ctx context.Context, c Context) (Measurement, error) {
	m := Measurement{Tools: []string{"docker build/inspect/run"}}
	var disposable bool
	if err := c.Option("disposable", &disposable); err != nil || !disposable {
		return m, fmt.Errorf("docker requires disposable=true")
	}
	project, err := c.Path(c.String("project"))
	if err != nil {
		return m, err
	}
	file, err := c.Path(c.String("dockerfile"))
	if err != nil {
		return m, err
	}
	tag := "loopexec-recipe:" + hashBytes([]byte(c.Root + fmt.Sprint(c.Iteration)))[:20]
	r, err := c.Run(ctx, project, c.Tool("docker"), "build", "--network=none", "--pull=false", "--platform", c.String("platform"), "-t", tag, "-f", file, ".")
	if err != nil {
		return m, err
	}
	os.WriteFile(filepath.Join(c.EvidenceDir, "docker-build.log"), []byte(r.Stdout+r.Stderr), 0600)
	m.Add("build", r.ExitCode == 0, "local disposable image")
	if r.ExitCode != 0 {
		return m, nil
	}
	r, err = c.Run(ctx, c.Root, c.Tool("docker"), "image", "inspect", tag)
	if err != nil || r.ExitCode != 0 {
		return m, fmt.Errorf("image inspect failed")
	}
	os.WriteFile(filepath.Join(c.EvidenceDir, "docker-inspect.json"), []byte(r.Stdout), 0600)
	var images []struct {
		ID           string
		Size         int64
		Architecture string
		Os           string
	}
	if err = json.Unmarshal([]byte(r.Stdout), &images); err != nil || len(images) != 1 {
		return m, fmt.Errorf("invalid image identity")
	}
	image := images[0]
	m.Metric("image_bytes", float64(image.Size))
	m.Add("size", float64(image.Size) <= c.Number("max_image_bytes"), image.ID)
	m.Add("platform", c.String("platform") == image.Os+"/"+image.Architecture, "observed image platform")
	for i, check := range c.Strings("checks") {
		argv, err := CommandWords(check)
		if err != nil {
			return m, err
		}
		command := append([]string{c.Tool("docker"), "run", "--rm", "--network=none", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", image.ID}, argv...)
		if err = commandCheck(ctx, c, &m, fmt.Sprintf("container-check-%d", i), command, c.Root); err != nil {
			return m, err
		}
	}
	return m, nil
}
func infrastructure(ctx context.Context, c Context, apply bool) (Measurement, error) {
	m := Measurement{Tools: []string{"terraform plan/show/state readback"}}
	var disposable bool
	if err := c.Option("disposable", &disposable); err != nil || !disposable {
		return m, fmt.Errorf("terraform adapter requires explicit disposable sandbox")
	}
	var identity string
	if err := c.Option("sandbox_identity", &identity); err != nil || identity != c.String("sandbox") {
		return m, fmt.Errorf("sandbox identity mismatch")
	}
	dir, err := c.Path(c.String("configuration"))
	if err != nil {
		return m, err
	}
	plan := filepath.Join(c.EvidenceDir, "terraform-plan")
	argv, err := CommandWords(c.String("drift_command"))
	if err != nil {
		return m, err
	}
	if len(argv) == 0 || filepath.Base(argv[0]) != "terraform" {
		return m, fmt.Errorf("built-in drift adapter requires terraform plan")
	}
	// Pin the read-only operation and output, rather than trusting an arbitrary command.
	if len(argv) != 2 || argv[1] != "plan" {
		return m, fmt.Errorf("drift_command must be literal terraform plan")
	}
	r, err := c.Run(ctx, dir, c.Tool("terraform"), "plan", "-input=false", "-refresh=true", "-detailed-exitcode", "-no-color", "-out="+plan)
	if err != nil {
		return m, err
	}
	os.WriteFile(filepath.Join(c.EvidenceDir, "terraform-plan.log"), []byte(r.Stdout+r.Stderr), 0600)
	if r.ExitCode != 0 && r.ExitCode != 2 {
		return m, fmt.Errorf("terraform plan failed")
	}
	r, err = c.Run(ctx, dir, c.Tool("terraform"), "show", "-json", plan)
	if err != nil || r.ExitCode != 0 {
		return m, fmt.Errorf("terraform plan readback failed")
	}
	os.WriteFile(filepath.Join(c.EvidenceDir, "terraform-plan.json"), []byte(r.Stdout), 0600)
	var p struct {
		ResourceChanges []struct {
			Address string
			Type    string
			Change  struct{ Actions []string }
		} `json:"resource_changes"`
	}
	if err = json.Unmarshal([]byte(r.Stdout), &p); err != nil {
		return m, err
	}
	allowed := map[string]bool{}
	for _, v := range c.Strings("allowed_changes") {
		allowed[v] = true
	}
	count := 0
	for _, resource := range p.ResourceChanges {
		actions := strings.Join(resource.Change.Actions, ",")
		if actions == "no-op" || actions == "read" {
			continue
		}
		count++
		if resource.Type != "terraform_data" || !allowed[resource.Address+":"+actions] {
			return m, fmt.Errorf("sandbox action outside pinned policy: %s:%s", resource.Address, actions)
		}
	}
	m.Metric("drift_count", float64(count))
	m.Add("drift-zero", count == 0, "fresh plan against actual local state")
	if apply && count > 0 {
		r, err = c.Run(ctx, dir, c.Tool("terraform"), "apply", "-input=false", "-no-color", plan)
		if err != nil || r.ExitCode != 0 {
			return m, fmt.Errorf("sandbox reconciliation failed")
		}
		os.WriteFile(filepath.Join(c.EvidenceDir, "terraform-apply.log"), []byte(r.Stdout+r.Stderr), 0600)
	}
	return m, nil
}
