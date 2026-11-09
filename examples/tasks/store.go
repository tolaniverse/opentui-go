package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type task struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Done  bool   `json:"done"`
}

type taskFile struct {
	NextID int    `json:"next_id"`
	Tasks  []task `json:"tasks"`
}

type store struct {
	path string
	data taskFile
}

func defaultDataPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "opentui-go", "tasks.json"), nil
}

func openStore(path string) (*store, error) {
	s := &store{path: path, data: taskFile{NextID: 1, Tasks: []task{}}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &s.data); err != nil {
		return nil, fmt.Errorf("read tasks: %w", err)
	}
	ids := make(map[int]bool, len(s.data.Tasks))
	for _, t := range s.data.Tasks {
		if t.ID < 1 || ids[t.ID] || strings.TrimSpace(t.Title) == "" || t.ID >= s.data.NextID {
			return nil, fmt.Errorf("invalid task data in %s", path)
		}
		ids[t.ID] = true
	}
	if s.data.NextID < 1 {
		return nil, fmt.Errorf("invalid next_id in %s", path)
	}
	if s.data.Tasks == nil {
		s.data.Tasks = []task{}
	}
	return s, nil
}

// change persists a copy before replacing the in-memory state. Failed
// writes therefore leave the UI and the on-disk file unchanged.
func (s *store) change(fn func(*taskFile) error) error {
	next := taskFile{NextID: s.data.NextID, Tasks: append([]task(nil), s.data.Tasks...)}
	if err := fn(&next); err != nil {
		return err
	}
	if err := writeTasks(s.path, next); err != nil {
		return err
	}
	s.data = next
	return nil
}

func (s *store) add(title string) (int, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return 0, errors.New("task title cannot be empty")
	}
	id := s.data.NextID
	err := s.change(func(d *taskFile) error {
		d.Tasks = append(d.Tasks, task{ID: id, Title: title})
		d.NextID++
		return nil
	})
	return id, err
}

func (s *store) edit(id int, title string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return errors.New("task title cannot be empty")
	}
	return s.change(func(d *taskFile) error {
		for i := range d.Tasks {
			if d.Tasks[i].ID == id {
				d.Tasks[i].Title = title
				return nil
			}
		}
		return fmt.Errorf("task %d not found", id)
	})
}

func (s *store) toggle(id int) error {
	return s.change(func(d *taskFile) error {
		for i := range d.Tasks {
			if d.Tasks[i].ID == id {
				d.Tasks[i].Done = !d.Tasks[i].Done
				return nil
			}
		}
		return fmt.Errorf("task %d not found", id)
	})
}

func writeTasks(path string, data taskFile) (err error) {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".tasks-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err := f.Chmod(0600); err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
