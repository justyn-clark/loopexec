package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var updateDocumentation = flag.Bool("update-docs", false, "Regenerate the checked-in CLI reference")

func documentationContract(root *cobra.Command) []byte {
	var out bytes.Buffer
	fmt.Fprintf(&out, "# Generated CLI reference\n\nGenerated from the Cobra command tree. Do not edit manually.\n\nSource version: `%s`.\n\n", root.Version)
	var visit func(*cobra.Command)
	visit = func(cmd *cobra.Command) {
		if cmd.Hidden {
			return
		}
		cmd.InitDefaultHelpFlag()
		cmd.InitDefaultVersionFlag()
		fmt.Fprintf(&out, "## `%s`\n\n%s\n\nUsage: `%s`\n\n", cmd.CommandPath(), cmd.Short, cmd.UseLine())
		if cmd.Long != "" {
			fmt.Fprintf(&out, "%s\n\n", strings.TrimSpace(cmd.Long))
		}
		if len(cmd.Aliases) > 0 {
			fmt.Fprintf(&out, "Aliases: %s\n\n", strings.Join(cmd.Aliases, ", "))
		}
		flags := pflag.NewFlagSet(cmd.Name(), pflag.ContinueOnError)
		flags.AddFlagSet(cmd.LocalNonPersistentFlags())
		flags.AddFlagSet(cmd.PersistentFlags())
		flags.AddFlagSet(cmd.InheritedFlags())
		flags.VisitAll(func(f *pflag.Flag) {
			if f.Hidden {
				return
			}
			name := "--" + f.Name
			if f.Shorthand != "" {
				name += ", -" + f.Shorthand
			}
			fmt.Fprintf(&out, "- `%s` (`%s`, default `%s`): %s", name, f.Value.Type(), f.DefValue, f.Usage)
			if f.Deprecated != "" {
				fmt.Fprintf(&out, " Deprecated: %s", f.Deprecated)
			}
			fmt.Fprintln(&out)
		})
		fmt.Fprintln(&out)
		children := cmd.Commands()
		sort.Slice(children, func(i, j int) bool { return children[i].Name() < children[j].Name() })
		for _, child := range children {
			visit(child)
		}
	}
	visit(root)
	fmt.Fprintln(&out, "## JSON field contract\n\nTypes and JSON tags are generated from the runtime structs. Value semantics and exit behavior are described in cli.md and SPEC.md.")
	queue := []reflect.Type{reflect.TypeOf(response{}), reflect.TypeOf(receiptEvent{})}
	seen := map[reflect.Type]bool{}
	var enqueue func(reflect.Type)
	enqueue = func(typ reflect.Type) {
		switch typ.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
			enqueue(typ.Elem())
		case reflect.Struct:
			if typ.PkgPath() == reflect.TypeOf(response{}).PkgPath() {
				queue = append(queue, typ)
			}
		}
	}
	for len(queue) > 0 {
		typ := queue[0]
		queue = queue[1:]
		if seen[typ] {
			continue
		}
		seen[typ] = true
		fmt.Fprintf(&out, "\n### `%s`\n\n", typ.Name())
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			tag := field.Tag.Get("json")
			if field.PkgPath != "" || tag == "-" {
				continue
			}
			fmt.Fprintf(&out, "- `%s`: `%s`\n", tag, field.Type)
			enqueue(field.Type)
		}
	}
	return out.Bytes()
}

func TestDocumentationContract(t *testing.T) {
	path := filepath.Join("..", "..", "docs", "cli-reference.generated.md")
	actual := documentationContract(newRootCmd())
	if *updateDocumentation {
		if err := os.WriteFile(path, actual, 0644); err != nil {
			t.Fatal(err)
		}
	}
	expected, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, expected) {
		t.Fatal("CLI documentation drift: update prose as needed, then run go test ./cmd/loopexec -run '^TestDocumentationContract$' -args -update-docs")
	}
}

func TestDocumentationContractDetectsChanges(t *testing.T) {
	baseline := documentationContract(newRootCmd())
	for name, mutate := range map[string]func(*cobra.Command){
		"command":     func(c *cobra.Command) { c.AddCommand(&cobra.Command{Use: "undocumented", Short: "New behavior"}) },
		"flag":        func(c *cobra.Command) { c.PersistentFlags().Bool("undocumented", false, "New flag") },
		"default":     func(c *cobra.Command) { c.PersistentFlags().Lookup("json").DefValue = "true" },
		"description": func(c *cobra.Command) { c.Short = "Changed contract" },
	} {
		t.Run(name, func(t *testing.T) {
			root := newRootCmd()
			mutate(root)
			if bytes.Equal(baseline, documentationContract(root)) {
				t.Fatal("contract change escaped documentation comparison")
			}
		})
	}
}
