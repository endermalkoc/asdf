package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/endermalkoc/cusp/internal/app"
	"github.com/endermalkoc/cusp/internal/store"
)

var (
	refSubject     string // TYPE:key (or bare fr_key) — resolved on the write-target branch
	refSubjectType string // explicit escape hatch for subjects with no reference token
	refSubjectID   string
	refSystem      string
	refExternalID  string
	refURL         string
)

var refCmd = &cobra.Command{
	Use:   "ref",
	Short: "Manage external references (a Cusp subject ↔ its id in an outside system)",
	Long: "Record and inspect external references — a link from a deliverable, requirement, or\n" +
		"test result to its id/key in an outside system (jira, github, beads, linear, …).\n" +
		"The subject is a TYPE:key reference (a bare value is a requirement fr_key), or given\n" +
		"explicitly with --subject-type/--subject-id for subjects that have no reference token.\n" +
		"An external ref is idempotent by (subject, system): re-adding updates it in place.",
}

// resolveRefSubject turns the --subject / --subject-type+--subject-id flags into a stored
// (subject_type, subject_id) pair plus a display ref, resolving a --subject against the
// write-target branch. Returns an error when no subject is given (add requires one).
func resolveRefSubject(ctx context.Context, r store.Execer) (subjectType, subjectID, subjectRef string, err error) {
	switch {
	case refSubject != "":
		resolver, e := app.LoadResolver(ctx, r)
		if e != nil {
			return "", "", "", e
		}
		t, ok := app.ResolveRef(resolver, refSubject)
		if !ok {
			return "", "", "", app.NotFoundErr(fmt.Errorf("unknown subject %q — use TYPE:key (e.g. REQ:ATT-FR-012) or --subject-type/--subject-id", refSubject))
		}
		return t.Type, t.ID, t.Type + ":" + t.Key, nil
	case refSubjectType != "" || refSubjectID != "":
		if refSubjectType == "" || refSubjectID == "" {
			return "", "", "", app.ValidationFailed(fmt.Errorf("--subject-type and --subject-id must be given together"))
		}
		return refSubjectType, refSubjectID, refSubjectType + ":" + refSubjectID, nil
	default:
		return "", "", "", app.ValidationFailed(fmt.Errorf("a subject is required: --subject TYPE:key or --subject-type/--subject-id"))
	}
}

var refAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add or update an external reference (idempotent by subject+system)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		if strings.TrimSpace(refSystem) == "" || strings.TrimSpace(refExternalID) == "" {
			return app.ValidationFailed(fmt.Errorf("--system and --external-id are required"))
		}
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		var subjectType, subjectID, subjectRef, id string
		var inserted bool
		err = runMutate(cmd, ws, app.MutateOpts{
			Summary: fmt.Sprintf("ref %s:%s", refSystem, refExternalID),
			Validate: func(vctx context.Context, r store.Execer) error {
				st, sid, sref, e := resolveRefSubject(vctx, r)
				if e != nil {
					return e
				}
				subjectType, subjectID, subjectRef = st, sid, sref
				return nil
			},
		}, func(ctx context.Context, w *app.Write) error {
			newID, ins, e := store.UpsertExternalRef(ctx, w.Tx, subjectType, subjectID, refSystem, refExternalID, refURL)
			if e != nil {
				return e
			}
			id, inserted = newID, ins
			w.MarkDirty("pub_external_ref")
			return nil
		})
		if err != nil {
			return err
		}
		verb := "updated"
		if inserted {
			verb = "added"
		}
		emit(store.ExternalRefRow{
			ID: id, SubjectType: subjectType, SubjectID: subjectID, SubjectRef: subjectRef,
			System: refSystem, ExternalID: refExternalID, URL: refURL,
		}, fmt.Sprintf("%s ref: %s → %s:%s  (id=%s)", verb, subjectRef, refSystem, refExternalID, id))
		return nil
	},
}

var refLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List external references (optionally for one subject)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		rd, done, err := connectRead(ctx)
		if err != nil {
			return err
		}
		defer done()
		var subjectType, subjectID string
		if refSubject != "" || refSubjectType != "" || refSubjectID != "" {
			st, sid, _, e := resolveRefSubject(ctx, rd)
			if e != nil {
				return e
			}
			subjectType, subjectID = st, sid
		}
		refsList, err := store.ListExternalRefs(ctx, rd, subjectType, subjectID)
		if err != nil {
			return err
		}
		if lbl, e := app.LabelIndex(ctx, rd); e == nil {
			for i := range refsList {
				refsList[i].SubjectRef = lbl(refsList[i].SubjectType, refsList[i].SubjectID)
			}
		}
		if flagJSON {
			emit(refsList, "")
			return nil
		}
		if len(refsList) == 0 {
			fmt.Println("(no external refs)")
			return nil
		}
		var b strings.Builder
		for _, r := range refsList {
			subj := r.SubjectRef
			if subj == "" {
				subj = r.SubjectType + ":" + r.SubjectID
			}
			line := fmt.Sprintf("%-28s %s:%s", subj, r.System, r.ExternalID)
			if r.URL != "" {
				line += "  " + r.URL
			}
			fmt.Fprintf(&b, "%s  (id=%s)\n", line, r.ID)
		}
		fmt.Print(b.String())
		return nil
	},
}

var refShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show one external reference",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		rd, done, err := connectRead(ctx)
		if err != nil {
			return err
		}
		defer done()
		r, ok, err := store.GetExternalRef(ctx, rd, args[0])
		if err != nil {
			return err
		}
		if !ok {
			return app.NotFound("external ref", args[0])
		}
		if lbl, e := app.LabelIndex(ctx, rd); e == nil {
			r.SubjectRef = lbl(r.SubjectType, r.SubjectID)
		}
		if flagJSON {
			emit(r, "")
			return nil
		}
		fmt.Print(printStruct(r))
		return nil
	},
}

var refRmCmd = &cobra.Command{
	Use:   "rm <id>",
	Short: "Delete an external reference by id",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return simpleDelete(cmd, "delete external ref "+args[0], "external ref", args[0], store.DeleteExternalRef, "pub_external_ref")
	},
}

func init() {
	refAddCmd.Flags().StringVar(&refSubject, "subject", "", "subject: TYPE:key (or a bare fr_key)")
	refAddCmd.Flags().StringVar(&refSubjectType, "subject-type", "", "explicit subject type (deliverable|requirement|test_result) for tokenless subjects")
	refAddCmd.Flags().StringVar(&refSubjectID, "subject-id", "", "explicit subject id (with --subject-type)")
	refAddCmd.Flags().StringVar(&refSystem, "system", "", "external system, e.g. jira|github|beads|linear (required)")
	refAddCmd.Flags().StringVar(&refExternalID, "external-id", "", "id/key in that system (required)")
	refAddCmd.Flags().StringVar(&refURL, "url", "", "URL to the external item (optional)")

	refLsCmd.Flags().StringVar(&refSubject, "subject", "", "only refs for this TYPE:key")
	refLsCmd.Flags().StringVar(&refSubjectType, "subject-type", "", "only refs for this subject type (with --subject-id)")
	refLsCmd.Flags().StringVar(&refSubjectID, "subject-id", "", "only refs for this subject id (with --subject-type)")

	refCmd.AddCommand(refAddCmd, refLsCmd, refShowCmd, refRmCmd)
	rootCmd.AddCommand(refCmd)
}
