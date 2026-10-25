package gitprovider_test

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	gp "github.com/Paca-AI/plugin-sdk-go/gitprovider"
)

var update = flag.Bool("update", false, "rewrite testdata/wire-shapes.golden.json")

// shapeOf describes a type's JSON shape: field name -> JSON type, recursively.
// It is deliberately independent of Go field names, ordering and type names, so
// two modules with their own copies of the types can agree on it.
func shapeOf(t reflect.Type) any { return shapeOfSeen(t, map[reflect.Type]bool{}) }

func shapeOfSeen(t reflect.Type, seen map[reflect.Type]bool) any {
	switch t.Kind() {
	case reflect.Pointer:
		return shapeOfSeen(t.Elem(), seen)
	case reflect.Struct:
		if t.PkgPath() == "time" {
			return "time"
		}
		if seen[t] {
			return "recursive:" + t.Name() // Identity <-> SetupStep refer to each other
		}
		seen[t] = true
		defer delete(seen, t)
		out := map[string]any{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			if name == "-" || !f.IsExported() {
				continue
			}
			if name == "" {
				name = f.Name
			}
			out[name] = shapeOfSeen(f.Type, seen)
		}
		return out
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			return "bytes"
		}
		return []any{shapeOfSeen(t.Elem(), seen)}
	case reflect.Map:
		return map[string]any{"*": shapeOfSeen(t.Elem(), seen)}
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "bool"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64:
		return "number"
	case reflect.Interface:
		return "any"
	}
	return t.Kind().String()
}

func TestWireShapesMatchGolden(t *testing.T) {
	got := map[string]any{"spi_version": gp.SPIVersion}
	shapes := map[string]any{}
	for name, typ := range gp.WireTypes() {
		shapes[name] = shapeOf(typ)
	}
	got["types"] = shapes
	raw, _ := json.MarshalIndent(got, "", "  ")
	raw = append(raw, '\n')
	path := filepath.Join("testdata", "wire-shapes.golden.json")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden file (run `go test ./gitprovider -run WireShapes -update`): %v", err)
	}
	if string(want) != string(raw) {
		t.Fatalf("the wire contract changed. If that is intended, bump SPIVersion when it is breaking, regenerate with -update, and copy testdata/wire-shapes.golden.json into services/api/internal/platform/gitprovider/testdata/ so core's copy of the types is updated to match.")
	}
}
