package cli

import (
	"testing"
)

func TestVolumeCommandsUseSDKEndpoints(t *testing.T) {
	for _, tc := range []struct {
		args []string
		path string
		body map[string]any
	}{
		{[]string{"volume", "list"}, "/blockstorage/list", map[string]any{"projectkey": "CLfixture"}},
		{[]string{"blockstorage", "tiers"}, "/blockstorage/listtiers", nil},
		{[]string{"volume", "get", "v1"}, "/blockstorage/details", map[string]any{"volumeid": "v1"}},
		{[]string{"volume", "cost", "--tier", "balanced", "--size-gib", "10"}, "/blockstorage/estimatedcost", map[string]any{"projectkey": "CLfixture", "tier": "balanced", "sizeingib": float64(10)}},
		{[]string{"volume", "cost", "--volume", "v1"}, "/blockstorage/estimatedcost", map[string]any{"projectkey": "CLfixture", "volumeid": "v1"}},
		{[]string{"volume", "create", "--name", "data", "--datacenter", "dc-fbg1", "--tier", "balanced", "--size-gib", "10"}, "/blockstorage/create", map[string]any{"name": "data", "datacenter": "dc-fbg1", "tier": "balanced", "sizeingib": float64(10)}},
		{[]string{"volume", "rename", "v1", "--name", "data2"}, "/blockstorage/edit", map[string]any{"volumeid": "v1", "name": "data2"}},
		{[]string{"volume", "resize", "v1", "--size-gib", "20"}, "/blockstorage/resize", map[string]any{"volumeid": "v1", "sizeingib": float64(20)}},
		{[]string{"volume", "attach", "v1", "--server", "s1"}, "/blockstorage/attach", map[string]any{"volumeid": "v1", "serverid": "s1"}},
		{[]string{"volume", "detach", "v1", "--yes"}, "/blockstorage/detach", map[string]any{"volumeid": "v1"}},
		{[]string{"volume", "delete", "v1", "--yes"}, "/blockstorage/delete", map[string]any{"volumeid": "v1"}},
	} {
		t.Run(tc.path+" "+tc.args[1], func(t *testing.T) {
			f := newFixture(t, nil)
			f.success(f.run(tc.args...))
			ex := f.exchanges()
			if len(ex) != 1 || ex[0].path != tc.path {
				t.Fatalf("exchanges %+v, want one to %s", ex, tc.path)
			}
			for k, v := range tc.body {
				if ex[0].body[k] != v {
					t.Fatalf("body[%s] = %v, want %v (body %v)", k, ex[0].body[k], v, ex[0].body)
				}
			}
		})
	}
}

func TestVolumeListSortedAndShaped(t *testing.T) {
	f := newFixture(t, nil)
	v := f.success(f.run("volume", "list"))
	rows := v["data"].([]any)
	if len(rows) != 2 || rows[0].(map[string]any)["id"] != "v1" {
		t.Fatalf("not sorted by ID: %v", rows)
	}
	if att := rows[0].(map[string]any)["attachments"]; att == nil {
		t.Fatal("attachments must be [] rather than null")
	}
	v = f.success(f.run("volume", "get", "v1"))
	att := v["data"].(map[string]any)["attachments"].([]any)
	if att[0].(map[string]any)["serverid"] != "s1" {
		t.Fatalf("attachments lost: %v", v["data"])
	}
}

func TestVolumeValidationConfirmationAndDryRun(t *testing.T) {
	f := newFixture(t, nil)
	for _, args := range [][]string{
		{"volume", "get", "../x"},
		{"volume", "create", "--name", " ", "--datacenter", "dc-fbg1", "--tier", "balanced", "--size-gib", "10"},
		{"volume", "create", "--name", "data", "--datacenter", "dc-fbg1", "--tier", "balanced", "--size-gib", "0"},
		{"volume", "create", "--name", "data"},
		{"volume", "rename", "v1", "--name", ""},
		{"volume", "resize", "v1", "--size-gib", "0"},
		{"volume", "attach", "v1", "--server", "a/b"},
		{"volume", "cost"},
		{"volume", "cost", "--tier", "balanced"},
		{"volume", "cost", "--volume", "v1", "--size-gib", "0"},
	} {
		exit, out, err := f.run(args...)
		errorCode(t, exit, out, err, 2, "invalid_input")
	}
	for _, verb := range []string{"detach", "delete"} {
		exit, out, err := f.run("volume", verb, "v1", "--non-interactive")
		errorCode(t, exit, out, err, 8, "confirmation_required")
	}
	for _, args := range [][]string{
		{"volume", "create", "--name", "data", "--datacenter", "dc-fbg1", "--tier", "balanced", "--size-gib", "10", "--dry-run"},
		{"volume", "rename", "v1", "--name", "x", "--dry-run"},
		{"volume", "resize", "v1", "--size-gib", "20", "--dry-run"},
		{"volume", "attach", "v1", "--server", "s1", "--dry-run"},
		{"volume", "detach", "v1", "--dry-run"},
		{"volume", "delete", "v1", "--dry-run"},
	} {
		v := f.success(f.run(args...))
		if v["data"].(map[string]any)["dry_run"] != true {
			t.Fatalf("%v: not a dry-run plan", args)
		}
	}
	if n := len(f.exchanges()); n != 0 {
		t.Fatalf("%d requests sent; validation, confirmation and dry-run must not reach the API", n)
	}
}
