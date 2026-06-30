package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/endermalkoc/cusp/internal/app"
	"github.com/endermalkoc/cusp/internal/enums"
	"github.com/endermalkoc/cusp/internal/store"
)

var (
	termName       string
	termAliases    []string
	termDomain     string
	termStatus     string
	termDefinition string
)

var termCmd = &cobra.Command{Use: "term", Short: "Manage glossary terms (shared vocabulary)"}

var termAddCmd = &cobra.Command{
	Use:   "add <slug> <definition>",
	Short: "Define a glossary term (the slug is its [[TERM:slug]] link key)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		slug, definition := args[0], args[1]
		name := termName
		if name == "" {
			name = slug
		}
		var t store.GlossaryTerm
		var resolved app.ResolvedRefs
		err = runMutate(cmd, ws, app.MutateOpts{
			Summary: fmt.Sprintf("add glossary term %s", slug),
			Validate: func(vctx context.Context, r store.Execer) error {
				if e := app.ValidateEnum("status", termStatus, enums.GlossaryStatus); e != nil {
					return e
				}
				resolver, e := app.LoadResolver(vctx, r)
				if e != nil {
					return e
				}
				resolved = app.ScanRefs(resolver, "glossary_term", "", definition)
				if !flagForce {
					return app.DanglingError(resolved.Dangling)
				}
				return nil
			},
		}, func(ctx context.Context, w *app.Write) error {
			domainID := ""
			if termDomain != "" {
				id, ok, e := store.DomainIDBySlug(ctx, w.Tx, termDomain)
				if e != nil {
					return e
				}
				if !ok {
					return app.NotFoundErr(fmt.Errorf("unknown domain %q", termDomain))
				}
				domainID = id
			}
			id, _, e := store.UpsertGlossaryTerm(ctx, w.Tx, store.GlossaryTerm{
				Slug: slug, Term: name, Definition: definition, DomainID: domainID, Status: termStatus,
			})
			if e != nil {
				return e
			}
			w.MarkDirty("req_glossary_term")
			if e := store.SetGlossaryAliases(ctx, w.Tx, id, termAliases); e != nil {
				return e
			}
			w.MarkDirty("req_glossary_alias")
			t = store.GlossaryTerm{ID: id, Slug: slug, Term: name, Definition: definition, DomainID: domainID, Status: termStatus, Aliases: termAliases}
			return app.ReconcileRefs(ctx, w, "glossary_term", id, resolved.Targets)
		})
		if err != nil {
			return err
		}
		emit(t, fmt.Sprintf("defined [[TERM:%s]] — %s  (id=%s)", t.Slug, t.Term, t.ID))
		return nil
	},
}

var termLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List glossary terms",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		r, done, err := connectRead(ctx)
		if err != nil {
			return err
		}
		defer done()
		terms, err := store.ListGlossaryTerms(ctx, r)
		if err != nil {
			return err
		}
		if flagJSON {
			emit(terms, "")
			return nil
		}
		var b strings.Builder
		for _, t := range terms {
			aka := ""
			if len(t.Aliases) > 0 {
				aka = "  (aka " + strings.Join(t.Aliases, ", ") + ")"
			}
			fmt.Fprintf(&b, "%-24s %s%s\n", t.Slug, t.Term, aka)
		}
		fmt.Print(b.String())
		return nil
	},
}

var termShowCmd = &cobra.Command{
	Use:   "show <slug>",
	Short: "Show a glossary term's fields",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		rd, done, err := connectRead(ctx)
		if err != nil {
			return err
		}
		defer done()
		t, ok, err := store.GetGlossaryTerm(ctx, rd, args[0])
		if err != nil {
			return err
		}
		if !ok {
			return app.NotFound("glossary term", args[0])
		}
		if flagJSON {
			emit(t, "")
			return nil
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%s  (%s)\n", t.Slug, t.Status)
		fmt.Fprintf(&b, "  term:       %s\n", t.Term)
		fmt.Fprintf(&b, "  definition: %s\n", t.Definition)
		if len(t.Aliases) > 0 {
			fmt.Fprintf(&b, "  aliases:    %s\n", strings.Join(t.Aliases, ", "))
		}
		if t.DomainSlug != "" {
			fmt.Fprintf(&b, "  domain:     %s\n", t.DomainSlug)
		}
		fmt.Fprintf(&b, "  id:         %s\n", t.ID)
		fmt.Print(b.String())
		return nil
	},
}

var termEditCmd = &cobra.Command{
	Use:   "edit <slug>",
	Short: "Edit a glossary term's name/definition/status (only the flags you pass change)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		if cmd.Flags().NFlag() == 0 {
			return fmt.Errorf("nothing to edit — pass --name/--definition/--status")
		}
		var t store.GlossaryTermRow
		var resolved app.ResolvedRefs
		err = runMutate(cmd, ws, app.MutateOpts{
			Summary: "edit glossary term " + args[0],
			Validate: func(vctx context.Context, r store.Execer) error {
				cur, ok, e := store.GetGlossaryTerm(vctx, r, args[0])
				if e != nil {
					return e
				}
				if !ok {
					return app.NotFound("glossary term", args[0])
				}
				if cmd.Flags().Changed("status") {
					if e := app.ValidateEnum("status", termStatus, enums.GlossaryStatus); e != nil {
						return e
					}
					cur.Status = termStatus
				}
				if cmd.Flags().Changed("name") {
					cur.Term = termName
				}
				if cmd.Flags().Changed("definition") {
					resolver, e := app.LoadResolver(vctx, r)
					if e != nil {
						return e
					}
					resolved = app.ScanRefs(resolver, "glossary_term", cur.ID, termDefinition)
					cur.Definition = termDefinition
					if !flagForce {
						if e := app.DanglingError(resolved.Dangling); e != nil {
							return e
						}
					}
				}
				t = cur
				return nil
			},
		}, func(ctx context.Context, w *app.Write) error {
			if e := store.UpdateGlossaryTerm(ctx, w.Tx, t.ID, t.Term, t.Definition, t.Status); e != nil {
				return e
			}
			w.MarkDirty("req_glossary_term")
			if cmd.Flags().Changed("definition") {
				return app.ReconcileRefs(ctx, w, "glossary_term", t.ID, resolved.Targets)
			}
			return nil
		})
		if err != nil {
			return err
		}
		emit(t, fmt.Sprintf("updated term %s", t.Slug))
		return nil
	},
}

var termDeleteCmd = &cobra.Command{
	Use:   "delete <slug>",
	Short: "Delete a glossary term (and its aliases and references)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		var deleted store.GlossaryTermRow
		err = runMutate(cmd, ws, app.MutateOpts{
			Summary: "delete glossary term " + args[0],
		}, func(ctx context.Context, w *app.Write) error {
			t, ok, e := store.DeleteGlossaryTerm(ctx, w.Tx, args[0])
			if e != nil {
				return e
			}
			if !ok {
				return app.NotFound("glossary term", args[0])
			}
			deleted = t
			for _, tbl := range []string{"req_glossary_term", "req_glossary_alias", "req_entity_ref", "req_edge", "pub_external_ref"} {
				w.MarkDirty(tbl)
			}
			return nil
		})
		if err != nil {
			return err
		}
		emit(deleted, fmt.Sprintf("deleted term %s", deleted.Slug))
		return nil
	},
}

func init() {
	termAddCmd.Flags().StringVar(&termName, "name", "", "display name (default: the slug)")
	termAddCmd.Flags().StringSliceVar(&termAliases, "alias", nil, "alternate surface form that also resolves (repeatable)")
	termAddCmd.Flags().StringVar(&termDomain, "domain", "", "scope to a domain slug (optional)")
	termAddCmd.Flags().StringVar(&termStatus, "status", "draft", "status (draft|active|deprecated)")
	termEditCmd.Flags().StringVar(&termName, "name", "", "new display name")
	termEditCmd.Flags().StringVar(&termDefinition, "definition", "", "new definition (re-validates inline links)")
	termEditCmd.Flags().StringVar(&termStatus, "status", "", "status (draft|active|deprecated)")
	termCmd.AddCommand(termAddCmd, termLsCmd, termShowCmd, termEditCmd, termDeleteCmd)
	rootCmd.AddCommand(termCmd)
}
