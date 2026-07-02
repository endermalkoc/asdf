package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/endermalkoc/cusp/internal/ids"
)

// CRUD helpers for the planning layer that complement planning.go's upserts and
// planning_read.go's list queries: single-row getters (for show/edit), milestone
// add/update/delete, entity deletes (FK cascade clears junctions), and targeted
// junction unlinks. Capability/Deliverable/View are keyed by their ULID id (no natural
// key); Milestone is keyed by slug.

// ---- Milestone ------------------------------------------------------------

// MilestoneRow is a full plan_milestone row.
type MilestoneRow struct {
	ID          string `json:"id"`
	Slug        string `json:"slug"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Sequence    *int   `json:"sequence,omitempty"`
	Status      string `json:"status"`
}

const milestoneCols = "id, slug, COALESCE(name,''), COALESCE(description,''), sequence, status"

func scanMilestone(s interface{ Scan(...any) error }) (MilestoneRow, error) {
	var m MilestoneRow
	var seq sql.NullInt64
	if err := s.Scan(&m.ID, &m.Slug, &m.Name, &m.Description, &seq, &m.Status); err != nil {
		return MilestoneRow{}, err
	}
	if seq.Valid {
		v := int(seq.Int64)
		m.Sequence = &v
	}
	return m, nil
}

// ListMilestones returns every milestone in sequence order (then slug).
func ListMilestones(ctx context.Context, x Execer) ([]MilestoneRow, error) {
	rows, err := x.QueryContext(ctx, "SELECT "+milestoneCols+" FROM `plan_milestone` ORDER BY COALESCE(sequence, 9999), slug")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MilestoneRow
	for rows.Next() {
		m, err := scanMilestone(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// GetMilestone fetches one milestone by slug. ok is false when none exists.
func GetMilestone(ctx context.Context, x Execer, slug string) (MilestoneRow, bool, error) {
	m, err := scanMilestone(x.QueryRowContext(ctx, "SELECT "+milestoneCols+" FROM `plan_milestone` WHERE slug=?", slug))
	if err == sql.ErrNoRows {
		return MilestoneRow{}, false, nil
	}
	if err != nil {
		return MilestoneRow{}, false, err
	}
	return m, true, nil
}

// MilestoneIDBySlug resolves a milestone slug to its id. ok is false when none exists.
func MilestoneIDBySlug(ctx context.Context, x Execer, slug string) (string, bool, error) {
	var id string
	err := x.QueryRowContext(ctx, "SELECT id FROM `plan_milestone` WHERE slug=?", slug).Scan(&id)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return id, true, nil
}

// AddMilestone mints a ULID and inserts a milestone (status defaults to pending).
func AddMilestone(ctx context.Context, x Execer, m MilestoneRow) (MilestoneRow, error) {
	if m.Status == "" {
		m.Status = "pending"
	}
	m.ID = ids.New()
	now := time.Now().UTC()
	if _, err := x.ExecContext(ctx,
		"INSERT INTO `plan_milestone` (id,slug,name,description,sequence,status,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?)",
		m.ID, m.Slug, nullIfEmpty(m.Name), nullIfEmpty(m.Description), nullIfNil(m.Sequence), m.Status, now, now); err != nil {
		return MilestoneRow{}, fmt.Errorf("add milestone %q: %w", m.Slug, err)
	}
	return m, nil
}

// UpdateMilestone writes a milestone's editable fields (name/description/sequence/status) by slug.
func UpdateMilestone(ctx context.Context, x Execer, m MilestoneRow) error {
	if _, err := x.ExecContext(ctx,
		"UPDATE `plan_milestone` SET name=?, description=?, sequence=?, status=?, updated_at=? WHERE slug=?",
		nullIfEmpty(m.Name), nullIfEmpty(m.Description), nullIfNil(m.Sequence), m.Status, time.Now().UTC(), m.Slug); err != nil {
		return fmt.Errorf("update milestone %q: %w", m.Slug, err)
	}
	return nil
}

// DeleteMilestone removes a milestone by slug (deliverables/runs referencing it FK-null out).
func DeleteMilestone(ctx context.Context, x Execer, slug string) (MilestoneRow, bool, error) {
	m, ok, err := GetMilestone(ctx, x, slug)
	if err != nil || !ok {
		return MilestoneRow{}, ok, err
	}
	if _, err := x.ExecContext(ctx, "DELETE FROM `plan_milestone` WHERE id=?", m.ID); err != nil {
		return MilestoneRow{}, false, fmt.Errorf("delete milestone %q: %w", slug, err)
	}
	return m, true, nil
}

// ---- Capability / Deliverable / View single-row getters (for show/edit) ---

// GetCapability fetches one capability (joined to its domain slug) by id.
func GetCapability(ctx context.Context, x Execer, id string) (CapabilityRow, bool, error) {
	var c CapabilityRow
	err := x.QueryRowContext(ctx,
		"SELECT c.id, COALESCE(c.title,''), COALESCE(c.level,''), COALESCE(d.slug,''), COALESCE(c.parent_id,'') "+
			"FROM `plan_capability` c LEFT JOIN `req_domain` d ON c.domain_id=d.id WHERE c.id=?", id).
		Scan(&c.ID, &c.Title, &c.Level, &c.DomainSlug, &c.ParentID)
	if err == sql.ErrNoRows {
		return CapabilityRow{}, false, nil
	}
	if err != nil {
		return CapabilityRow{}, false, err
	}
	return c, true, nil
}

// GetDeliverable fetches one deliverable (joined to its milestone slug) by id.
func GetDeliverable(ctx context.Context, x Execer, id string) (DeliverableRow, bool, error) {
	var d DeliverableRow
	err := x.QueryRowContext(ctx,
		"SELECT dl.id, COALESCE(dl.title,''), COALESCE(dl.size,''), COALESCE(dl.status,''), COALESCE(dl.ai_ready,''), COALESCE(m.slug,'') "+
			"FROM `plan_deliverable` dl LEFT JOIN `plan_milestone` m ON dl.milestone_id=m.id WHERE dl.id=?", id).
		Scan(&d.ID, &d.Title, &d.Size, &d.Status, &d.AIReady, &d.MilestoneSlug)
	if err == sql.ErrNoRows {
		return DeliverableRow{}, false, nil
	}
	if err != nil {
		return DeliverableRow{}, false, err
	}
	return d, true, nil
}

// GetPlanView fetches one view (joined to its domain + backing-spec slugs) by id.
func GetPlanView(ctx context.Context, x Execer, id string) (PlanViewRow, bool, error) {
	var v PlanViewRow
	err := x.QueryRowContext(ctx,
		"SELECT v.id, COALESCE(v.title,''), COALESCE(v.route,''), COALESCE(vd.slug,''), "+
			"COALESCE(sd.slug,''), COALESCE(s.path,''), COALESCE(s.slug,''), COALESCE(s.title,'') "+
			"FROM `plan_view` v LEFT JOIN `req_domain` vd ON v.domain_id=vd.id "+
			"LEFT JOIN `req_spec` s ON v.spec_id=s.id LEFT JOIN `req_domain` sd ON s.domain_id=sd.id WHERE v.id=?", id).
		Scan(&v.ID, &v.Title, &v.Route, &v.DomainSlug, &v.SpecDomain, &v.SpecPath, &v.SpecSlug, &v.SpecTitle)
	if err == sql.ErrNoRows {
		return PlanViewRow{}, false, nil
	}
	if err != nil {
		return PlanViewRow{}, false, err
	}
	return v, true, nil
}

// ---- deletes (FK cascade clears junctions; polymorphic refs cleaned explicitly) ----

// DeleteCapability removes a capability by id. Its junction rows cascade; child capabilities'
// parent_id FK-nulls; polymorphic refs (external_ref/edge/entity_ref) are cleaned.
func DeleteCapability(ctx context.Context, x Execer, id string) (bool, error) {
	return deletePlanningRow(ctx, x, "plan_capability", "capability", id)
}

// DeleteDeliverable removes a deliverable by id (junctions cascade; refs cleaned).
func DeleteDeliverable(ctx context.Context, x Execer, id string) (bool, error) {
	return deletePlanningRow(ctx, x, "plan_deliverable", "deliverable", id)
}

// DeletePlanView removes a view by id (junctions cascade; refs cleaned).
func DeletePlanView(ctx context.Context, x Execer, id string) (bool, error) {
	return deletePlanningRow(ctx, x, "plan_view", "view", id)
}

func deletePlanningRow(ctx context.Context, x Execer, table, nodeType, id string) (bool, error) {
	if err := DeleteNodeRefs(ctx, x, nodeType, id); err != nil {
		return false, err
	}
	res, err := x.ExecContext(ctx, "DELETE FROM `"+table+"` WHERE id=?", id)
	if err != nil {
		return false, fmt.Errorf("delete %s %s: %w", nodeType, id, err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ---- targeted junction unlinks -------------------------------------------

func unlinkJunction(ctx context.Context, x Execer, table, colA, colB, a, b string) error {
	if _, err := x.ExecContext(ctx,
		"DELETE FROM `"+table+"` WHERE `"+colA+"`=? AND `"+colB+"`=?", a, b); err != nil {
		return fmt.Errorf("unlink %s (%s,%s): %w", table, a, b, err)
	}
	return nil
}

// UnlinkCapabilityMilestone removes one capability↔milestone link.
func UnlinkCapabilityMilestone(ctx context.Context, x Execer, capID, milestoneID string) error {
	return unlinkJunction(ctx, x, "plan_capability_milestone", "capability_id", "milestone_id", capID, milestoneID)
}

// UnlinkCapabilityDeliverable removes one capability↔deliverable link.
func UnlinkCapabilityDeliverable(ctx context.Context, x Execer, capID, deliverableID string) error {
	return unlinkJunction(ctx, x, "plan_capability_deliverable", "capability_id", "deliverable_id", capID, deliverableID)
}

// UnlinkDeliverableView removes one deliverable↔view link.
func UnlinkDeliverableView(ctx context.Context, x Execer, deliverableID, viewID string) error {
	return unlinkJunction(ctx, x, "plan_deliverable_view", "deliverable_id", "view_id", deliverableID, viewID)
}

// UnlinkDeliverableDependency removes one deliverable↔blocked-by link.
func UnlinkDeliverableDependency(ctx context.Context, x Execer, deliverableID, blockedByID string) error {
	return unlinkJunction(ctx, x, "plan_deliverable_dependency", "deliverable_id", "blocked_by_id", deliverableID, blockedByID)
}
