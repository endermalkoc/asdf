package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/endermalkoc/cusp/internal/app"
	"github.com/endermalkoc/cusp/internal/store"
)

// The section CLI is where the curated-vocabulary friction lives: `section add` resolves
// the requested type and FAILS if it does not exist, pointing at `section-type add`.
// Creating a type is a deliberate, separate command — never an inline flag on `section
// add`. The spec and entity namespaces share one command tree (sectionNS).

var (
	flagSectionType     string
	flagSectionBody     string
	flagTypeTitle       string
	flagTypeLevel       int
	flagTypePosition    int
	flagTypeDescription string
)

// sectionNS bundles the store entry points for one section namespace (spec | entity).
type sectionNS struct {
	noun          string // "spec" | "entity"
	ownerArg      string // help token for the owner argument
	resolveOwner  func(context.Context, store.Execer, string) (string, bool, error)
	typeByKey     func(context.Context, store.Execer, string) (store.SectionTypeRow, bool, error)
	listTypes     func(context.Context, store.Execer) ([]store.SectionTypeRow, error)
	upsertType    func(context.Context, store.Execer, store.SectionTypeRow) (bool, error)
	upsertSection func(context.Context, store.Execer, string, string, string) (string, bool, error)
	deleteSection func(context.Context, store.Execer, string, string) (bool, error)
	listSections  func(context.Context, store.Execer, string) ([]store.SectionRow, error)
	sectionTable  string
	typeTable     string
}

func readBody(s string) (string, error) {
	if s == "-" {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	return s, nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 70 {
		s = s[:67] + "..."
	}
	return s
}

// sectionCmd builds the `<noun> section add|ls` tree.
func (ns sectionNS) sectionCmd() *cobra.Command {
	parent := &cobra.Command{Use: "section", Short: "Manage a " + ns.noun + "'s prose sections"}

	addCmd := &cobra.Command{
		Use:   "add " + ns.ownerArg + " --type <key>",
		Short: "Set a " + ns.noun + " section (its type must already exist — see section-type add)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			ws, err := connect(ctx)
			if err != nil {
				return err
			}
			defer ws.Close()
			if flagSectionType == "" {
				return fmt.Errorf("--type is required")
			}
			body, err := readBody(flagSectionBody)
			if err != nil {
				return err
			}
			var ownerID string
			err = runMutate(cmd, ws, app.MutateOpts{
				Summary: fmt.Sprintf("set %s section %s/%s", ns.noun, args[0], flagSectionType),
				Validate: func(vctx context.Context, r store.Execer) error {
					id, ok, e := ns.resolveOwner(vctx, r, args[0])
					if e != nil {
						return e
					}
					if !ok {
						return app.NotFoundErr(fmt.Errorf("unknown %s %q", ns.noun, args[0]))
					}
					ownerID = id
					// Friction: the section type must already exist; there is no inline
					// create flag. An unknown type points the author at section-type add.
					if _, ok, e := ns.typeByKey(vctx, r, flagSectionType); e != nil {
						return e
					} else if !ok {
						return fmt.Errorf("unknown section type %q — create it first:\n  cusp %s section-type add %s --title <t> --position <n>",
							flagSectionType, ns.noun, flagSectionType)
					}
					return nil
				},
			}, func(ctx context.Context, w *app.Write) error {
				if _, _, e := ns.upsertSection(ctx, w.Tx, ownerID, flagSectionType, body); e != nil {
					return e
				}
				w.MarkDirty(ns.sectionTable)
				return nil
			})
			if err != nil {
				return err
			}
			emit(map[string]string{ns.noun: args[0], "type": flagSectionType},
				fmt.Sprintf("set %s section %s [%s]", ns.noun, args[0], flagSectionType))
			return nil
		},
	}
	addCmd.Flags().StringVar(&flagSectionType, "type", "", "section type key (must exist; see `section-type ls`)")
	addCmd.Flags().StringVar(&flagSectionBody, "body", "", "section body Markdown (use - to read stdin)")

	lsCmd := &cobra.Command{
		Use:   "ls " + ns.ownerArg,
		Short: "List a " + ns.noun + "'s sections in render order",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			r, done, err := connectRead(ctx)
			if err != nil {
				return err
			}
			defer done()
			id, ok, err := ns.resolveOwner(ctx, r, args[0])
			if err != nil {
				return err
			}
			if !ok {
				return app.NotFoundErr(fmt.Errorf("unknown %s %q", ns.noun, args[0]))
			}
			secs, err := ns.listSections(ctx, r, id)
			if err != nil {
				return err
			}
			if flagJSON {
				emit(secs, "")
				return nil
			}
			var b strings.Builder
			for _, s := range secs {
				fmt.Fprintf(&b, "%-18s %s\n", s.Key, firstLine(s.Body))
			}
			fmt.Print(b.String())
			return nil
		},
	}

	delCmd := &cobra.Command{
		Use:   "delete " + ns.ownerArg + " --type <key>",
		Short: "Remove a " + ns.noun + " section",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			ws, err := connect(ctx)
			if err != nil {
				return err
			}
			defer ws.Close()
			if flagSectionType == "" {
				return fmt.Errorf("--type is required")
			}
			var ownerID string
			err = runMutate(cmd, ws, app.MutateOpts{
				Summary: fmt.Sprintf("delete %s section %s/%s", ns.noun, args[0], flagSectionType),
				Validate: func(vctx context.Context, r store.Execer) error {
					id, ok, e := ns.resolveOwner(vctx, r, args[0])
					if e != nil {
						return e
					}
					if !ok {
						return app.NotFoundErr(fmt.Errorf("unknown %s %q", ns.noun, args[0]))
					}
					ownerID = id
					return nil
				},
			}, func(ctx context.Context, w *app.Write) error {
				ok, e := ns.deleteSection(ctx, w.Tx, ownerID, flagSectionType)
				if e != nil {
					return e
				}
				if !ok {
					return app.NotFoundErr(fmt.Errorf("no %q section on %s %q", flagSectionType, ns.noun, args[0]))
				}
				w.MarkDirty(ns.sectionTable)
				return nil
			})
			if err != nil {
				return err
			}
			emit(map[string]string{ns.noun: args[0], "type": flagSectionType},
				fmt.Sprintf("deleted %s section %s [%s]", ns.noun, args[0], flagSectionType))
			return nil
		},
	}
	delCmd.Flags().StringVar(&flagSectionType, "type", "", "section type key to remove")

	parent.AddCommand(addCmd, lsCmd, delCmd)
	return parent
}

// sectionTypeCmd builds the `<noun> section-type add|ls` tree.
func (ns sectionNS) sectionTypeCmd() *cobra.Command {
	parent := &cobra.Command{Use: "section-type", Short: "Manage the " + ns.noun + " section-type vocabulary"}

	addCmd := &cobra.Command{
		Use:   "add <key>",
		Short: "Add a " + ns.noun + " section type (the deliberate cost of a new section kind)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			ws, err := connect(ctx)
			if err != nil {
				return err
			}
			defer ws.Close()
			t := store.SectionTypeRow{
				Key: args[0], Title: flagTypeTitle, Level: flagTypeLevel,
				Position: flagTypePosition, Description: flagTypeDescription, Origin: "authored",
			}
			err = runMutate(cmd, ws, app.MutateOpts{
				Summary: fmt.Sprintf("add %s section type %s", ns.noun, args[0]),
			}, func(ctx context.Context, w *app.Write) error {
				if _, e := ns.upsertType(ctx, w.Tx, t); e != nil {
					return e
				}
				w.MarkDirty(ns.typeTable)
				return nil
			})
			if err != nil {
				return err
			}
			emit(t, fmt.Sprintf("added %s section type %q (position %d, level %d)", ns.noun, t.Key, t.Position, t.Level))
			return nil
		},
	}
	addCmd.Flags().StringVar(&flagTypeTitle, "title", "", "section title, rendered as the ## heading (empty = headingless)")
	addCmd.Flags().IntVar(&flagTypeLevel, "level", 2, "heading depth (2=##, 3=###, 0=headingless)")
	addCmd.Flags().IntVar(&flagTypePosition, "position", 0, "canonical render order")
	addCmd.Flags().StringVar(&flagTypeDescription, "description", "", "guidance shown when picking the type")

	lsCmd := &cobra.Command{
		Use:   "ls",
		Short: "List the " + ns.noun + " section-type vocabulary in render order",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			r, done, err := connectRead(ctx)
			if err != nil {
				return err
			}
			defer done()
			types, err := ns.listTypes(ctx, r)
			if err != nil {
				return err
			}
			if flagJSON {
				emit(types, "")
				return nil
			}
			var b strings.Builder
			for _, t := range types {
				h := t.Title
				if h == "" {
					h = "(headingless)"
				}
				fmt.Fprintf(&b, "%-18s pos=%-4d lvl=%d  %-9s %s\n", t.Key, t.Position, t.Level, t.Origin, h)
			}
			fmt.Print(b.String())
			return nil
		},
	}

	parent.AddCommand(addCmd, lsCmd)
	return parent
}

func init() {
	specNS := sectionNS{
		noun: "spec", ownerArg: "<spec-prefix>",
		resolveOwner:  store.SpecIDByPrefix,
		typeByKey:     store.SpecSectionTypeByKey,
		listTypes:     store.ListSpecSectionTypes,
		upsertType:    store.UpsertSpecSectionType,
		upsertSection: store.UpsertSpecSection,
		deleteSection: store.DeleteSpecSection,
		listSections:  store.ListSpecSections,
		sectionTable:  "req_spec_section", typeTable: "req_spec_section_type",
	}
	entityNS := sectionNS{
		noun: "entity", ownerArg: "<entity-name>",
		resolveOwner:  store.EntityIDByName,
		typeByKey:     store.EntitySectionTypeByKey,
		listTypes:     store.ListEntitySectionTypes,
		upsertType:    store.UpsertEntitySectionType,
		upsertSection: store.UpsertEntitySection,
		deleteSection: store.DeleteEntitySection,
		listSections:  store.ListEntitySections,
		sectionTable:  "ent_entity_section", typeTable: "ent_entity_section_type",
	}
	specCmd.AddCommand(specNS.sectionCmd(), specNS.sectionTypeCmd())
	entityCmd.AddCommand(entityNS.sectionCmd(), entityNS.sectionTypeCmd())
}
