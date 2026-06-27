package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/endermalkoc/asdf/internal/app"
)

var (
	flagRemoteUser string
	flagPushForce  bool
)

var doltCmd = &cobra.Command{
	Use:   "dolt",
	Short: "Sync the database with a Dolt remote (push/pull/fetch, manage remotes)",
	Long: "ASDF's store is a Dolt database, so syncing a version-controlled knowledge graph is\n" +
		"`dolt push`/`pull` over the canonical branch. Remote/branch default to origin/main.\n" +
		"Authenticated remotes: pass --user and set DOLT_REMOTE_PASSWORD in the server's env.",
}

var doltRemoteCmd = &cobra.Command{Use: "remote", Short: "Manage Dolt remotes"}

var doltRemoteAddCmd = &cobra.Command{
	Use:   "add <name> <url>",
	Short: "Configure a remote (e.g. file:///path, https://…, dolthub://org/repo)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		if err := app.RemoteAdd(ctx, ws, args[0], args[1]); err != nil {
			return err
		}
		emit(map[string]string{"name": args[0], "url": args[1]},
			fmt.Sprintf("added remote %s → %s", args[0], args[1]))
		return nil
	},
}

var doltRemoteRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a configured remote",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		if err := app.RemoteRemove(ctx, ws, args[0]); err != nil {
			return err
		}
		emit(map[string]string{"removed": args[0]}, "removed remote "+args[0])
		return nil
	},
}

var doltRemoteLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List configured remotes",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		remotes, err := app.RemoteList(ctx, ws)
		if err != nil {
			return err
		}
		if flagJSON {
			emit(remotes, "")
			return nil
		}
		if len(remotes) == 0 {
			fmt.Println("(no remotes — add one with `asdf dolt remote add <name> <url>`)")
			return nil
		}
		for _, r := range remotes {
			fmt.Printf("%-12s %s\n", r.Name, r.URL)
		}
		return nil
	},
}

var doltPushCmd = &cobra.Command{
	Use:   "push [remote] [branch]",
	Short: "Push a branch to a remote (default origin main)",
	Args:  cobra.MaximumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		remote, branch := remoteBranch(args)
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		if err := app.Push(ctx, ws, remote, branch, flagRemoteUser, flagPushForce); err != nil {
			return err
		}
		emit(map[string]string{"remote": remote, "branch": branch},
			fmt.Sprintf("pushed %s → %s", branch, remote))
		return nil
	},
}

var doltPullCmd = &cobra.Command{
	Use:   "pull [remote] [branch]",
	Short: "Pull a branch from a remote and merge it (default origin main)",
	Args:  cobra.MaximumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		remote, branch := remoteBranch(args)
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		if err := app.Pull(ctx, ws, remote, branch, flagRemoteUser); err != nil {
			return err
		}
		emit(map[string]string{"remote": remote, "branch": branch},
			fmt.Sprintf("pulled %s/%s", remote, branch))
		return nil
	},
}

var doltFetchCmd = &cobra.Command{
	Use:   "fetch [remote]",
	Short: "Fetch a remote's refs without merging (default origin)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		remote, _ := remoteBranch(args)
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		if err := app.Fetch(ctx, ws, remote, flagRemoteUser); err != nil {
			return err
		}
		emit(map[string]string{"remote": remote}, "fetched "+remote)
		return nil
	},
}

var syncCmd = &cobra.Command{
	Use:   "sync [remote] [branch]",
	Short: "Pull then push the canonical branch (share and receive changes; default origin main)",
	Args:  cobra.MaximumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		remote, branch := remoteBranch(args)
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		if err := app.Sync(ctx, ws, remote, branch, flagRemoteUser); err != nil {
			return err
		}
		emit(map[string]string{"remote": remote, "branch": branch},
			fmt.Sprintf("synced %s/%s (pulled + pushed)", remote, branch))
		return nil
	},
}

func init() {
	doltCmd.PersistentFlags().StringVar(&flagRemoteUser, "user", "",
		"remote auth user (password via DOLT_REMOTE_PASSWORD in the server env)")
	doltPushCmd.Flags().BoolVar(&flagPushForce, "force", false, "force-push (overwrite remote history)")
	syncCmd.PersistentFlags().StringVar(&flagRemoteUser, "user", "", "remote auth user")

	doltRemoteCmd.AddCommand(doltRemoteAddCmd, doltRemoteRemoveCmd, doltRemoteLsCmd)
	doltCmd.AddCommand(doltRemoteCmd, doltPushCmd, doltPullCmd, doltFetchCmd)
	rootCmd.AddCommand(doltCmd, syncCmd)
}

// remoteBranch resolves the optional [remote] [branch] positionals, defaulting to origin/main.
func remoteBranch(args []string) (remote, branch string) {
	remote, branch = "origin", "main"
	if len(args) >= 1 && args[0] != "" {
		remote = args[0]
	}
	if len(args) >= 2 && args[1] != "" {
		branch = args[1]
	}
	return remote, branch
}
