package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/endermalkoc/asdf/internal/app"
	"github.com/endermalkoc/asdf/internal/enums"
	"github.com/endermalkoc/asdf/internal/store"
)

var (
	termName    string
	termAliases []string
	termDomain  string
	termStatus  string
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
		err = app.Mutate(ctx, ws, app.MutateOpts{
			Summary:   fmt.Sprintf("add glossary term %s", slug),
			Changeset: flagChangeset,
			Actor:     flagActor,
			Validate: func(vctx context.Context) error {
				if e := app.ValidateEnum("status", termStatus, enums.GlossaryStatus); e != nil {
					return e
				}
				resolver, e := app.LoadResolver(vctx, ws.DB())
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
				id, ok, e := store.DomainIDByAbbrev(ctx, w.Tx, termDomain)
				if e != nil {
					return e
				}
				if !ok {
					return fmt.Errorf("unknown domain %q", termDomain)
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
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		terms, err := store.ListGlossaryTerms(ctx, ws.DB())
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

func init() {
	termAddCmd.Flags().StringVar(&termName, "name", "", "display name (default: the slug)")
	termAddCmd.Flags().StringSliceVar(&termAliases, "alias", nil, "alternate surface form that also resolves (repeatable)")
	termAddCmd.Flags().StringVar(&termDomain, "domain", "", "scope to a domain abbreviation (optional)")
	termAddCmd.Flags().StringVar(&termStatus, "status", "draft", "status (draft|active|deprecated)")
	termCmd.AddCommand(termAddCmd, termLsCmd)
	rootCmd.AddCommand(termCmd)
}
