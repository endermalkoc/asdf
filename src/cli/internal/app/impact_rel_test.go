package app_test

import (
	"context"
	"testing"

	"github.com/endermalkoc/cusp/internal/app"
	"github.com/endermalkoc/cusp/internal/refs"
	"github.com/endermalkoc/cusp/internal/store"
	"github.com/endermalkoc/cusp/internal/testutil"
)

// Impact over ent_relationship: an entity subject surfaces its entity↔entity relationships as
// inbound/outbound links (via "relationship (<cardinality>)"). Shares mutate/hasLink from dolt_test.go.

func TestImpact_EntityRelationship(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	var studentID, courseID string
	mutate(t, ws, "main", func(ctx context.Context, w *app.Write) error {
		var e error
		if studentID, _, e = store.UpsertEntity(ctx, w.Tx, store.Entity{Name: "Student", Status: "active"}); e != nil {
			return e
		}
		if courseID, _, e = store.UpsertEntity(ctx, w.Tx, store.Entity{Name: "Course", Status: "active"}); e != nil {
			return e
		}
		_, e = store.UpsertEntityRelationship(ctx, w.Tx, studentID, courseID, "1:N", "enrollment")
		w.MarkDirty("ent_entity")
		w.MarkDirty("ent_relationship")
		return e
	})

	ctx := context.Background()
	r, release, err := app.Reader(ctx, ws, "main")
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// Student → Course is outbound from Student, inbound to Course.
	repS, err := app.Impact(ctx, r, refs.Target{Type: "entity", Key: "Student", ID: studentID}, false)
	if err != nil {
		t.Fatalf("impact Student: %v", err)
	}
	if !hasLink(repS.Outbound, "entity:Course", "relationship (1:N)") {
		t.Errorf("expected outbound entity:Course via relationship (1:N), got %+v", repS.Outbound)
	}
	repC, err := app.Impact(ctx, r, refs.Target{Type: "entity", Key: "Course", ID: courseID}, false)
	if err != nil {
		t.Fatalf("impact Course: %v", err)
	}
	if !hasLink(repC.Inbound, "entity:Student", "relationship (1:N)") {
		t.Errorf("expected inbound entity:Student via relationship (1:N), got %+v", repC.Inbound)
	}
}
