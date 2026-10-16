package db

import (
	"context"
	"testing"

	"bailleul.dev/budget-server/internal/domain/entities"
)

func TestCommentRepository_AddAndListForOperation(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)
	account := createTestAccount(t, pool, user.ID)
	operation := mustCreateOperation(t, pool, account.ID, "Restaurant", "-25.00")
	repo := NewCommentRepository(pool)

	first, err := repo.Add(ctx, &entities.Comment{ID: newTestUUID(), OperationID: operation.ID, UserID: user.ID, Body: "First comment"})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM comments WHERE id = $1", first.ID) })
	second, err := repo.Add(ctx, &entities.Comment{ID: newTestUUID(), OperationID: operation.ID, UserID: user.ID, Body: "Second comment"})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM comments WHERE id = $1", second.ID) })

	list, err := repo.ListForOperation(ctx, operation.ID)
	if err != nil {
		t.Fatalf("ListForOperation: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d comments, want 2", len(list))
	}
	// Oldest first.
	if list[0].Body != "First comment" || list[1].Body != "Second comment" {
		t.Errorf("got %+v, want chronological order", list)
	}
}

func TestCommentRepository_ListForOperation_Empty(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)
	account := createTestAccount(t, pool, user.ID)
	operation := mustCreateOperation(t, pool, account.ID, "No comments", "-5.00")

	list, err := NewCommentRepository(pool).ListForOperation(ctx, operation.ID)
	if err != nil {
		t.Fatalf("ListForOperation: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("got %d comments, want 0", len(list))
	}
}

func TestCommentRepository_ListForAuthor_OnlyThatAuthorAcrossAccounts(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	me := createTestUser(t, pool)
	other := createTestUser(t, pool)
	myAccount := createTestAccount(t, pool, me.ID)
	theirAccount := createTestAccount(t, pool, other.ID)
	myOp := mustCreateOperation(t, pool, myAccount.ID, "Mine", "-1.00")
	theirOp := mustCreateOperation(t, pool, theirAccount.ID, "Theirs", "-2.00")
	repo := NewCommentRepository(pool)

	// I comment on my own operation and on one of another user's (shared).
	for _, c := range []*entities.Comment{
		{ID: newTestUUID(), OperationID: myOp.ID, UserID: me.ID, Body: "on mine"},
		{ID: newTestUUID(), OperationID: theirOp.ID, UserID: me.ID, Body: "on theirs"},
		{ID: newTestUUID(), OperationID: theirOp.ID, UserID: other.ID, Body: "their own words"},
	} {
		created, err := repo.Add(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { mustExec(t, pool, "DELETE FROM comments WHERE id = $1", created.ID) })
	}

	got, err := repo.ListForAuthor(ctx, me.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d comments, want the 2 I wrote", len(got))
	}
	for _, c := range got {
		if c.UserID != me.ID {
			t.Errorf("got a comment written by %s", c.UserID)
		}
	}
}
