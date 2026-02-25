// Command brain is a CLI tool for managing persistent AI agent memory.
//
// The brain file is always ~/.brain/agent.brain -- no path argument needed.
//
// Usage:
//
//	brain <command> [arguments...]
//
// Run 'brain help' for a full list of commands.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/glthr/brAIn/internal/brain"
	"github.com/glthr/brAIn/internal/daemon"
	"github.com/glthr/brAIn/internal/helpers"
)

func defaultBrainFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		fatalf("cannot determine home directory: %v\n", err)
	}
	return filepath.Join(home, ".brain", "agent.brain")
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	if cmd == "help" || cmd == "-h" || cmd == "--help" {
		usage()
		return
	}

	args := os.Args[2:]
	brainFile := defaultBrainFile()

	if cmd == "init" {
		cmdInit(brainFile, args)
		return
	}

	if cmd == "daemon" {
		fatalf("The 'daemon' command has been moved to a separate binary.\n" +
			"Use 'brain-daemon' directly, or install it as a service with 'brain install-service'.\n")
		return
	}

	if cmd == "config" {
		cmdConfig()
		return
	}

	if cmd == "install-service" {
		cmdInstallService(args)
		return
	}

	if cmd == "uninstall-service" {
		cmdUninstallService(args)
		return
	}

	if cmd == "daemon-status" {
		cmdDaemonStatus()
		return
	}
	if cmd == "daemon-restart" {
		cmdDaemonRestart()
		return
	}
	if cmd == "daemon-update" {
		cmdDaemonUpdate()
		return
	}

	cfg := daemon.LoadConfig()
	cliOpts := []brain.Option{brain.WithConsolidationInterval(0)}
	if cfg.LLMUrl != "" && cfg.LLMModel != "" {
		cliOpts = append(cliOpts, brain.WithLLM(cfg.LLMUrl, cfg.LLMModel, cfg.LLMApiKey))
	}
	if cfg.EmbedModel != "" {
		cliOpts = append(cliOpts, brain.WithEmbeddingModel(cfg.EmbedModel))
	}
	b, err := brain.New(brainFile, cliOpts...)
	if err != nil {
		fatalf("open brain: %v\n", err)
	}
	defer b.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	switch cmd {
	case "encode":
		cmdEncode(ctx, b, args)

	case "fact":
		cmdFact(ctx, b, args)
	case "recall":
		cmdRecall(ctx, b, args)
	case "episode":
		cmdEpisode(ctx, b, args)
	case "episodes":
		cmdEpisodes(ctx, b, args)
	case "working":
		cmdWorking(ctx, b, args)

	case "forget":
		cmdForget(ctx, b, args)
	case "pin":
		cmdPin(ctx, b, args)
	case "unpin":
		cmdUnpin(ctx, b, args)
	case "correct":
		cmdCorrect(ctx, b, args)
	case "explain":
		cmdExplain(ctx, b, args)

	case "attention":
		cmdAttention(ctx, b, args)
	case "context":
		cmdContext(ctx, b, args)

	case "goal":
		cmdGoal(ctx, b, args)
	case "resolve-goal":
		cmdResolveGoal(ctx, b, args)

	case "context-budget":
		cmdContextBudget(ctx, b, args)

	case "timeline":
		cmdTimeline(ctx, b, args)

	case "procedure":
		cmdProcedure(ctx, b, args)
	case "procedures":
		cmdProcedures(ctx, b)
	case "skills":
		cmdSkills(ctx, b)
	case "skill":
		cmdSkill(ctx, b, args)

	case "interact":
		cmdInteract(ctx, b, args)
	case "whoami":
		cmdWhoami(ctx, b)
	case "traits":
		cmdTraits(ctx, b, args)
	case "profile":
		cmdProfile(ctx, b, args)
	case "memories":
		cmdMemories(ctx, b, args)
	case "needs-analysis":
		cmdNeedsAnalysis(ctx, b)
	case "mark-analyzed":
		cmdMarkAnalyzed(ctx, b, args)
	case "contacts":
		cmdContacts(ctx, b, args)
	case "merge-contact":
		cmdMergeContact(ctx, b, args)
	case "history":
		cmdHistory(ctx, b, args)

	case "analyze-profiles":
		cmdAnalyzeProfiles(ctx, b)
	case "process-encodings":
		cmdProcessEncodings(ctx, b)
	case "consolidate":
		cmdConsolidate(ctx, b)
	case "activity":
		cmdActivity(ctx, b)
	case "stats":
		cmdStats(ctx, b)
	case "meta":
		cmdMeta(ctx, b, args)
	case "backup":
		cmdBackup(ctx, b)
	case "validate":
		cmdValidate(ctx, b)

	default:
		fatalf("unknown command: %s\nRun 'brain help' for usage.\n", cmd)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: brain <command> [arguments...]

Brain file: ~/.brain/agent.brain (always)

Commands:
  init          Create the brain file (if it doesn't exist)
  fact          Store a fact in semantic memory
  recall        Look up facts by keyword (falls back to keyword search without embedding model)
  episode       Record an episodic memory
  episodes      List recent episodes
  working       Store a working memory item
  encode        Encode a conversation turn (extracts memories via LLM)
  attention     Show current attentional focus (goals first, then working memories)
  context       One-shot start-of-turn context: whoami, attention, recall by topics, episodes, traits
  goal          Create a persistent goal (survives until resolved)
  resolve-goal  Mark a goal as completed and remove it
  context-budget Return ranked, deduplicated memories fitting within a token budget
  timeline      Show archived episodic memories (narrative history before semanticization)
  forget        Hard-delete a specific memory by ID
  pin           Pin a memory (salience=1.0, exempt from decay/forgetting)
  unpin         Remove the pin from a memory
  correct       Update memory content in-place (preserves history)
  explain       Trace a memory's lifecycle (source, salience, pinned status)
  procedure     Store a new procedure
  procedures    List learned procedures
  skills        List emerging skills (procedures learned from conversation or consolidation)
  skill         Search procedures by goal/intent and show steps (to apply an emerging or explicit skill)
  interact      Record an interaction (agent or human); use --reliability to signal behavioral trustworthiness
  whoami        Show the most recent human user
  traits        Show the person schema, behavioral priors, and confidence tier for a contact
  profile       Show full profile (info + person schema + trust dimensions) for a contact
  memories      List memories associated with a user
  needs-analysis List users with new data since last analysis
  mark-analyzed  Stamp a user as analyzed (used internally by the profiler)
  contacts      List known contacts
  history       Show interaction history for a contact
  analyze-profiles Run LLM-powered profile inference for all users with new data
  process-encodings Process raw conversation turns through LLM extraction
  consolidate   Run the consolidation sleep cycle (adaptive decay, skip pinned, archive before semanticize)
  install-service   Install daemon as a system service (launchd/systemd)
  uninstall-service Remove the daemon system service
  daemon-status     Show daemon status and last log lines
  daemon-restart    Rebuild daemon binary from source_path and restart service
  daemon-update     Trigger daemon to rebuild from source and restart (sends SIGUSR2; set source_path in config)
  activity      Show recent brain activity (daemon health check)
  config        Show current config values and file location
  stats         Show brain statistics
  meta          Show or set metadata
  backup        Copy brain to timestamped backup and record last_backup_at
  validate      Run read-only checks (schema, tables, row counts)

Run 'brain <command> --help' for details on each command.

Note: flags must come before positional text (Go convention).

Examples:
  brain init
  brain fact --tags go,concurrency --user user-alice --agent cursor Go channels are typed conduits
  brain recall goroutine
  brain episode --tags debugging,go --user user-alice --agent cursor Debugged deadlock using pprof
  brain working --user user-alice --agent cursor Currently reviewing PR #42
  brain procedure --name code_review --steps "read diff,check tests,comment"
  brain procedures
  brain skills
  brain skill "convert to PDF"
  brain encode --user user-alice --name Alice --agent cursor --prompt "How do I test Go?" --response "Use table-driven tests..."
  brain interact --id user-alice --name Alice --entity human --outcome success
  brain whoami
  brain traits user-alice
  brain memories user-alice
  brain needs-analysis
  brain profile user-alice
  brain contacts --type human
  brain attention
  brain context --topics "terminology,neuroscience"
  brain consolidate
  brain install-service
  brain uninstall-service
  brain daemon-status
  brain daemon-restart
  brain daemon-update
  brain stats
  brain meta
  brain backup
  brain validate
`)
}

func fatalf(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, format, a...)
	os.Exit(1)
}

func printMemories(mems []brain.Memory) {
	if len(mems) == 0 {
		fmt.Println("No results.")
		return
	}
	for _, m := range mems {
		extra := ""
		if m.UserID != "" {
			extra = fmt.Sprintf(" user=%s", m.UserID)
		}
		if m.Agent != "" {
			extra += fmt.Sprintf(" agent=%s", m.Agent)
		}
		if proj := m.Metadata["project"]; proj != "" {
			extra += fmt.Sprintf(" project=%s", proj)
		}
		fmt.Printf("[id=%d salience=%.2f retrievals=%d%s] %s\n",
			m.ID, m.Salience, m.Retrievals, extra, m.Content)
		if len(m.Tags) > 0 {
			fmt.Printf("  tags: %s\n", strings.Join(m.Tags, ", "))
		}
	}
}

func printKV(key, value string) {
	if value == "" {
		value = "(not set)"
	}
	fmt.Printf("  %-16s %s\n", key, value)
}

func cmdConfig() {
	cfg := daemon.LoadConfig()
	path := daemon.ConfigFilePath()

	exists := "yes"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		exists = "no (using defaults)"
	}

	fmt.Printf("Config file: %s (%s)\n", path, exists)
	fmt.Println()
	fmt.Printf("  %-25s %s\n", "consolidation_interval", cfg.ConsolidationInterval)
	fmt.Printf("  %-25s %s\n", "profile_interval", cfg.ProfileInterval)
	fmt.Printf("  %-25s %s\n", "llm_url", cfg.LLMUrl)
	fmt.Printf("  %-25s %s\n", "llm_model", cfg.LLMModel)
	if cfg.LLMApiKey != "" {
		fmt.Printf("  %-25s %s\n", "llm_api_key", "***")
	} else {
		fmt.Printf("  %-25s %s\n", "llm_api_key", "(not set)")
	}
	if cfg.EmbedModel != "" {
		fmt.Printf("  %-25s %s\n", "embed_model", cfg.EmbedModel)
	} else {
		fmt.Printf("  %-25s %s\n", "embed_model", "(not set — keyword search disabled)")
	}
}

func cmdInit(brainFile string, args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	desc := fs.String("desc", "", "Description")
	fs.Parse(args)

	dir := filepath.Dir(brainFile)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fatalf("init: create directory %s: %v\n", dir, err)
	}

	var opts []brain.Option
	opts = append(opts, brain.WithConsolidationInterval(0))
	if *desc != "" {
		opts = append(opts, brain.WithDescription(*desc))
	}

	b, err := brain.New(brainFile, opts...)
	if err != nil {
		fatalf("init: %v\n", err)
	}
	b.Close()

	if err := daemon.WriteDefaultConfig(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not create config: %v\n", err)
	} else {
		fmt.Printf("Created %s\n", daemon.ConfigFilePath())
	}

	fmt.Printf("Created %s\n", brainFile)
}

func cmdEncode(ctx context.Context, b *brain.Brain, args []string) {
	fs := flag.NewFlagSet("encode", flag.ExitOnError)
	user := fs.String("user", "", "User/contact ID (required)")
	name := fs.String("name", "", "User display name")
	agent := fs.String("agent", "", "AI agent that created this memory (e.g. cursor, claude-code)")
	project := fs.String("project", "", "Project name or path (auto-detected from CWD if not provided)")
	prompt := fs.String("prompt", "", "The user's prompt (required)")
	response := fs.String("response", "", "The agent's response (required)")
	fs.Parse(args)

	if *user == "" || *prompt == "" || *response == "" {
		fatalf("Usage: brain encode --user ID --name NAME [--agent NAME] --prompt TEXT --response TEXT\n")
	}

	proj := helpers.ResolveProject(*project, false)

	id, err := b.Encode(ctx, *user, *name, *agent, proj, *prompt, *response)
	if err != nil {
		fatalf("encode: %v\n", err)
	}

	fmt.Printf("Encoded conversation turn id=%d\n", id)
}

func cmdFact(ctx context.Context, b *brain.Brain, args []string) {
	fs := flag.NewFlagSet("fact", flag.ExitOnError)
	tags := fs.String("tags", "", "Comma-separated tags")
	salience := fs.Float64("salience", 0, "Salience (0-1). 0 = use default")
	user := fs.String("user", "", "Associate with this user/contact ID")
	agent := fs.String("agent", "", "AI agent that created this memory (e.g. cursor, claude-code)")
	project := fs.String("project", "", "Associate with this project name or path")
	fs.Parse(args)

	if fs.NArg() < 1 {
		fatalf("Usage: brain fact [--tags t1,t2] [--salience 0.8] [--user ID] [--agent NAME] [--project PATH] <text...>\n")
	}
	text := strings.Join(fs.Args(), " ")

	fact := brain.Fact{Content: text, Tags: helpers.ParseTags(*tags), UserID: *user, Agent: *agent}
	if *project != "" {
		fact.Metadata = map[string]string{"project": *project}
	}
	var id int64
	var err error
	if *salience > 0 {
		id, err = b.Semantic.StoreWithImportance(ctx, fact, *salience)
	} else {
		id, err = b.Semantic.Store(ctx, fact)
	}
	if err != nil {
		fatalf("store fact: %v\n", err)
	}
	fmt.Printf("Stored fact id=%d\n", id)
}

func cmdRecall(ctx context.Context, b *brain.Brain, args []string) {
	fs := flag.NewFlagSet("recall", flag.ExitOnError)
	limit := fs.Int("limit", 10, "Max results")
	project := fs.String("project", "", "Project name or path to scope recall (default: CWD)")
	all := fs.Bool("all", false, "Search across all projects")
	fs.Parse(args)

	if fs.NArg() < 1 {
		fatalf("Usage: brain recall [--limit N] [--project PATH] [--all] <keyword...>\n")
	}
	keyword := strings.Join(fs.Args(), " ")

	proj := helpers.ResolveProject(*project, *all)
	candidateLimit := helpers.RecallCandidateLimit(*limit, *all, proj)

	results, err := b.Semantic.Lookup(ctx, keyword, candidateLimit)
	if err != nil {
		fatalf("recall: %v\n", err)
	}
	if !*all && proj != "" {
		results = helpers.FilterMemoriesByProject(results, proj)
	}
	if *limit > 0 && len(results) > *limit {
		results = results[:*limit]
	}
	if len(results) == 0 {
		fmt.Println("No results.")
		return
	}
	for _, r := range results {
		fmt.Printf("[id=%d salience=%.2f] %s\n", r.ID, r.Salience, r.Content)
		if proj := r.Metadata["project"]; proj != "" {
			fmt.Printf("  project: %s\n", proj)
		}
		if len(r.Tags) > 0 {
			fmt.Printf("  tags: %s\n", strings.Join(r.Tags, ", "))
		}
	}
}

func cmdForget(ctx context.Context, b *brain.Brain, args []string) {
	fs := flag.NewFlagSet("forget", flag.ExitOnError)
	fs.Parse(args)
	if fs.NArg() < 1 {
		fatalf("Usage: brain forget <memory-id>\n")
	}
	id, err := strconv.ParseInt(fs.Arg(0), 10, 64)
	if err != nil {
		fatalf("forget: invalid id %q\n", fs.Arg(0))
	}
	if err := b.ForgetMemory(ctx, id); err != nil {
		fatalf("forget: %v\n", err)
	}
	fmt.Printf("Deleted memory id=%d\n", id)
}

func cmdPin(ctx context.Context, b *brain.Brain, args []string) {
	fs := flag.NewFlagSet("pin", flag.ExitOnError)
	fs.Parse(args)
	if fs.NArg() < 1 {
		fatalf("Usage: brain pin <memory-id>\n")
	}
	id, err := strconv.ParseInt(fs.Arg(0), 10, 64)
	if err != nil {
		fatalf("pin: invalid id %q\n", fs.Arg(0))
	}
	if err := b.PinMemory(ctx, id); err != nil {
		fatalf("pin: %v\n", err)
	}
	fmt.Printf("Pinned memory id=%d (salience=1.0, exempt from decay)\n", id)
}

func cmdUnpin(ctx context.Context, b *brain.Brain, args []string) {
	fs := flag.NewFlagSet("unpin", flag.ExitOnError)
	fs.Parse(args)
	if fs.NArg() < 1 {
		fatalf("Usage: brain unpin <memory-id>\n")
	}
	id, err := strconv.ParseInt(fs.Arg(0), 10, 64)
	if err != nil {
		fatalf("unpin: invalid id %q\n", fs.Arg(0))
	}
	if err := b.UnpinMemory(ctx, id); err != nil {
		fatalf("unpin: %v\n", err)
	}
	fmt.Printf("Unpinned memory id=%d (subject to normal decay again)\n", id)
}

func cmdCorrect(ctx context.Context, b *brain.Brain, args []string) {
	fs := flag.NewFlagSet("correct", flag.ExitOnError)
	fs.Parse(args)
	if fs.NArg() < 2 {
		fatalf("Usage: brain correct <memory-id> <new content...>\n")
	}
	id, err := strconv.ParseInt(fs.Arg(0), 10, 64)
	if err != nil {
		fatalf("correct: invalid id %q\n", fs.Arg(0))
	}
	newContent := strings.Join(fs.Args()[1:], " ")
	if err := b.CorrectMemory(ctx, id, newContent); err != nil {
		fatalf("correct: %v\n", err)
	}
	fmt.Printf("Updated memory id=%d\n", id)
}

func cmdExplain(ctx context.Context, b *brain.Brain, args []string) {
	fs := flag.NewFlagSet("explain", flag.ExitOnError)
	fs.Parse(args)
	if fs.NArg() < 1 {
		fatalf("Usage: brain explain <memory-id>\n")
	}
	id, err := strconv.ParseInt(fs.Arg(0), 10, 64)
	if err != nil {
		fatalf("explain: invalid id %q\n", fs.Arg(0))
	}
	exp, err := b.ExplainMemory(ctx, id)
	if err != nil {
		fatalf("explain: %v\n", err)
	}
	m := exp.Memory
	fmt.Printf("[id=%d type=%s salience=%.2f retrievals=%d]\n", m.ID, m.Type, m.Salience, m.Retrievals)
	fmt.Printf("  created:       %s\n", m.CreatedAt.Format("2006-01-02 15:04"))
	fmt.Printf("  last accessed: %s\n", m.LastAccessed.Format("2006-01-02 15:04"))
	if exp.Pinned {
		fmt.Println("  pinned:        yes (exempt from decay)")
	}
	fmt.Printf("  content:       %s\n", m.Content)
	if len(m.Tags) > 0 {
		fmt.Printf("  tags:          %s\n", strings.Join(m.Tags, ", "))
	}
	if len(exp.SourceIDs) > 0 {
		ids := make([]string, len(exp.SourceIDs))
		for i, sid := range exp.SourceIDs {
			ids[i] = strconv.FormatInt(sid, 10)
		}
		fmt.Printf("  compressed from episodes: %s\n", strings.Join(ids, ", "))
	}
	if exp.SemanticID != 0 {
		fmt.Printf("  compressed into semantic: id=%d\n", exp.SemanticID)
	}
}

func cmdGoal(ctx context.Context, b *brain.Brain, args []string) {
	fs := flag.NewFlagSet("goal", flag.ExitOnError)
	user := fs.String("user", "", "Associate with this user/contact ID")
	agent := fs.String("agent", "", "AI agent that created this goal")
	fs.Parse(args)
	if fs.NArg() < 1 {
		fatalf("Usage: brain goal [--user ID] [--agent NAME] <goal text...>\n")
	}
	text := strings.Join(fs.Args(), " ")
	id, err := b.Working.StoreGoal(ctx, text, *user, *agent)
	if err != nil {
		fatalf("goal: %v\n", err)
	}
	fmt.Printf("Goal stored id=%d\n", id)
}

func cmdResolveGoal(ctx context.Context, b *brain.Brain, args []string) {
	fs := flag.NewFlagSet("resolve-goal", flag.ExitOnError)
	fs.Parse(args)
	if fs.NArg() < 1 {
		fatalf("Usage: brain resolve-goal <goal-id>\n")
	}
	id, err := strconv.ParseInt(fs.Arg(0), 10, 64)
	if err != nil {
		fatalf("resolve-goal: invalid id %q\n", fs.Arg(0))
	}
	if err := b.Working.ResolveGoal(ctx, id); err != nil {
		fatalf("resolve-goal: %v\n", err)
	}
	fmt.Printf("Goal id=%d resolved and removed.\n", id)
}

func cmdContextBudget(ctx context.Context, b *brain.Brain, args []string) {
	fs := flag.NewFlagSet("context-budget", flag.ExitOnError)
	topic := fs.String("topic", "", "Topic to recall context for (required)")
	tokens := fs.Int("tokens", 2000, "Token budget (chars/4 estimate)")
	fs.Parse(args)

	if *topic == "" && fs.NArg() > 0 {
		*topic = strings.Join(fs.Args(), " ")
	}
	if *topic == "" {
		fatalf("Usage: brain context-budget --topic <topic> [--tokens N]\n")
	}

	mems, err := b.Semantic.ContextBudget(ctx, *topic, *tokens)
	if err != nil {
		fatalf("context-budget: %v\n", err)
	}
	if len(mems) == 0 {
		fmt.Println("No memories found within budget.")
		return
	}
	usedTokens := 0
	for _, m := range mems {
		tokens := len(m.Content) / 4
		usedTokens += tokens
		fmt.Printf("[id=%d salience=%.2f ~%dtok] %s\n", m.ID, m.Salience, tokens, m.Content)
		if len(m.Tags) > 0 {
			fmt.Printf("  tags: %s\n", strings.Join(m.Tags, ", "))
		}
	}
	fmt.Printf("\n%d memories, ~%d tokens used of %d budget\n", len(mems), usedTokens, *tokens)
}

func cmdTimeline(ctx context.Context, b *brain.Brain, args []string) {
	fs := flag.NewFlagSet("timeline", flag.ExitOnError)
	limit := fs.Int("limit", 20, "Max results")
	user := fs.String("user", "", "Filter by user ID")
	fs.Parse(args)

	archives, err := b.Timeline(ctx, *user, *limit)
	if err != nil {
		fatalf("timeline: %v\n", err)
	}
	if len(archives) == 0 {
		fmt.Println("No archived episodes found. (Episodes are archived when they are semanticized into facts during consolidation.)")
		return
	}
	for _, a := range archives {
		sem := ""
		if a.SemanticID != 0 {
			sem = fmt.Sprintf(" → semantic id=%d", a.SemanticID)
		}
		fmt.Printf("[%s%s] %s\n", a.CreatedAt.Format("2006-01-02 15:04"), sem, a.Content)
		if len(a.Tags) > 0 {
			fmt.Printf("  tags: %s\n", strings.Join(a.Tags, ", "))
		}
	}
}

func cmdEpisode(ctx context.Context, b *brain.Brain, args []string) {
	fs := flag.NewFlagSet("episode", flag.ExitOnError)
	tags := fs.String("tags", "", "Comma-separated tags")
	salience := fs.Float64("salience", 0, "Salience (0-1). 0 = use default")
	user := fs.String("user", "", "Associate with this user/contact ID")
	agent := fs.String("agent", "", "AI agent that created this memory (e.g. cursor, claude-code)")
	project := fs.String("project", "", "Associate with this project name or path")
	fs.Parse(args)

	if fs.NArg() < 1 {
		fatalf("Usage: brain episode [--tags t1,t2] [--salience 0.8] [--user ID] [--agent NAME] [--project PATH] <text...>\n")
	}
	text := strings.Join(fs.Args(), " ")

	ep := brain.Episode{Content: text, Tags: helpers.ParseTags(*tags), UserID: *user, Agent: *agent}
	if *project != "" {
		ep.Metadata = map[string]string{"project": *project}
	}
	var id int64
	var err error
	if *salience > 0 {
		id, err = b.Episodic.RecordWithImportance(ctx, ep, *salience)
	} else {
		id, err = b.Episodic.Record(ctx, ep)
	}
	if err != nil {
		fatalf("record episode: %v\n", err)
	}
	fmt.Printf("Recorded episode id=%d\n", id)
}

func cmdEpisodes(ctx context.Context, b *brain.Brain, args []string) {
	fs := flag.NewFlagSet("episodes", flag.ExitOnError)
	limit := fs.Int("limit", 10, "Max results")
	tag := fs.String("tag", "", "Filter by tag")
	fs.Parse(args)

	if *tag != "" {
		results, err := b.Episodic.Search(ctx, brain.EpisodicSearchOpts{
			Tags:  []string{*tag},
			Limit: *limit,
		})
		if err != nil {
			fatalf("search episodes: %v\n", err)
		}
		printMemories(results)
		return
	}

	results, err := b.Episodic.Recent(ctx, *limit)
	if err != nil {
		fatalf("recent episodes: %v\n", err)
	}
	printMemories(results)
}

func cmdWorking(ctx context.Context, b *brain.Brain, args []string) {
	fs := flag.NewFlagSet("working", flag.ExitOnError)
	ttl := fs.Duration("ttl", 0, "Custom TTL (e.g. 1h, 15m). 0 = use default")
	user := fs.String("user", "", "Associate with this user/contact ID")
	agent := fs.String("agent", "", "AI agent that created this memory (e.g. cursor, claude-code)")
	fs.Parse(args)

	if fs.NArg() < 1 {
		fatalf("Usage: brain working [--ttl 1h] [--user ID] [--agent NAME] <text...>\n")
	}
	text := strings.Join(fs.Args(), " ")

	var id int64
	var err error
	if *user != "" {
		if *agent != "" {
			id, err = b.Working.StoreForUserWithAgent(ctx, text, nil, *user, *agent)
		} else {
			id, err = b.Working.StoreForUser(ctx, text, nil, *user)
		}
	} else if *ttl > 0 {
		if *agent != "" {
			id, err = b.Working.StoreWithTTLAndAgent(ctx, text, nil, *ttl, *agent)
		} else {
			id, err = b.Working.StoreWithTTL(ctx, text, nil, *ttl)
		}
	} else {
		if *agent != "" {
			id, err = b.Working.StoreWithAgent(ctx, text, nil, *agent)
		} else {
			id, err = b.Working.Store(ctx, text, nil)
		}
	}
	if err != nil {
		fatalf("store working: %v\n", err)
	}
	fmt.Printf("Stored working memory id=%d\n", id)
}

func cmdAttention(ctx context.Context, b *brain.Brain, args []string) {
	fs := flag.NewFlagSet("attention", flag.ExitOnError)
	limit := fs.Int("limit", 10, "Max results")
	fs.Parse(args)

	goals, _ := b.Working.ListGoals(ctx)
	if len(goals) > 0 {
		fmt.Println("=== Active Goals ===")
		for _, g := range goals {
			pinMark := ""
			if g.Metadata["pinned"] == "true" {
				pinMark = " [pinned]"
			}
			fmt.Printf("[goal id=%d%s] %s\n", g.ID, pinMark, g.Content)
		}
		fmt.Println()
	}

	results, err := b.Working.Recent(ctx, *limit)
	if err != nil {
		fatalf("recent working: %v\n", err)
	}
	if len(results) == 0 && len(goals) == 0 {
		fmt.Println("No active working memories or goals.")
		fmt.Println("(Working memory is short-lived: items expire in ~30 min, and encoded conversation turns are removed after the daemon processes them. Store scratchpad with: brain working --user <id> <text>)")
		return
	}
	if len(results) > 0 {
		if len(goals) > 0 {
			fmt.Println("=== Working Memory ===")
		}
		printMemories(results)
	}
}

func cmdContext(ctx context.Context, b *brain.Brain, args []string) {
	fs := flag.NewFlagSet("context", flag.ExitOnError)
	topics := fs.String("topics", "", "Comma-separated topics for recall and episodes (1-3 keywords from the user prompt)")
	fs.Parse(args)

	profile, err := b.Social.MostRecentHuman(ctx)
	if err != nil || profile == nil {
		fmt.Println("user: unknown")
		return
	}
	userID := profile.ID
	name := profile.Name
	if name == "" {
		name = userID
	}

	fmt.Println("=== Whoami ===")
	fmt.Printf("%s (id=%s interactions=%d)\n\n", name, userID, profile.InteractionCount)

	// Attention
	goals, _ := b.Working.ListGoals(ctx)
	if len(goals) > 0 {
		fmt.Println("=== Active Goals ===")
		for _, g := range goals {
			pinMark := ""
			if g.Metadata["pinned"] == "true" {
				pinMark = " [pinned]"
			}
			fmt.Printf("[goal id=%d%s] %s\n", g.ID, pinMark, g.Content)
		}
		fmt.Println()
	}
	workingResults, _ := b.Working.Recent(ctx, 10)
	if len(workingResults) > 0 {
		if len(goals) > 0 {
			fmt.Println("=== Working Memory ===")
		} else {
			fmt.Println("=== Attention ===")
		}
		printMemories(workingResults)
		fmt.Println()
	}
	if len(goals) == 0 && len(workingResults) == 0 {
		fmt.Println("=== Attention ===")
		fmt.Println("No active working memories or goals.")
		fmt.Println()
	}

	proj := helpers.ResolveProject("", false)
	limit := 10
	candidateLimit := helpers.RecallCandidateLimit(limit, false, proj)

	var topicList []string
	if *topics != "" {
		for _, t := range strings.Split(*topics, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				topicList = append(topicList, t)
			}
		}
	}
	for _, topic := range topicList {
		fmt.Printf("=== Recall %q ===\n", topic)
		results, errLookup := b.Semantic.Lookup(ctx, topic, candidateLimit)
		if errLookup != nil {
			fmt.Printf("recall: %v\n", errLookup)
		} else {
			filtered := helpers.FilterMemoriesByProject(results, proj)
			if limit > 0 && len(filtered) > limit {
				filtered = filtered[:limit]
			}
			if len(filtered) == 0 {
				fmt.Println("No results.")
			} else {
				for _, r := range filtered {
					fmt.Printf("[id=%d salience=%.2f] %s\n", r.ID, r.Salience, r.Content)
					if p := r.Metadata["project"]; p != "" {
						fmt.Printf("  project: %s\n", p)
					}
					if len(r.Tags) > 0 {
						fmt.Printf("  tags: %s\n", strings.Join(r.Tags, ", "))
					}
				}
			}
		}
		fmt.Printf("\n=== Episodes --tag %q ===\n", topic)
		epTag, errEp := b.Episodic.Search(ctx, brain.EpisodicSearchOpts{Tags: []string{topic}, Limit: limit})
		if errEp != nil {
			fmt.Printf("episodes: %v\n", errEp)
		} else {
			printMemories(epTag)
		}
		fmt.Println()
	}

	fmt.Println("=== Episodes (recent 5) ===")
	epRecent, _ := b.Episodic.Recent(ctx, 5)
	printMemories(epRecent)
	fmt.Println()

	fmt.Printf("=== Traits %s ===\n", userID)
	socialSchema, err := b.Social.GetSocialSchema(ctx, userID)
	if err != nil {
		fmt.Printf("get social schema: %v\n", err)
	} else {
		behavioralPriors, _ := b.Social.GetBehavioralPriors(ctx, userID)
		if socialSchema == "" && behavioralPriors == "" {
			fmt.Println("No profile yet.")
		} else {
			if p, err := b.Social.GetContactProfile(ctx, userID); err == nil {
				fmt.Printf("Confidence: %s (interactions=%d, memories=%d)\n\n",
					p.ConfidenceTier, p.InteractionCount, p.MemoryCount)
			}
			if socialSchema != "" {
				fmt.Println("Social Schema:")
				fmt.Println(socialSchema)
			}
			if behavioralPriors != "" {
				if socialSchema != "" {
					fmt.Println()
				}
				fmt.Println("Behavioral Priors:")
				fmt.Println(behavioralPriors)
			}
		}
	}
}

func cmdProcedure(ctx context.Context, b *brain.Brain, args []string) {
	fs := flag.NewFlagSet("procedure", flag.ExitOnError)
	name := fs.String("name", "", "Procedure name (required)")
	desc := fs.String("desc", "", "Description")
	steps := fs.String("steps", "", "Comma-separated steps")
	rate := fs.Float64("rate", 0.5, "Initial success rate")
	fs.Parse(args)

	if *name == "" {
		fatalf("Usage: brain procedure --name NAME [--desc DESC] [--steps s1,s2] [--rate 0.8]\n")
	}

	var stepSlice []string
	if *steps != "" {
		stepSlice = strings.Split(*steps, ",")
		for i := range stepSlice {
			stepSlice[i] = strings.TrimSpace(stepSlice[i])
		}
	}

	id, err := b.Procedural.Store(ctx, brain.Procedure{
		Name:        *name,
		Description: *desc,
		Steps:       stepSlice,
		SuccessRate: *rate,
	})
	if err != nil {
		fatalf("store procedure: %v\n", err)
	}
	fmt.Printf("Stored procedure id=%d name=%s\n", id, *name)
}

func cmdProcedures(ctx context.Context, b *brain.Brain) {
	procs, err := b.Procedural.All(ctx)
	if err != nil {
		fatalf("list procedures: %v\n", err)
	}
	if len(procs) == 0 {
		fmt.Println("No procedures stored.")
		return
	}
	for _, p := range procs {
		fmt.Printf("%-20s  success=%.0f%%  used=%d  %s\n",
			p.Name, p.SuccessRate*100, p.UseCount, p.Description)
		if len(p.Steps) > 0 {
			for i, s := range p.Steps {
				fmt.Printf("  %d. %s\n", i+1, s)
			}
		}
	}
}

func cmdSkills(ctx context.Context, b *brain.Brain) {
	procs, err := b.Procedural.Emerging(ctx)
	if err != nil {
		fatalf("list emerging skills: %v\n", err)
	}
	if len(procs) == 0 {
		fmt.Println("No emerging skills yet. Skills are learned from conversation or consolidation (e.g. when you teach a how-to or the agent discovers what works).")
		return
	}
	for _, p := range procs {
		fmt.Printf("%-20s  success=%.0f%%  used=%d  %s\n",
			p.Name, p.SuccessRate*100, p.UseCount, p.Description)
		if len(p.Steps) > 0 {
			for i, s := range p.Steps {
				fmt.Printf("  %d. %s\n", i+1, s)
			}
		}
	}
}

func cmdSkill(ctx context.Context, b *brain.Brain, args []string) {
	query := strings.TrimSpace(strings.Join(args, " "))
	if query == "" {
		fmt.Fprintf(os.Stderr, "Usage: brain skill <goal or intent>\n")
		fmt.Fprintf(os.Stderr, "Example: brain skill \"convert to PDF\"\n")
		os.Exit(1)
	}
	procs, err := b.Procedural.Search(ctx, query, 5)
	if err != nil {
		fatalf("search procedures: %v\n", err)
	}
	if len(procs) == 0 {
		fmt.Println("No matching procedures. Try 'brain procedures' to list all, or add one with 'brain procedure --name X --steps a,b,c'.")
		return
	}
	for _, p := range procs {
		fmt.Printf("%s (success=%.0f%% used=%d)\n", p.Name, p.SuccessRate*100, p.UseCount)
		if p.Description != "" {
			fmt.Printf("  %s\n", p.Description)
		}
		if len(p.Steps) > 0 {
			for i, s := range p.Steps {
				fmt.Printf("  %d. %s\n", i+1, s)
			}
		}
		fmt.Println()
	}
}

func cmdWhoami(ctx context.Context, b *brain.Brain) {
	profile, err := b.Social.MostRecentHuman(ctx)
	if err != nil {
		fmt.Println("user: unknown")
		return
	}
	name := profile.Name
	if name == "" {
		name = profile.ID
	}
	fmt.Printf("%s (id=%s interactions=%d)\n",
		name, profile.ID, profile.InteractionCount)
}

func cmdInteract(ctx context.Context, b *brain.Brain, args []string) {
	fs := flag.NewFlagSet("interact", flag.ExitOnError)
	id := fs.String("id", "", "Contact ID (required)")
	name := fs.String("name", "", "Contact name")
	entity := fs.String("entity", "agent", "Entity type: agent or human")
	itype := fs.String("type", "interaction", "Interaction type")
	outcome := fs.String("outcome", "", "Outcome description (required)")
	valence := fs.Float64("valence", 0.5, "Valence score (-1 to 1, agents only; ignored for humans)")
	reliability := fs.Float64("reliability", -1, "Reliability signal 0–1 (agents only): did the agent behave within expected parameters? -1 = not specified")
	notes := fs.String("notes", "", "Free-form notes")
	fs.Parse(args)

	if *id == "" || *outcome == "" {
		fatalf("Usage: brain interact --id ID --outcome TEXT [--name NAME] [--entity agent|human] [--type TYPE] [--reliability 0.9] [--notes TEXT]\nFor agents only: [--valence 0.8]\n")
	}

	et := brain.EntityTypeAgent
	if *entity == "human" {
		et = brain.EntityTypeHuman
	}

	var metadata map[string]string
	if *reliability >= 0 && *reliability <= 1 {
		metadata = map[string]string{
			"reliability": fmt.Sprintf("%.4f", *reliability),
		}
	}
	valenceVal := *valence
	if et == brain.EntityTypeHuman {
		valenceVal = 0 // no score for human interactions
	}
	iid, err := b.Social.RecordInteraction(ctx, brain.Interaction{
		ContactID:   *id,
		ContactName: *name,
		EntityType:  et,
		Type:        *itype,
		Outcome:     *outcome,
		Valence:     valenceVal,
		Notes:       *notes,
		Metadata:    metadata,
	})
	if err != nil {
		fatalf("record interaction: %v\n", err)
	}
	fmt.Printf("Recorded interaction id=%d with %s (%s)\n", iid, *id, *entity)
}

func cmdMergeContact(ctx context.Context, b *brain.Brain, args []string) {
	if len(args) < 2 {
		fatalf("Usage: brain merge-contact <from-id> <to-id>\nExample: brain merge-contact user-unknown user-alice\n")
	}
	fromID, toID := args[0], args[1]
	if fromID == toID {
		fatalf("from and to IDs must be different\n")
	}
	n, err := b.MergeContact(ctx, fromID, toID)
	if err != nil {
		fatalf("merge contact: %v\n", err)
	}
	fmt.Printf("Merged %s → %s (%d records reassigned)\n", fromID, toID, n)
}

func cmdContacts(ctx context.Context, b *brain.Brain, args []string) {
	fs := flag.NewFlagSet("contacts", flag.ExitOnError)
	entity := fs.String("type", "", "Filter by entity type: agent, human (default: all)")
	fs.Parse(args)

	var profiles []brain.ContactProfile
	var err error

	switch *entity {
	case "agent":
		profiles, err = b.Social.ListAgents(ctx)
	case "human":
		profiles, err = b.Social.ListHumans(ctx)
	default:
		profiles, err = b.Social.ListAll(ctx)
	}
	if err != nil {
		fatalf("list contacts: %v\n", err)
	}
	if len(profiles) == 0 {
		fmt.Println("No contacts found.")
		return
	}
	for _, p := range profiles {
		if p.EntityType == brain.EntityTypeHuman {
			fmt.Printf("[%s] %-20s  interactions=%d\n",
				p.EntityType, p.Name, p.InteractionCount)
		} else {
			fmt.Printf("[%s] %-20s trust=%.2f  interactions=%d  avg_valence=%.2f\n",
				p.EntityType, p.Name, p.Trust, p.InteractionCount, p.AvgValence)
		}
		if p.Notes != "" {
			fmt.Printf("  notes: %s\n", p.Notes)
		}
	}
}

func cmdHistory(ctx context.Context, b *brain.Brain, args []string) {
	fs := flag.NewFlagSet("history", flag.ExitOnError)
	limit := fs.Int("limit", 10, "Max results")
	fs.Parse(args)

	if fs.NArg() < 1 {
		fatalf("Usage: brain history [--limit N] <contact-id>\n")
	}
	contactID := fs.Arg(0)

	interactions, err := b.Social.InteractionHistory(ctx, contactID, *limit)
	if err != nil {
		fatalf("history: %v\n", err)
	}
	if len(interactions) == 0 {
		fmt.Println("No interactions found.")
		return
	}
	for _, i := range interactions {
		if i.EntityType == brain.EntityTypeHuman {
			fmt.Printf("[%s] type=%s outcome=%s\n", i.EntityType, i.Type, i.Outcome)
		} else {
			fmt.Printf("[%s] type=%s outcome=%s valence=%.2f\n",
				i.EntityType, i.Type, i.Outcome, i.Valence)
		}
		if i.Notes != "" {
			fmt.Printf("  %s\n", i.Notes)
		}
	}
}

func cmdTraits(ctx context.Context, b *brain.Brain, args []string) {
	fs := flag.NewFlagSet("traits", flag.ExitOnError)
	fs.Parse(args)

	if fs.NArg() < 1 {
		fatalf("Usage: brain traits <contact-id>\n")
	}
	contactID := fs.Arg(0)

	socialSchema, err := b.Social.GetSocialSchema(ctx, contactID)
	if err != nil {
		fatalf("get social schema: %v\n", err)
	}
	behavioralPriors, _ := b.Social.GetBehavioralPriors(ctx, contactID)

	if socialSchema == "" && behavioralPriors == "" {
		fmt.Println("No profile yet.")
		return
	}

	if p, err := b.Social.GetContactProfile(ctx, contactID); err == nil {
		fmt.Printf("Confidence: %s (interactions=%d, memories=%d)\n\n",
			p.ConfidenceTier, p.InteractionCount, p.MemoryCount)
	}

	if socialSchema != "" {
		fmt.Println("Social Schema:")
		fmt.Println(socialSchema)
	}
	if behavioralPriors != "" {
		if socialSchema != "" {
			fmt.Println()
		}
		fmt.Println("Behavioral Priors:")
		fmt.Println(behavioralPriors)
	}
}

func cmdProfile(ctx context.Context, b *brain.Brain, args []string) {
	fs := flag.NewFlagSet("profile", flag.ExitOnError)
	fs.Parse(args)

	if fs.NArg() < 1 {
		fatalf("Usage: brain profile <contact-id>\n")
	}
	contactID := fs.Arg(0)

	p, err := b.Social.GetContactProfile(ctx, contactID)
	if err != nil {
		fatalf("profile: %v\n", err)
	}

	fmt.Printf("%s (%s)\n", p.Name, p.ID)
	fmt.Printf("  type:          %s\n", p.EntityType)
	if p.EntityType == brain.EntityTypeAgent {
		fmt.Printf("  trust:         %.2f (legacy)\n", p.Trust)
		fmt.Printf("  competence:    %.2f\n", p.CompetenceTrust)
		fmt.Printf("  reliability:   %.2f\n", p.ReliabilityTrust)
	}
	fmt.Printf("  confidence:    %s\n", p.ConfidenceTier)
	fmt.Printf("  interactions:  %d\n", p.InteractionCount)
	fmt.Printf("  memories:      %d\n", p.MemoryCount)
	if p.EntityType == brain.EntityTypeAgent {
		fmt.Printf("  avg_valence:   %.2f\n", p.AvgValence)
	}
	if p.Notes != "" {
		fmt.Printf("  notes:         %s\n", p.Notes)
	}
	if p.SocialSchema != "" {
		fmt.Printf("\n  Social Schema:\n  %s\n", strings.ReplaceAll(p.SocialSchema, "\n", "\n  "))
	}
	if p.BehavioralPriors != "" {
		fmt.Printf("\n  Behavioral Priors:\n  %s\n", strings.ReplaceAll(p.BehavioralPriors, "\n", "\n  "))
	}
}

func cmdMemories(ctx context.Context, b *brain.Brain, args []string) {
	fs := flag.NewFlagSet("memories", flag.ExitOnError)
	limit := fs.Int("limit", 50, "Max results")
	fs.Parse(args)

	if fs.NArg() < 1 {
		fatalf("Usage: brain memories [--limit N] <user-id>\n")
	}
	userID := fs.Arg(0)

	mems, err := b.MemoriesForUser(ctx, userID, *limit)
	if err != nil {
		fatalf("memories: %v\n", err)
	}
	if len(mems) == 0 {
		fmt.Printf("No memories associated with %s.\n", userID)
		return
	}
	for _, m := range mems {
		fmt.Printf("[id=%d %s salience=%.2f] %s\n",
			m.ID, m.Type, m.Salience, m.Content)
		if len(m.Tags) > 0 {
			fmt.Printf("  tags: %s\n", strings.Join(m.Tags, ", "))
		}
	}
}

func cmdNeedsAnalysis(ctx context.Context, b *brain.Brain) {
	users, err := b.Social.HumansNeedingAnalysis(ctx)
	if err != nil {
		fatalf("needs-analysis: %v\n", err)
	}
	if len(users) == 0 {
		fmt.Println("All user profiles are up to date.")
		return
	}
	for _, u := range users {
		fmt.Printf("%s (id=%s interactions=%d)\n",
			u.Name, u.ID, u.InteractionCount)
	}
}

func cmdMarkAnalyzed(ctx context.Context, b *brain.Brain, args []string) {
	fs := flag.NewFlagSet("mark-analyzed", flag.ExitOnError)
	fs.Parse(args)
	if fs.NArg() < 1 {
		fatalf("Usage: brain mark-analyzed <user-id>\n")
	}
	userID := fs.Arg(0)
	if err := b.Social.MarkAnalyzed(ctx, userID); err != nil {
		fatalf("mark-analyzed: %v\n", err)
	}
	fmt.Printf("Marked %s as analyzed.\n", userID)
}

func cmdAnalyzeProfiles(ctx context.Context, b *brain.Brain) {
	start := time.Now()
	results, err := b.AnalyzeProfiles(ctx)
	if err != nil {
		fatalf("analyze-profiles: %v\n", err)
	}
	if len(results) == 0 {
		fmt.Println("No users needed profile analysis.")
		return
	}
	fmt.Printf("Profile analysis completed in %v\n", time.Since(start).Round(time.Millisecond))
	for _, r := range results {
		if r.Skipped {
			fmt.Printf("  %s: skipped (%s)\n", r.UserID, r.SkipReason)
		} else {
			ssStatus := "unchanged"
			if r.SocialSchemaUpdated {
				ssStatus = "updated"
			}
			bpStatus := "unchanged"
			if r.BehavioralPriorsUpdated {
				bpStatus = "updated"
			}
			fmt.Printf("  %s: social_schema %s, behavioral_priors %s (%d memories analyzed)\n", r.UserID, ssStatus, bpStatus, r.NewMemories)
		}
	}
}

func cmdProcessEncodings(ctx context.Context, b *brain.Brain) {
	start := time.Now()
	stats, err := b.ProcessEncodings(ctx)
	if err != nil {
		fatalf("process-encodings: %v\n", err)
	}
	if stats == nil {
		fmt.Println("No batch encoder configured (LLM required). Skipping.")
		return
	}
	if stats.Processed == 0 {
		fmt.Println("No pending conversation encodings to process.")
		return
	}
	fmt.Printf("Encoding processing completed in %v\n", time.Since(start).Round(time.Millisecond))
	fmt.Printf("  processed:    %d\n", stats.Processed)
	fmt.Printf("  episodes:     %d\n", stats.Episodes)
	fmt.Printf("  facts:        %d\n", stats.Facts)
	fmt.Printf("  interactions: %d\n", stats.Interactions)
	fmt.Printf("  errors:       %d\n", stats.Errors)
}

func cmdConsolidate(ctx context.Context, b *brain.Brain) {
	start := time.Now()
	stats, err := b.Consolidate(ctx)
	if err != nil {
		fatalf("consolidate: %v\n", err)
	}
	fmt.Printf("Consolidation completed in %v\n", time.Since(start).Round(time.Millisecond))
	fmt.Printf("  expired:        %d\n", stats.Expired)
	fmt.Printf("  decayed:        %d\n", stats.Decayed)
	fmt.Printf("  transferred:    %d\n", stats.Transferred)
	fmt.Printf("  forgotten:      %d\n", stats.Forgotten)
	fmt.Printf("  semanticized:   %d\n", stats.Semanticized)
	fmt.Printf("  embedded:       %d\n", stats.Embedded)
}

func cmdActivity(ctx context.Context, b *brain.Brain) {
	report, err := b.Activity(ctx)
	if err != nil {
		fatalf("activity: %v\n", err)
	}

	total := report.MemoryCounts["working"] + report.MemoryCounts["episodic"] +
		report.MemoryCounts["semantic"] + report.MemoryCounts["procedural"]

	fmt.Println("Memory")
	fmt.Printf("  total:      %d (working=%d episodic=%d semantic=%d procedural=%d)\n",
		total,
		report.MemoryCounts["working"],
		report.MemoryCounts["episodic"],
		report.MemoryCounts["semantic"],
		report.MemoryCounts["procedural"])

	fmt.Println("Social")
	fmt.Printf("  contacts:   %d humans, %d agents\n", report.HumanCount, report.AgentCount)

	fmt.Println("Pending work")
	fmt.Printf("  encodings:  %d conversation(s) waiting for LLM extraction\n", report.PendingIngests)
	fmt.Printf("  profiling:  %d user(s) need analysis\n", report.UsersNeedingAnalysis)

	fmt.Println("LLM")
	if report.LLMConfigured {
		status := "reachable"
		if !report.LLMAvailable {
			status = "NOT reachable"
		}
		fmt.Printf("  status:     configured, %s\n", status)
	} else {
		fmt.Printf("  status:     not configured\n")
	}

	fmt.Println("Recent consolidations")
	if len(report.RecentConsolidations) == 0 {
		fmt.Println("  (none)")
	} else {
		for _, e := range report.RecentConsolidations {
			ago := time.Since(e.CreatedAt).Round(time.Second)
			fmt.Printf("  [%s] (%s ago) %s\n", e.CreatedAt.Format("2006-01-02 15:04"), ago, summarizeStats(e.StatsJSON))
		}
	}
}

func summarizeStats(statsJSON string) string {
	var s brain.ConsolidationStats
	if err := json.Unmarshal([]byte(statsJSON), &s); err != nil {
		return statsJSON
	}
	parts := []string{}
	if s.Expired > 0 {
		parts = append(parts, fmt.Sprintf("expired=%d", s.Expired))
	}
	if s.Decayed > 0 {
		parts = append(parts, fmt.Sprintf("decayed=%d", s.Decayed))
	}
	if s.Transferred > 0 {
		parts = append(parts, fmt.Sprintf("transferred=%d", s.Transferred))
	}
	if s.Forgotten > 0 {
		parts = append(parts, fmt.Sprintf("forgotten=%d", s.Forgotten))
	}
	if s.Semanticized > 0 {
		parts = append(parts, fmt.Sprintf("semanticized=%d", s.Semanticized))
	}
	if len(parts) == 0 {
		return "no changes"
	}
	return strings.Join(parts, " ")
}

func cmdStats(ctx context.Context, b *brain.Brain) {
	wc, _ := b.Working.Count(ctx)
	sc, _ := b.Semantic.Count(ctx)
	ec, _ := b.Episodic.Count(ctx)
	pc, _ := b.Procedural.Count(ctx)
	agents, _ := b.Social.ListAgents(ctx)
	humans, _ := b.Social.ListHumans(ctx)
	meta, _ := b.Meta(ctx)

	fmt.Println("Memory")
	fmt.Printf("  working:    %d\n", wc)
	fmt.Printf("  semantic:   %d\n", sc)
	fmt.Printf("  episodic:   %d\n", ec)
	fmt.Printf("  procedural: %d\n", pc)
	fmt.Println("Social")
	fmt.Printf("  agents:     %d\n", len(agents))
	fmt.Printf("  humans:     %d\n", len(humans))
	if meta != nil {
		fmt.Println("Brain")
		fmt.Printf("  created:    %s\n", meta.CreatedAt)
		if meta.Description != "" {
			fmt.Printf("  desc:       %s\n", meta.Description)
		}
		if meta.LastBackupAt != "" {
			fmt.Printf("  backed up:  %s\n", meta.LastBackupAt)
		}
	}
}

func cmdBackup(ctx context.Context, b *brain.Brain) {
	brainPath := defaultBrainFile()
	home, _ := os.UserHomeDir()
	backupsDir := filepath.Join(home, ".brain", "backups")
	if err := os.MkdirAll(backupsDir, 0o755); err != nil {
		fatalf("backup: create backups dir: %v\n", err)
	}
	ts := time.Now().UTC().Format("20060102-150405")
	backupPath := filepath.Join(backupsDir, "agent.brain."+ts)
	data, err := os.ReadFile(brainPath)
	if err != nil {
		fatalf("backup: read brain file: %v\n", err)
	}
	if err := os.WriteFile(backupPath, data, 0o644); err != nil {
		fatalf("backup: write backup file: %v\n", err)
	}
	if err := b.RecordBackup(ctx); err != nil {
		fatalf("backup: record timestamp: %v\n", err)
	}
	fmt.Printf("Backed up to %s\n", backupPath)
	fmt.Printf("last_backup_at updated.\n")
}

func cmdValidate(ctx context.Context, b *brain.Brain) {
	r, err := b.Validate(ctx)
	if err != nil {
		fatalf("validate: %v\n", err)
	}
	fmt.Println("Brain validation (read-only)")
	fmt.Printf("  schema_version:  %s\n", r.SchemaVer)
	fmt.Printf("  created_at:      %s\n", r.CreatedAt)
	fmt.Println("  Table row counts:")
	for _, t := range []string{"memories", "contacts", "interactions", "consolidation_log", "brain_meta", "schema_migrations", "episodic_archive"} {
		if n, ok := r.TableCounts[t]; ok {
			fmt.Printf("    %-20s %d\n", t, n)
		}
	}
	if len(r.Issues) > 0 {
		fmt.Println("  Issues:")
		for _, s := range r.Issues {
			fmt.Printf("    - %s\n", s)
		}
		fmt.Printf("  Result: FAIL (%d issue(s))\n", len(r.Issues))
		return
	}
	fmt.Printf("  Result: OK\n")
}

func cmdMeta(ctx context.Context, b *brain.Brain, args []string) {
	if len(args) == 0 {
		meta, err := b.Meta(ctx)
		if err != nil {
			fatalf("meta: %v\n", err)
		}
		printKV("schema_version", meta.SchemaVersion)
		printKV("created_at", meta.CreatedAt)
		printKV("description", meta.Description)
		printKV("last_backup_at", meta.LastBackupAt)
		return
	}

	for _, arg := range args {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			fatalf("meta: expected key=value, got %q\n", arg)
		}
		if err := b.SetMeta(ctx, parts[0], parts[1]); err != nil {
			fatalf("set %s: %v\n", parts[0], err)
		}
		fmt.Printf("Set %s = %s\n", parts[0], parts[1])
	}
}

func cmdInstallService(args []string) {
	fs := flag.NewFlagSet("install-service", flag.ExitOnError)
	fs.Parse(args)

	if err := daemon.WriteDefaultConfig(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not create config: %v\n", err)
	}

	switch runtime.GOOS {
	case "darwin":
		installLaunchd()
	case "linux":
		installSystemd()
	default:
		fatalf("Unsupported OS: %s\nSupported: darwin (macOS), linux\n", runtime.GOOS)
	}
}

func cmdUninstallService(args []string) {
	fs := flag.NewFlagSet("uninstall-service", flag.ExitOnError)
	fs.Parse(args)

	switch runtime.GOOS {
	case "darwin":
		uninstallLaunchd()
	case "linux":
		uninstallSystemd()
	default:
		fatalf("Unsupported OS: %s\n", runtime.GOOS)
	}
}

func cmdDaemonStatus() {
	home, err := os.UserHomeDir()
	if err != nil {
		fatalf("cannot determine home directory: %v\n", err)
	}
	logPath := filepath.Join(home, ".brain", "daemon.log")

	fmt.Println("Daemon status")
	fmt.Println("-------------")
	switch runtime.GOOS {
	case "darwin":
		if isLaunchdServiceLoaded(launchdLabel) {
			out, _ := exec.CommandContext(context.Background(), "launchctl", "list", launchdLabel).Output()
			if pid := parseLaunchdListPID(string(out)); pid != "" {
				fmt.Printf("  running (pid %s)\n", pid)
			} else {
				fmt.Println("  running")
			}
		} else {
			fmt.Println("  not running")
		}
	case "linux":
		out, err := exec.CommandContext(context.Background(), "systemctl", "--user", "is-active", "brain-daemon").Output()
		if err != nil {
			fmt.Println("  not running")
		} else {
			fmt.Printf("  %s\n", strings.TrimSpace(string(out)))
		}
	default:
		fmt.Printf("  unsupported OS: %s\n", runtime.GOOS)
	}
	fmt.Printf("\nLog: %s (last 15 lines)\n", logPath)
	fmt.Println("-----")
	printLastLogLines(logPath, 15)
}

func cmdDaemonUpdate() {
	cfg := daemon.LoadConfig()
	if cfg.SourcePath == "" {
		fmt.Fprintf(os.Stderr, "daemon-update requires source_path in ~/.brain/config (or BRAIN_SOURCE_PATH).\n")
		fmt.Fprintf(os.Stderr, "Example: echo 'source_path = %s' >> ~/.brain/config\n", filepath.Join(os.Getenv("HOME"), "Desktop", "brAIn"))
		os.Exit(1)
	}
	pid := getDaemonPID()
	if pid == 0 {
		fmt.Fprintf(os.Stderr, "daemon not running. Start it with 'brain install-service' or run brain-daemon.\n")
		os.Exit(1)
	}
	if err := syscall.Kill(pid, syscall.SIGUSR2); err != nil {
		fatalf("send SIGUSR2 to daemon (pid %d): %v\n", pid, err)
	}
	fmt.Printf("Sent SIGUSR2 to daemon (pid %d). It will rebuild from %s and restart.\n", pid, cfg.SourcePath)
}

func cmdDaemonRestart() {
	cfg := daemon.LoadConfig()
	if cfg.SourcePath == "" {
		fmt.Fprintf(os.Stderr, "daemon-restart requires source_path in ~/.brain/config (or BRAIN_SOURCE_PATH).\n")
		fmt.Fprintf(os.Stderr, "Example: echo 'source_path = %s' >> ~/.brain/config\n", filepath.Join(os.Getenv("HOME"), "Desktop", "brAIn"))
		os.Exit(1)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fatalf("cannot determine home directory: %v\n", err)
	}
	daemonBin := filepath.Join(home, "bin", "brain-daemon")

	fmt.Printf("Rebuilding daemon from %s...\n", cfg.SourcePath)
	if err := daemon.BuildAndReplace(cfg.SourcePath, daemonBin); err != nil {
		fatalf("rebuild daemon: %v\n", err)
	}
	fmt.Printf("Rebuilt: %s\n", daemonBin)

	switch runtime.GOOS {
	case "darwin":
		if !isLaunchdServiceLoaded(launchdLabel) {
			fatalf("daemon service is not loaded. Run 'brain install-service' first.\n")
		}
		label := fmt.Sprintf("gui/%d/%s", os.Getuid(), launchdLabel)
		if err := exec.CommandContext(context.Background(), "launchctl", "kickstart", "-k", label).Run(); err != nil {
			fatalf("restart daemon service: %v\n", err)
		}
		fmt.Printf("Restarted launchd service: %s\n", label)
	case "linux":
		if err := exec.CommandContext(context.Background(), "systemctl", "--user", "restart", "brain-daemon").Run(); err != nil {
			fatalf("restart daemon service: %v\n", err)
		}
		fmt.Println("Restarted systemd service: brain-daemon")
	default:
		fatalf("unsupported OS: %s\n", runtime.GOOS)
	}
}

func getDaemonPID() int {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.CommandContext(context.Background(), "launchctl", "list", launchdLabel).Output()
		if err != nil {
			return 0
		}
		pidStr := parseLaunchdListPID(string(out))
		if pidStr == "" {
			return 0
		}
		pid, _ := strconv.Atoi(pidStr)
		return pid
	case "linux":
		out, err := exec.CommandContext(context.Background(), "systemctl", "--user", "show", "brain-daemon", "--property=MainPID", "--value").Output()
		if err != nil {
			return 0
		}
		pid, _ := strconv.Atoi(strings.TrimSpace(string(out)))
		return pid
	}
	return 0
}

// parseLaunchdListPID extracts PID from launchctl list output (macOS may print plist-style "PID" = 12345).
func parseLaunchdListPID(out string) string {
	const prefix = `"PID" = `
	i := strings.Index(out, prefix)
	if i < 0 {
		return ""
	}
	i += len(prefix)
	end := i
	for end < len(out) && out[end] >= '0' && out[end] <= '9' {
		end++
	}
	if end > i {
		return out[i:end]
	}
	return ""
}

func printLastLogLines(path string, n int) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("(no log file yet)")
		} else {
			fmt.Fprintf(os.Stderr, "read log: %v\n", err)
		}
		return
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "read log: %v\n", err)
		return
	}
	start := 0
	if len(lines) > n {
		start = len(lines) - n
	}
	for i := start; i < len(lines); i++ {
		fmt.Println(lines[i])
	}
}

const launchdLabel = "com.brain.daemon"

func launchdPlistPath() string {
	home, _ := os.UserHomeDir()
	return home + "/Library/LaunchAgents/" + launchdLabel + ".plist"
}

func installLaunchd() {
	home, _ := os.UserHomeDir()
	daemonBin := home + "/bin/brain-daemon"

	plist := strings.ReplaceAll(launchdTemplate, "{{DAEMON_BIN}}", daemonBin)

	plistPath := launchdPlistPath()
	if err := os.MkdirAll(home+"/Library/LaunchAgents", 0o755); err != nil {
		fatalf("create LaunchAgents dir: %v\n", err)
	}
	if err := os.WriteFile(plistPath, []byte(plist), 0o644); err != nil {
		fatalf("write plist: %v\n", err)
	}

	cfg := daemon.LoadConfig()
	fmt.Printf("Installed launchd service:\n")
	fmt.Printf("  plist:         %s\n", plistPath)
	fmt.Printf("  config:        %s\n", daemon.ConfigFilePath())
	fmt.Printf("  consolidation: every %s\n", cfg.ConsolidationInterval)
	fmt.Printf("  profiling:     every %s\n", cfg.ProfileInterval)
	fmt.Printf("  log:           %s\n", home+"/.brain/daemon.log")

	if isLaunchdServiceLoaded(launchdLabel) {
		runCmd("launchctl", "unload", plistPath)
	}
	runCmd("launchctl", "load", plistPath)
	fmt.Println("Daemon launched via launchd.")
	fmt.Println("It will also start automatically at each login. Logs: ~/.brain/daemon.log")
	fmt.Println()
	fmt.Println("Edit ~/.brain/config to change intervals, then restart the service.")
}

func uninstallLaunchd() {
	plistPath := launchdPlistPath()
	if _, err := os.Stat(plistPath); os.IsNotExist(err) {
		fmt.Println("No launchd service found.")
		return
	}
	fmt.Printf("Unloading and removing %s\n", plistPath)
	if isLaunchdServiceLoaded(launchdLabel) {
		runCmd("launchctl", "unload", plistPath)
	}
	if err := os.Remove(plistPath); err != nil {
		fatalf("remove plist: %v\n", err)
	}
	fmt.Println("Service removed.")
}

const launchdTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.brain.daemon</string>
	<key>ProgramArguments</key>
	<array>
		<string>{{DAEMON_BIN}}</string>
	</array>
	<key>EnvironmentVariables</key>
	<dict>
		<key>PATH</key>
		<string>/usr/local/bin:/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
	</dict>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
</dict>
</plist>
`

func systemdServicePath() string {
	home, _ := os.UserHomeDir()
	return home + "/.config/systemd/user/brain-daemon.service"
}

func installSystemd() {
	home, _ := os.UserHomeDir()
	daemonBin := home + "/bin/brain-daemon"

	unit := strings.ReplaceAll(systemdTemplate, "{{DAEMON_BIN}}", daemonBin)

	servicePath := systemdServicePath()
	dir := home + "/.config/systemd/user"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fatalf("create systemd user dir: %v\n", err)
	}
	if err := os.WriteFile(servicePath, []byte(unit), 0o644); err != nil {
		fatalf("write unit file: %v\n", err)
	}

	cfg := daemon.LoadConfig()
	fmt.Printf("Installed systemd user service:\n")
	fmt.Printf("  unit:          %s\n", servicePath)
	fmt.Printf("  config:        %s\n", daemon.ConfigFilePath())
	fmt.Printf("  consolidation: every %s\n", cfg.ConsolidationInterval)
	fmt.Printf("  profiling:     every %s\n", cfg.ProfileInterval)

	runCmd("systemctl", "--user", "daemon-reload")
	runCmd("systemctl", "--user", "enable", "--now", "brain-daemon")
	fmt.Println("Daemon launched via systemd.")
	fmt.Println("It will also start automatically at each login. Logs: ~/.brain/daemon.log")
	fmt.Println()
	fmt.Println("Edit ~/.brain/config to change intervals, then restart the service.")
}

func uninstallSystemd() {
	servicePath := systemdServicePath()
	if _, err := os.Stat(servicePath); os.IsNotExist(err) {
		fmt.Println("No systemd service found.")
		return
	}
	fmt.Printf("Stopping and removing %s\n", servicePath)
	runCmd("systemctl", "--user", "disable", "--now", "brain-daemon")
	if err := os.Remove(servicePath); err != nil {
		fatalf("remove unit file: %v\n", err)
	}
	runCmd("systemctl", "--user", "daemon-reload")
	fmt.Println("Service removed.")
}

const systemdTemplate = `[Unit]
Description=brAIn memory daemon - background profile analysis and consolidation
After=network.target

[Service]
Type=simple
ExecStart={{DAEMON_BIN}}
Restart=on-failure
RestartSec=30

[Install]
WantedBy=default.target
`

func isLaunchdServiceLoaded(label string) bool {
	cmd := exec.CommandContext(context.Background(), "launchctl", "list", label)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}

func runCmd(name string, args ...string) {
	c := exec.CommandContext(context.Background(), name, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Run()
}
