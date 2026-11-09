package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStorePersistsAddEditToggle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "tasks.json")
	s, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.add("   "); err == nil {
		t.Fatal("empty title accepted")
	}
	id, err := s.add("  write test  ")
	if err != nil || id != 1 {
		t.Fatalf("add = %d, %v", id, err)
	}
	if err := s.edit(id, "revised"); err != nil {
		t.Fatal(err)
	}
	if err := s.toggle(id); err != nil {
		t.Fatal(err)
	}
	other, err := s.add("second")
	if err != nil || other != 2 {
		t.Fatalf("second add = %d, %v", other, err)
	}
	s, err = openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.data.NextID != 3 || len(s.data.Tasks) != 2 || s.data.Tasks[0] != (task{1, "revised", true}) {
		t.Fatalf("reloaded = %+v", s.data)
	}
	if err := s.edit(999, "missing"); err == nil {
		t.Fatal("editing unknown task succeeded")
	}
	if err := s.toggle(999); err == nil {
		t.Fatal("toggling unknown task succeeded")
	}
}

func TestStoreRejectsCorruptDataWithoutOverwriting(t *testing.T) {
	for _, content := range []string{
		"not json", `{"next_id":2,"tasks":[{"id":1,"title":""}]}`,
		`{"next_id":2,"tasks":[{"id":1,"title":"a"},{"id":1,"title":"b"}]}`,
		`{"next_id":1,"tasks":[{"id":1,"title":"a"}]}`,
	} {
		t.Run(content, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "tasks.json")
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := openStore(path); err == nil {
				t.Fatal("accepted invalid data")
			}
			b, err := os.ReadFile(path)
			if err != nil || string(b) != content {
				t.Fatalf("corrupt data overwritten: %s, %v", b, err)
			}
		})
	}
}

func TestStoreFailureDoesNotChangeMemory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	s, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.add("saved"); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(t.TempDir(), "a-file")
	if err := os.WriteFile(parent, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	s.path = filepath.Join(parent, "tasks.json")
	_, err = s.add("unsaved")
	if err == nil || s.data.NextID != 2 || len(s.data.Tasks) != 1 || s.data.Tasks[0].Title != "saved" {
		t.Fatalf("write failure changed memory: %+v, %v", s.data, err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) {
		t.Fatalf("write failure changed existing file: %q, %v", after, err)
	}
}
