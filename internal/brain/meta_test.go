package brain

import (
	"context"
	"testing"
)

func TestBrainMetaSeeded(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t,
		WithDescription("test brain"),
	)

	meta, err := b.Meta(ctx)
	if err != nil {
		t.Fatalf("meta: %v", err)
	}

	if meta.SchemaVersion != SchemaVersion {
		t.Errorf("schema_version: got %q, want %q", meta.SchemaVersion, SchemaVersion)
	}
	if meta.CreatedAt == "" {
		t.Error("created_at should be populated")
	}
	if meta.Description != "test brain" {
		t.Errorf("description: %q", meta.Description)
	}
}

func TestBrainMetaSetAndRecordBackup(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	err := b.SetMeta(ctx, "custom_key", "custom_value")
	if err != nil {
		t.Fatalf("set meta: %v", err)
	}

	meta, _ := b.Meta(ctx)
	if meta.LastBackupAt != "" {
		t.Error("last_backup_at should be empty initially")
	}

	err = b.RecordBackup(ctx)
	if err != nil {
		t.Fatalf("record backup: %v", err)
	}

	meta, _ = b.Meta(ctx)
	if meta.LastBackupAt == "" {
		t.Error("last_backup_at should be set after RecordBackup")
	}
}

func TestBrainMetaCreatedAtImmutable(t *testing.T) {
	ctx := context.Background()
	b := newTestBrain(t)

	meta1, _ := b.Meta(ctx)
	createdAt := meta1.CreatedAt

	err := b.SetMeta(ctx, "description", "updated desc")
	if err != nil {
		t.Fatalf("set: %v", err)
	}

	meta2, _ := b.Meta(ctx)
	if meta2.CreatedAt != createdAt {
		t.Error("created_at should not change")
	}
	if meta2.Description != "updated desc" {
		t.Errorf("description: %q", meta2.Description)
	}
}
