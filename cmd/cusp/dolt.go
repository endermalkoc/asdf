package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/endermalkoc/cusp/internal/app"
	"github.com/endermalkoc/cusp/internal/doltserver"
	"github.com/endermalkoc/cusp/internal/workspace"
)

var (
	flagRemoteUser string
	flagPushForce  bool
)

var doltCmd = &cobra.Command{
	Use:   "dolt",
	Short: "Sync the database with a Dolt remote (push/pull/fetch, manage remotes)",
	Long: "Cusp's store is a Dolt database, so syncing a version-controlled knowledge graph is\n" +
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
			fmt.Println("(no remotes — add one with `cusp dolt remote add <name> <url>`)")
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

// ownedServerDir resolves the workspace's server directory for owned-mode
// lifecycle commands, refusing when the workspace targets an external server
// (--dsn / $CUSP_DSN, or an explicit server port / shared server in metadata).
func ownedServerDir(verb string) (string, error) {
	cuspDir, err := workspace.ResolveCuspDir()
	if err != nil {
		return "", err
	}
	if flagDSN != "" || doltserver.ResolveServerMode(cuspDir) == doltserver.ServerModeExternal {
		return "", fmt.Errorf("`cusp dolt %s` manages this workspace's owned server, but it is configured to use an external server", verb)
	}
	return doltserver.ResolveServerDir(cuspDir), nil
}

var doltStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start (or adopt) the managed Dolt SQL server for this workspace",
	Long: "Starts a dolt sql-server for this workspace in the background. The server auto-starts\n" +
		"on demand, so manual start is rarely needed — use it for explicit control or diagnostics.\n" +
		"Idempotent: if a server is already running it reports the existing one.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		serverDir, err := ownedServerDir("start")
		if err != nil {
			return err
		}
		state, err := doltserver.Start(serverDir)
		if err != nil {
			return err
		}
		emit(state, fmt.Sprintf("dolt server running (pid %d, port %d)\n  data: %s\n  logs: %s",
			state.PID, state.Port, state.DataDir, doltserver.LogPath(serverDir)))
		return nil
	},
}

var doltStopForce bool

var doltStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the managed Dolt SQL server for this workspace",
	Long: "Gracefully stops the dolt sql-server managed for this workspace (flush → SIGTERM →\n" +
		"SIGKILL). It auto-restarts on the next command unless auto-start is disabled.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		serverDir, err := ownedServerDir("stop")
		if err != nil {
			return err
		}
		if err := doltserver.StopWithForce(serverDir, doltStopForce); err != nil {
			if errors.Is(err, doltserver.ErrServerNotRunning) {
				emit(map[string]any{"stopped": false, "running": false}, "dolt server was not running")
				return nil
			}
			return err
		}
		emit(map[string]any{"stopped": true}, "dolt server stopped")
		return nil
	},
}

var doltStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the Dolt server's status (mode, pid, port, data dir)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cuspDir, err := workspace.ResolveCuspDir()
		if err != nil {
			return err
		}
		if flagDSN != "" {
			emit(map[string]any{"mode": "external", "dsn": flagDSN},
				"mode:    external\nserver:  "+flagDSN)
			return nil
		}
		mode := doltserver.ResolveServerMode(cuspDir)
		serverDir := doltserver.ResolveServerDir(cuspDir)
		state, err := doltserver.IsRunning(serverDir)
		if err != nil {
			return err
		}
		if flagJSON {
			emit(map[string]any{
				"mode": mode.String(), "running": state.Running,
				"pid": state.PID, "port": state.Port, "data_dir": state.DataDir,
			}, "")
			return nil
		}
		if state.Running {
			fmt.Printf("mode:    %s\nrunning: yes (pid %d, port %d)\ndata:    %s\nlogs:    %s\n",
				mode, state.PID, state.Port, state.DataDir, doltserver.LogPath(serverDir))
		} else {
			fmt.Printf("mode:    %s\nrunning: no\n", mode)
		}
		return nil
	},
}

var (
	compactDays  int
	compactForce bool
)

var doltCompactCmd = &cobra.Command{
	Use:   "compact",
	Short: "Squash Dolt commits older than --days into one, reclaiming history storage",
	Long: "Squashes Dolt commits older than --days into a single base commit, preserving the\n" +
		"recent commits on top, then GCs to reclaim space. Reduces storage from auto-commit\n" +
		"history while keeping recent change tracking. For a full squash, use `cusp flatten`.\n\n" +
		"  cusp dolt compact --dry-run       # preview the commit breakdown\n" +
		"  cusp dolt compact --force         # squash commits older than 30 days\n" +
		"  cusp dolt compact --days 7 --force",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		if compactDays < 0 {
			return fmt.Errorf("--days must be non-negative")
		}
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()

		res, err := app.CompactDolt(ctx, ws, compactDays, flagDryRun || !compactForce, time.Now())
		if err != nil {
			return err
		}
		if flagJSON {
			emit(res, "")
			return nil
		}
		if res.TotalCommits <= 1 {
			fmt.Printf("Only %d commit(s). Nothing to compact.\n", res.TotalCommits)
			return nil
		}
		if res.Compacted {
			fmt.Printf("✓ compacted %d commits → %d\n  squashed: %d old → 1 base\n  preserved: %d recent\n",
				res.TotalCommits, res.RecentCommits+1, res.OldCommits, res.RecentCommits)
			return nil
		}
		fmt.Printf("Total commits:  %d\nOld (>%d days): %d\nRecent:         %d\nCutoff date:    %s\n",
			res.TotalCommits, res.CutoffDays, res.OldCommits, res.RecentCommits, res.CutoffDate)
		switch {
		case res.OldCommits <= 1:
			fmt.Println("Nothing to compact (0-1 old commits).")
		case flagDryRun:
			fmt.Printf("Would squash %d commits → %d. Run with --force to proceed.\n", res.TotalCommits, res.RecentCommits+1)
		default:
			fmt.Printf("Would squash %d old commits into 1, preserving %d recent. Use --force to confirm or --dry-run to preview.\n",
				res.OldCommits, res.RecentCommits)
		}
		return nil
	},
}

func init() {
	doltCmd.PersistentFlags().StringVar(&flagRemoteUser, "user", "",
		"remote auth user (password via DOLT_REMOTE_PASSWORD in the server env)")
	doltPushCmd.Flags().BoolVar(&flagPushForce, "force", false, "force-push (overwrite remote history)")
	syncCmd.PersistentFlags().StringVar(&flagRemoteUser, "user", "", "remote auth user")

	doltStopCmd.Flags().BoolVarP(&doltStopForce, "force", "f", false, "force-kill without waiting for graceful shutdown")
	doltCompactCmd.Flags().IntVar(&compactDays, "days", 30, "keep commits newer than N days")
	doltCompactCmd.Flags().BoolVarP(&compactForce, "force", "f", false, "confirm the commit squash")

	doltRemoteCmd.AddCommand(doltRemoteAddCmd, doltRemoteRemoveCmd, doltRemoteLsCmd)
	doltCmd.AddCommand(doltRemoteCmd, doltPushCmd, doltPullCmd, doltFetchCmd,
		doltStartCmd, doltStopCmd, doltStatusCmd, doltCompactCmd)
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
