package db

import (
	"context"
	"testing"

	"bailleul.dev/budget-server/internal/domain/entities"
)

func TestCategoryRepository_AddAndGetByID(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	category := createTestCategory(t, pool)

	got, err := NewCategoryRepository(pool).GetByID(ctx, category.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil || got.Name != category.Name {
		t.Fatalf("got %v, want %v", got, category)
	}
	if got.UserID != nil {
		t.Errorf("UserID = %v, want nil (a standard category)", got.UserID)
	}
}

func TestCategoryRepository_ListVisibleTo(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := NewCategoryRepository(pool)
	user := createTestUser(t, pool)
	otherUser := createTestUser(t, pool)

	standard := createTestCategory(t, pool)
	mine, err := repo.Add(ctx, &entities.Category{ID: newTestUUID(), UserID: &user.ID, Name: "My Category " + newTestUUIDString()})
	if err != nil {
		t.Fatalf("Add (mine): %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM categories WHERE id = $1", mine.ID) })
	theirs, err := repo.Add(ctx, &entities.Category{ID: newTestUUID(), UserID: &otherUser.ID, Name: "Their Category " + newTestUUIDString()})
	if err != nil {
		t.Fatalf("Add (theirs): %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM categories WHERE id = $1", theirs.ID) })

	visible, err := repo.ListVisibleTo(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListVisibleTo: %v", err)
	}
	ids := map[string]bool{}
	for _, c := range visible {
		ids[c.ID.String()] = true
	}
	if !ids[standard.ID.String()] {
		t.Error("a standard category should be visible to any user")
	}
	if !ids[mine.ID.String()] {
		t.Error("a user's own category should be visible to them")
	}
	if ids[theirs.ID.String()] {
		t.Error("another user's private category should not be visible")
	}
}

func TestCategoryRepository_Update(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	category := createTestCategory(t, pool)

	category.Name = "Renamed " + category.Name
	updated, err := NewCategoryRepository(pool).Update(ctx, category)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != category.Name {
		t.Fatalf("Name = %q, want %q", updated.Name, category.Name)
	}
}

func TestCategoryRepository_Delete(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	category, err := NewCategoryRepository(pool).Add(ctx, &entities.Category{ID: newTestUUID(), Name: "To Delete " + newTestUUIDString()})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	if err := NewCategoryRepository(pool).Delete(ctx, category.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, err := NewCategoryRepository(pool).GetByID(ctx, category.ID)
	if err != nil {
		t.Fatalf("GetByID after delete: %v", err)
	}
	if got != nil {
		t.Fatalf("got %v, want nil after Delete", got)
	}
}

// A parent/child pair - covers the self-referencing parent_id FK
// (categories.parent_id -> categories).
func TestCategoryRepository_ParentChild(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := NewCategoryRepository(pool)
	parent := createTestCategory(t, pool)

	child, err := repo.Add(ctx, &entities.Category{ID: newTestUUID(), ParentID: &parent.ID, Name: "Child " + newTestUUIDString()})
	if err != nil {
		t.Fatalf("Add (child): %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM categories WHERE id = $1", child.ID) })

	got, err := repo.GetByID(ctx, child.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ParentID == nil || *got.ParentID != parent.ID {
		t.Fatalf("ParentID = %v, want %s", got.ParentID, parent.ID)
	}
}
