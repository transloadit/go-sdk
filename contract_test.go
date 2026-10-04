package transloadit

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"
	"strings"
	"testing"
)

// TestOpenAPIContract guards against the Transloadit API drifting away from
// the request/response shapes BearerToken, BearerTokenRequest and
// AssemblyNotificationPayload were modeled on. It fetches the public OpenAPI
// spec published at https://api2.transloadit.com/openapi.json and fails if
// those schemas have changed, so a human notices before users do.
// See https://api2.transloadit.com/openapi.json
func TestOpenAPIContract(t *testing.T) {
	spec := fetchOpenAPISpec(t)

	t.Run("token request schema", func(t *testing.T) {
		schema := resolveSchema(t, spec, requestBodySchema(t, spec, "/token", "application/x-www-form-urlencoded"))
		assertRequired(t, schema, "grant_type")
		assertHasProperties(t, schema, "grant_type", "scope", "aud")
	})

	t.Run("token response schema", func(t *testing.T) {
		schema := resolveSchema(t, spec, responseSchema(t, spec, "/token", "2XX", "application/json"))
		assertRequired(t, schema, "access_token", "expires_in", "scope", "token_type")
	})

	t.Run("assembly notification payload schema", func(t *testing.T) {
		webhook := mustIndex(t, mustIndex(t, spec, "webhooks"), "assemblyNotification")
		post := mustIndex(t, webhook, "post")
		payloadRef := mustIndex(t, mustIndex(t, mustIndex(t, post, "x-transloadit-webhook"), "payload"), "schema")
		schema := resolveSchema(t, spec, payloadRef)

		branches, ok := schema["anyOf"].([]interface{})
		if !ok || len(branches) == 0 {
			t.Fatalf("expected assemblyNotificationPayload to be an anyOf union, got %#v", schema)
		}

		var gotRequired [][]string
		for _, b := range branches {
			branch := resolveSchema(t, spec, b)
			gotRequired = append(gotRequired, stringSlice(branch["required"]))
			// A sample of fields AssemblyInfo/AssemblyNotificationPayload rely
			// on; not exhaustive, but enough to catch a reshaped payload.
			assertHasProperties(t, branch,
				"ok", "error", "assembly_id", "region", "instance",
				"websocket_url", "update_stream_url", "tus_url", "account_id",
				"notify_duration", "warnings", "results", "uploads")
		}

		wantRequired := [][]string{{"ok"}, {"ok"}, {"error"}}
		if !sameRequiredSets(gotRequired, wantRequired) {
			t.Fatalf("assemblyNotificationPayload anyOf required fields changed: got %v, want %v", gotRequired, wantRequired)
		}
	})
}

func fetchOpenAPISpec(t *testing.T) map[string]interface{} {
	t.Helper()

	res, err := http.Get("https://api2.transloadit.com/openapi.json")
	if err != nil {
		t.Fatalf("fetch openapi spec: %s", err)
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read openapi spec: %s", err)
	}

	var spec map[string]interface{}
	if err := json.Unmarshal(body, &spec); err != nil {
		t.Fatalf("decode openapi spec: %s", err)
	}

	return spec
}

func requestBodySchema(t *testing.T, spec map[string]interface{}, path, contentType string) interface{} {
	t.Helper()

	op := mustIndex(t, mustIndex(t, mustIndex(t, spec, "paths"), path), "post")
	body := mustIndex(t, op, "requestBody")
	content := mustIndex(t, mustIndex(t, body, "content"), contentType)
	return mustIndex(t, content, "schema")
}

func responseSchema(t *testing.T, spec map[string]interface{}, path, status, contentType string) interface{} {
	t.Helper()

	op := mustIndex(t, mustIndex(t, mustIndex(t, spec, "paths"), path), "post")
	resp := mustIndex(t, mustIndex(t, op, "responses"), status)
	content := mustIndex(t, mustIndex(t, resp, "content"), contentType)
	return mustIndex(t, content, "schema")
}

// mustIndex looks up key in a map[string]interface{} (or a schema wrapping
// one), failing the test with a readable path hint if it's missing.
func mustIndex(t *testing.T, node interface{}, key string) map[string]interface{} {
	t.Helper()

	m, ok := node.(map[string]interface{})
	if !ok {
		t.Fatalf("expected object while looking up %q, got %T", key, node)
	}

	v, ok := m[key]
	if !ok {
		t.Fatalf("missing expected key %q in openapi spec (contract changed?)", key)
	}

	child, ok := v.(map[string]interface{})
	if !ok {
		t.Fatalf("expected %q to be an object, got %T", key, v)
	}

	return child
}

// resolveSchema follows a chain of "$ref"s pointing into
// #/components/schemas, if present, and returns the resolved schema object.
func resolveSchema(t *testing.T, spec map[string]interface{}, node interface{}) map[string]interface{} {
	t.Helper()

	const prefix = "#/components/schemas/"

	for i := 0; i < 10; i++ {
		schema, ok := node.(map[string]interface{})
		if !ok {
			t.Fatalf("expected a schema object, got %T", node)
		}

		ref, ok := schema["$ref"].(string)
		if !ok {
			return schema
		}

		if !strings.HasPrefix(ref, prefix) {
			t.Fatalf("unsupported $ref %q", ref)
		}

		schemas := mustIndex(t, spec, "components")["schemas"]
		node = mustIndex(t, schemas, strings.TrimPrefix(ref, prefix))
	}

	t.Fatalf("$ref chain too deep, possible cycle")
	return nil
}

func assertRequired(t *testing.T, schema map[string]interface{}, want ...string) {
	t.Helper()

	got := stringSlice(schema["required"])
	sort.Strings(got)
	sortedWant := append([]string{}, want...)
	sort.Strings(sortedWant)

	if fmt.Sprint(got) != fmt.Sprint(sortedWant) {
		t.Fatalf("required fields changed: got %v, want %v", got, sortedWant)
	}
}

func assertHasProperties(t *testing.T, schema map[string]interface{}, names ...string) {
	t.Helper()

	properties, _ := schema["properties"].(map[string]interface{})
	for _, name := range names {
		if _, ok := properties[name]; !ok {
			t.Errorf("expected property %q to exist, it was removed or renamed", name)
		}
	}
}

func stringSlice(v interface{}) []string {
	list, ok := v.([]interface{})
	if !ok {
		return nil
	}

	out := make([]string, 0, len(list))
	for _, item := range list {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}

	return out
}

func sameRequiredSets(got, want [][]string) bool {
	if len(got) != len(want) {
		return false
	}

	for i := range got {
		g := append([]string{}, got[i]...)
		w := append([]string{}, want[i]...)
		sort.Strings(g)
		sort.Strings(w)
		if fmt.Sprint(g) != fmt.Sprint(w) {
			return false
		}
	}

	return true
}
