package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBuiltInToolsLifecycle(t *testing.T) {
	workDir := t.TempDir()
	registry := NewRegistry()
	ctx := context.Background()
	toolCtx := Context{WorkDir: workDir}

	create, ok := registry.Find("create")
	if !ok {
		t.Fatal("create tool is not registered")
	}
	if _, err := create.Execute(ctx, toolCtx, json.RawMessage(`{"path":"notes.txt","content":"one\ntwo\nthree"}`)); err != nil {
		t.Fatalf("create: %v", err)
	}

	read, ok := registry.Find("read")
	if !ok {
		t.Fatal("read tool is not registered")
	}
	readResult, err := read.Execute(ctx, toolCtx, json.RawMessage(`{"path":"notes.txt","offset":2,"limit":1}`))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got := readResult.Content[0].Text; got != "two" {
		t.Fatalf("read content = %q, want %q", got, "two")
	}

	write, ok := registry.Find("write")
	if !ok {
		t.Fatal("write tool is not registered")
	}
	if _, err := write.Execute(ctx, toolCtx, json.RawMessage(`{"path":"nested/output.txt","content":"updated"}`)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(workDir, "nested", "output.txt")); err != nil || string(data) != "updated" {
		t.Fatalf("written file = %q, err = %v", string(data), err)
	}

	save, ok := registry.Find("save")
	if !ok {
		t.Fatal("save tool is not registered")
	}
	if _, err := save.Execute(ctx, toolCtx, json.RawMessage(`{"path":"session.json","messages":[{"role":"user","content":"hello"}]}`)); err != nil {
		t.Fatalf("save: %v", err)
	}

	delete, ok := registry.Find("delete")
	if !ok {
		t.Fatal("delete tool is not registered")
	}
	if _, err := delete.Execute(ctx, toolCtx, json.RawMessage(`{"path":"notes.txt"}`)); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workDir, "notes.txt")); !os.IsNotExist(err) {
		t.Fatalf("notes.txt still exists, stat error = %v", err)
	}
}

func TestDeleteRefusesWorkingDirectory(t *testing.T) {
	workDir := t.TempDir()
	delete := DeleteDefinition()
	_, err := delete.Execute(context.Background(), Context{WorkDir: workDir}, json.RawMessage(`{"path":"."}`))
	if err == nil {
		t.Fatal("delete should refuse the working directory")
	}
}
