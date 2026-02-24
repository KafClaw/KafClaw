package cli

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/KafClaw/KafClaw/internal/agent"
	"github.com/KafClaw/KafClaw/internal/bus"
	"github.com/KafClaw/KafClaw/internal/channels"
	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/group"
	"github.com/KafClaw/KafClaw/internal/identity"
	"github.com/KafClaw/KafClaw/internal/knowledge"
	"github.com/KafClaw/KafClaw/internal/memory"
	"github.com/KafClaw/KafClaw/internal/orchestrator"
	"github.com/KafClaw/KafClaw/internal/policy"
	"github.com/KafClaw/KafClaw/internal/provider"
	"github.com/KafClaw/KafClaw/internal/scheduler"
	"github.com/KafClaw/KafClaw/internal/timeline"
	"github.com/KafClaw/KafClaw/internal/tools"
	"github.com/spf13/cobra"
)

func newTraceID() string {
	var b [8]byte
	if _, err := io.ReadFull(rand.Reader, b[:]); err == nil {
		return hex.EncodeToString(b[:])
	}
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

var gatewayCmd = &cobra.Command{
	Use:   "gateway",
	Short: "Start the agent gateway (WhatsApp, etc)",
	Run:   runGateway,
}

var gatewaySignalNotify = signal.Notify
var gatewaySignalStop = signal.Stop

func runGateway(cmd *cobra.Command, args []string) {
	runGatewayMain(cmd, args)
}

func runGatewayMain(cmd *cobra.Command, args []string) {
	printHeader("🌐 KafClaw Gateway")
	fmt.Println("Starting KafClaw Gateway...")

	// 1. Load Config
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Config error: %v\n", err)
		os.Exit(1)
	}
	if err := validateEmbeddingHardGate(cfg); err != nil {
		fmt.Printf("Memory embedding gate failed: %v\n", err)
		os.Exit(1)
	}
	// 2. Setup Timeline (QMD)
	home, _ := os.UserHomeDir()
	timelinePath := fmt.Sprintf("%s/.kafclaw/timeline.db", home)
	timeSvc, err := timeline.NewTimelineService(timelinePath)
	if err != nil {
		fmt.Printf("Failed to init timeline: %v\n", err)
		os.Exit(1)
	}

	// Seed default settings if missing
	seedSetting := func(key, value string) {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			return
		}
		if v, err := timeSvc.GetSetting(key); err == nil && strings.TrimSpace(v) != "" {
			return
		}
		_ = timeSvc.SetSetting(key, value)
	}
	seedSetting("bot_repo_path", "/Users/kamir/GITHUB.kamir/KafClaw/kafclaw")
	seedSetting("default_work_repo_path", filepath.Join(home, "KafClaw-Workspace"))
	seedSetting("default_repo_search_path", home)
	seedSetting("kafscale_lfs_proxy_url", "http://localhost:8080")
	if err := reconcileDurableRuntimeState(timeSvc); err != nil {
		fmt.Printf("⚠️ Runtime reconciliation failed: %v\n", err)
	}

	// Resolve work repo path (settings override config)
	workRepoPath := cfg.Paths.WorkRepoPath
	if v, err := timeSvc.GetSetting("work_repo_path"); err == nil && strings.TrimSpace(v) != "" {
		workRepoPath = strings.TrimSpace(v)
	}
	if warn, err := config.EnsureWorkRepo(workRepoPath); err != nil {
		fmt.Printf("Work repo error: %v\n", err)
	} else if warn != "" {
		fmt.Printf("Work repo warning: %s\n", warn)
	}
	var workRepoMu sync.RWMutex
	getWorkRepo := func() string {
		workRepoMu.RLock()
		defer workRepoMu.RUnlock()
		return workRepoPath
	}
	// Resolve system repo path (settings override config)
	systemRepoPath := cfg.Paths.SystemRepoPath
	if v, err := timeSvc.GetSetting("bot_repo_path"); err == nil && strings.TrimSpace(v) != "" {
		systemRepoPath = strings.TrimSpace(v)
	}

	// Helper: resolve repo from query param (?repo=identity → systemRepoPath, else work repo)
	resolveRepo := func(r *http.Request) string {
		if r.URL.Query().Get("repo") == "identity" {
			return systemRepoPath
		}
		return getWorkRepo()
	}

	// 3. Setup Bus
	msgBus := bus.NewMessageBus()

	// 4. Setup Providers
	prov, provErr := provider.Resolve(cfg, "main")
	if provErr != nil {
		fmt.Printf("Provider error: %v\n", provErr)
		os.Exit(1)
	}

	if cfg.Providers.LocalWhisper.Enabled {
		if oaProv, ok := prov.(*provider.OpenAIProvider); ok {
			prov = provider.NewLocalWhisperProvider(cfg.Providers.LocalWhisper, oaProv)
		}
	}

	// 4b. Setup Policy Engine
	policyEngine := policy.NewDefaultEngine()
	// Allow Tier 2 (shell) by default for the personal bot — the shell tool
	// already has its own deny-pattern and allow-list safety layer.
	policyEngine.MaxAutoTier = 2
	// External users (non-owner) are restricted to read-only tools (tier 0).
	policyEngine.ExternalMaxTier = 0

	// 4c. Setup Memory System (uses dedicated embedding resolver, independent from chat provider)
	var memorySvc *memory.MemoryService
	if embedder, source := resolveMemoryEmbedder(cfg, prov); embedder != nil {
		vecStore := memory.NewSQLiteVecStore(timeSvc.DB(), 1536)
		memorySvc = memory.NewMemoryService(vecStore, embedder)
		fmt.Println("🧠 Memory system initialized:", source)
	} else {
		fmt.Println("ℹ️  Memory system disabled (no embedding provider available)")
	}

	// 4d. Setup Group Collaboration (conditional)
	grpState := &groupState{}

	// Helper: build a group manager from config + settings
	buildGrpManager := func(grpCfg config.GroupConfig) *group.Manager {
		if grpCfg.LFSProxyURL == "" || grpCfg.LFSProxyURL == "http://localhost:8080" {
			if url, err := timeSvc.GetSetting("kafscale_lfs_proxy_url"); err == nil && url != "" {
				grpCfg.LFSProxyURL = url
			}
		}
		registry := tools.NewRegistry()
		ctxBuilder := agent.NewContextBuilder(cfg.Paths.Workspace, workRepoPath, systemRepoPath, registry)
		agentID := grpCfg.AgentID
		if agentID == "" {
			hostname, _ := os.Hostname()
			agentID = fmt.Sprintf("kafclaw-%s", hostname)
		}
		identity := ctxBuilder.BuildIdentityEnvelope(agentID, "KafClaw", cfg.Model.Name)
		// Enrich channels based on actual config.
		if cfg.Channels.WhatsApp.Enabled {
			identity.Channels = append(identity.Channels, "whatsapp")
		}
		if cfg.Channels.Telegram.Enabled {
			identity.Channels = append(identity.Channels, "telegram")
		}
		if cfg.Channels.Discord.Enabled {
			identity.Channels = append(identity.Channels, "discord")
		}
		if cfg.Channels.Slack.Enabled {
			identity.Channels = append(identity.Channels, "slack")
		}
		if cfg.Channels.MSTeams.Enabled {
			identity.Channels = append(identity.Channels, "msteams")
		}
		mgr := group.NewManager(grpCfg, timeSvc, identity)
		// Bridge group memory items into local vector store for RAG
		if memorySvc != nil {
			mgr.SetMemoryIndexer(memorySvc)
		}
		return mgr
	}

	// Helper: start Kafka consumer + router, returns cancel func
	startGrpKafka := func(grpCfg config.GroupConfig, mgr *group.Manager, parentCtx context.Context, orchHandler group.OrchestratorHandler) context.CancelFunc {
		if grpCfg.KafkaBrokers == "" {
			return func() {}
		}
		kafkaCtx, kafkaCancel := context.WithCancel(parentCtx)
		extTopics := group.ExtendedTopics(grpCfg.GroupName)
		knowledgeTopics := collectKnowledgeTopics(cfg)
		allTopics := extTopics.AllTopics()
		allTopics = append(allTopics, knowledgeTopics...)
		consumerGroup := grpCfg.ConsumerGroup
		if consumerGroup == "" {
			consumerGroup = grpCfg.AgentID
			if consumerGroup == "" {
				hostname, _ := os.Hostname()
				consumerGroup = fmt.Sprintf("kafclaw-%s", hostname)
			}
		}
		dialer, err := group.BuildKafkaDialerFromGroupConfig(grpCfg)
		if err != nil {
			fmt.Println("⚠️ Kafka consumer config error: invalid or incomplete Kafka security settings.")
			fmt.Println("⚠️ Group router not started until Kafka security settings are fixed.")
			kafkaCancel()
			return func() {}
		}
		kafkaConsumer := group.NewKafkaConsumerWithDialer(
			grpCfg.KafkaBrokers,
			consumerGroup,
			allTopics,
			dialer,
		)
		grpState.SetConsumer(kafkaConsumer)
		router := group.NewGroupRouter(mgr, msgBus, kafkaConsumer)
		if orchHandler != nil {
			router.SetOrchestratorHandler(orchHandler)
		}
		if cfg.Knowledge.Enabled && len(knowledgeTopics) > 0 {
			router.SetKnowledgeHandler(group.NewKnowledgeHandler(timeSvc, cfg.Node.ClawID, cfg.Knowledge.GovernanceEnabled), knowledgeTopics)
			fmt.Printf("🧠 Knowledge router enabled (%d topic(s))\n", len(knowledgeTopics))
		}
		go func() {
			if err := router.Run(kafkaCtx); err != nil {
				fmt.Printf("⚠️ Group router stopped: %v\n", err)
			}
		}()
		fmt.Println("📡 Kafka consumer started for group topics")
		return kafkaCancel
	}

	if cfg.Group.Enabled && cfg.Group.GroupName != "" {
		mgr := buildGrpManager(cfg.Group)
		grpState.SetManager(mgr, nil)
		fmt.Println("🤝 Group collaboration enabled:", cfg.Group.GroupName)
	} else if cfg.Orchestrator.Enabled || cfg.Gateway.Host == "0.0.0.0" {
		// Non-standalone: allow auto-rejoin from DB
		if active, err := timeSvc.GetSetting("group_active"); err == nil && active == "true" {
			if gn, err := timeSvc.GetSetting("group_name"); err == nil && gn != "" {
				cfg.Group.GroupName = gn
				cfg.Group.Enabled = true
				mgr := buildGrpManager(cfg.Group)
				grpState.SetManager(mgr, nil)
				fmt.Println("🤝 Group collaboration restored from settings:", cfg.Group.GroupName)
			}
		}
	} else {
		fmt.Println("🖥️  Standalone Desktop mode: skipping group auto-rejoin")
	}

	// --- Shared mode variable (used by all handlers) ---
	var modeMu sync.RWMutex
	currentMode := "standalone"

	getMode := func() string {
		modeMu.RLock()
		defer modeMu.RUnlock()
		return currentMode
	}

	recalcMode := func() {
		modeMu.Lock()
		defer modeMu.Unlock()
		currentMode = "standalone"
		if cfg.Group.Enabled && cfg.Orchestrator.Enabled {
			currentMode = "full"
		} else if cfg.Group.Enabled {
			currentMode = "group"
		}
		if cfg.Gateway.Host == "0.0.0.0" {
			currentMode = "headless"
		}
	}
	recalcMode()

	// Audit: log mode at startup
	if getMode() == "standalone" {
		_ = timeSvc.AddEvent(&timeline.TimelineEvent{
			EventID:        fmt.Sprintf("MODE_STANDALONE_%d", time.Now().UnixNano()),
			Timestamp:      time.Now(),
			SenderID:       "system",
			SenderName:     "KafClaw",
			EventType:      "SYSTEM",
			ContentText:    "Entered Standalone Desktop mode",
			Classification: "MODE_CHANGE",
			Authorized:     true,
			Metadata:       `{"mode":"standalone","event":"ENTER_STANDALONE"}`,
		})
		_ = timeSvc.SetSetting("current_mode", "standalone")
		fmt.Println("🖥️  Standalone Desktop mode active — group features disabled")
	}

	// Helper: block mutating group endpoints in standalone mode
	isStandaloneBlocked := func(w http.ResponseWriter) bool {
		if getMode() == "standalone" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "group operations are disabled in standalone mode",
			})
			return true
		}
		return false
	}

	// Build group publisher for the loop (nil-safe)
	var groupPublisher agent.GroupTracePublisher
	if grpState.Manager() != nil {
		groupPublisher = &groupTraceAdapter{mgr: grpState.Manager()}
	}

	// 4e. Setup Orchestrator (conditional)
	var orch *orchestrator.Orchestrator
	if cfg.Orchestrator.Enabled && grpState.Manager() != nil {
		orch = orchestrator.New(cfg.Orchestrator, grpState.Manager(), timeSvc)
		fmt.Println("🎯 Orchestrator enabled:", cfg.Orchestrator.Role)
	}

	gatewayStartTime := time.Now()

	// 5. Setup Auto-Indexer (background memory indexing)
	var autoIndexer *memory.AutoIndexer
	if memorySvc != nil {
		autoIndexer = memory.NewAutoIndexer(memorySvc, memory.AutoIndexerConfig{
			MinLength:     100,
			BatchSize:     5,
			FlushInterval: 30 * time.Second,
		})
		fmt.Println("📝 Auto-indexer initialized")
	}

	// 5a. Setup Expertise Tracker
	expertiseTracker := memory.NewExpertiseTracker(timeSvc.DB())
	fmt.Println("🎯 Expertise tracker initialized")

	// 5a-ii. Setup Working Memory Store
	workingMemoryStore := memory.NewWorkingMemoryStore(timeSvc.DB())
	fmt.Println("📋 Working memory store initialized")

	// 5a-iii. Setup Observer (observational memory)
	var observer *memory.Observer
	if cfg.Observer.Enabled {
		observer = memory.NewObserver(memory.ObserverConfig{
			Enabled:          true,
			Model:            cfg.Observer.Model,
			MessageThreshold: cfg.Observer.MessageThreshold,
			MaxObservations:  cfg.Observer.MaxObservations,
		}, prov, timeSvc.DB())
		if observer != nil {
			fmt.Println("👁️  Observer initialized")
		}
	}

	// 5a-iv. Setup ER1 Client (personal memory sync)
	var er1Client *memory.ER1Client
	if cfg.ER1.URL != "" && memorySvc != nil {
		er1Client = memory.NewER1Client(memory.ER1Config{
			URL:          cfg.ER1.URL,
			APIKey:       cfg.ER1.APIKey,
			UserID:       cfg.ER1.UserID,
			SyncInterval: cfg.ER1.SyncInterval,
		}, memorySvc)
		if er1Client != nil {
			fmt.Println("🔗 ER1 client initialized")
		}
	}

	// 5a-v. Auto-scaffold workspace if soul files are missing (for headless/Docker agents)
	if cfg.Paths.Workspace != "" {
		hasSoulFiles := true
		for _, name := range identity.TemplateNames {
			if _, err := os.Stat(filepath.Join(cfg.Paths.Workspace, name)); err != nil {
				hasSoulFiles = false
				break
			}
		}
		if !hasSoulFiles {
			result, err := identity.ScaffoldWorkspace(cfg.Paths.Workspace, false)
			if err != nil {
				fmt.Printf("⚠️ Workspace scaffold error: %v\n", err)
			} else if len(result.Created) > 0 {
				fmt.Printf("📂 Auto-scaffolded workspace: created %v\n", result.Created)
			}
		}
	}

	// 5b. Setup Loop
	loop := agent.NewLoop(agent.LoopOptions{
		Bus:                     msgBus,
		Provider:                prov,
		Timeline:                timeSvc,
		Policy:                  policyEngine,
		MemoryService:           memorySvc,
		AutoIndexer:             autoIndexer,
		ExpertiseTracker:        expertiseTracker,
		WorkingMemory:           workingMemoryStore,
		Observer:                observer,
		GroupPublisher:          groupPublisher,
		Workspace:               cfg.Paths.Workspace,
		WorkRepo:                workRepoPath,
		SystemRepo:              systemRepoPath,
		WorkRepoGetter:          getWorkRepo,
		Model:                   cfg.Model.Name,
		MaxIterations:           cfg.Model.MaxToolIterations,
		MaxSubagentSpawnDepth:   cfg.Tools.Subagents.MaxSpawnDepth,
		MaxSubagentChildren:     cfg.Tools.Subagents.MaxChildrenPerAgent,
		MaxSubagentConcurrent:   cfg.Tools.Subagents.MaxConcurrent,
		SubagentArchiveAfter:    cfg.Tools.Subagents.ArchiveAfterMinutes,
		AgentID:                 cfg.Group.AgentID,
		SubagentAllowAgents:     cfg.Tools.Subagents.AllowAgents,
		SubagentModel:           cfg.Tools.Subagents.Model,
		SubagentThinking:        cfg.Tools.Subagents.Thinking,
		SubagentMemoryShareMode: cfg.Tools.Subagents.MemoryShareMode,
		SubagentToolsAllow:      cfg.Tools.Subagents.Tools.Allow,
		SubagentToolsDeny:       cfg.Tools.Subagents.Tools.Deny,
		Config:                  cfg,
	})

	// 5b. Index soul files (non-blocking background)
	if memorySvc != nil {
		go func() {
			indexer := memory.NewSoulFileIndexer(memorySvc, cfg.Paths.Workspace)
			if err := indexer.IndexAll(context.Background()); err != nil {
				fmt.Printf("⚠️ Soul file indexing error: %v\n", err)
			}
		}()
	}

	// 6. Setup Channels
	// WhatsApp
	wa := channels.NewWhatsAppChannel(cfg.Channels.WhatsApp, msgBus, prov, timeSvc)
	slack := channels.NewSlackChannel(cfg.Channels.Slack, msgBus, timeSvc)
	msteams := channels.NewMSTeamsChannel(cfg.Channels.MSTeams, msgBus, timeSvc)

	// 7. Start Everything
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	gatewaySignalNotify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer gatewaySignalStop(sigChan)

	// Start Auto-Indexer
	if autoIndexer != nil {
		go autoIndexer.Run(ctx)
	}

	// Start ER1 Sync Loop
	if er1Client != nil {
		go er1Client.SyncLoop(ctx)
	}

	// Start Memory Lifecycle Manager (daily pruning)
	lifecycleMgr := memory.NewLifecycleManager(timeSvc.DB(), memory.LifecycleConfig{})
	go func() {
		// Run once at startup
		lifecycleMgr.RunDaily()
		// Then daily
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				lifecycleMgr.RunDaily()
			}
		}
	}()

	// Start Channels
	if err := wa.Start(ctx); err != nil {
		fmt.Printf("Failed to start WhatsApp: %v\n", err)
	}
	if err := slack.Start(ctx); err != nil {
		fmt.Printf("Failed to start Slack: %v\n", err)
	}
	if err := msteams.Start(ctx); err != nil {
		fmt.Printf("Failed to start MSTeams: %v\n", err)
	}

	// Route web UI outbound to WhatsApp and timeline
	msgBus.Subscribe("webui", func(msg *bus.OutboundMessage) {
		go func() {
			webUserID, err := strconv.ParseInt(msg.ChatID, 10, 64)
			if err != nil {
				fmt.Printf("⚠️ webui outbound invalid web_user_id: %s\n", msg.ChatID)
				return
			}
			jid, ok, err := timeSvc.GetWebLink(webUserID)
			if err != nil {
				fmt.Printf("⚠️ webui outbound link lookup error: %v\n", err)
			}
			status := "no_link"
			jid = strings.TrimSpace(jid)
			if ok && jid != "" {
				jid = normalizeWhatsAppJID(jid)
				status = "queued"
			} else {
				fmt.Printf("⚠️ webui outbound no WhatsApp link for web_user_id=%d\n", webUserID)
			}

			// Check silent mode and optional override
			forceSend := true
			if user, err := timeSvc.GetWebUser(webUserID); err == nil {
				forceSend = user.ForceSend
			}
			if status != "no_link" && timeSvc.IsSilentMode() && !forceSend {
				fmt.Printf("🔇 webui outbound suppressed (silent mode) to %s web_user_id=%d\n", jid, webUserID)
				status = "suppressed"
			} else if status != "no_link" {
				// Send via WhatsApp channel; bypass silent when forceSend is enabled
				if timeSvc.IsSilentMode() && forceSend {
					sendCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()
					if err := wa.Send(sendCtx, &bus.OutboundMessage{
						Channel: "whatsapp",
						ChatID:  jid,
						Content: msg.Content,
					}); err != nil {
						fmt.Printf("⚠️ webui outbound direct send error: %v\n", err)
						status = "error"
					} else {
						status = "sent"
					}
				} else {
					msgBus.PublishOutbound(&bus.OutboundMessage{
						Channel: "whatsapp",
						ChatID:  jid,
						Content: msg.Content,
					})
					status = "queued"
				}
			}

			// Log outbound to timeline for Web UI visibility (always)
			_ = timeSvc.AddEvent(&timeline.TimelineEvent{
				EventID:        fmt.Sprintf("WEBUI_ACK_%d", time.Now().UnixNano()),
				Timestamp:      time.Now(),
				SenderID:       "AGENT",
				SenderName:     "Agent",
				EventType:      "SYSTEM",
				ContentText:    msg.Content,
				Classification: fmt.Sprintf("WEBUI_OUTBOUND->%s force=%v status=%s", jid, forceSend, status),
				Authorized:     true,
			})
			fmt.Printf("📤 WebUI outbound status=%s to=%s\n", status, jid)
		}()
	})

	// Start Delivery Worker
	deliveryWorker := agent.NewDeliveryWorker(timeSvc, msgBus)
	go deliveryWorker.Run(ctx)

	// Start Scheduler (conditional)
	if cfg.Scheduler.Enabled {
		schedCfg := scheduler.Config{
			Enabled:        true,
			TickInterval:   cfg.Scheduler.TickInterval,
			MaxConcLLM:     cfg.Scheduler.MaxConcLLM,
			MaxConcShell:   cfg.Scheduler.MaxConcShell,
			MaxConcDefault: cfg.Scheduler.MaxConcDefault,
		}
		sched := scheduler.New(schedCfg, msgBus, timeSvc)
		go sched.Run(ctx)
		fmt.Println("Scheduler started")
	}

	// Start Bus Dispatcher
	go msgBus.DispatchOutbound(ctx)

	// Start Local HTTP Server for Local Network access
	// Start Local HTTP Server for Local Network access (API)
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/chat", func(w http.ResponseWriter, r *http.Request) {
			if cfg.Gateway.AuthToken != "" {
				token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
				if token != cfg.Gateway.AuthToken {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
			}
			if r.Method != http.MethodPost {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			msg := r.URL.Query().Get("message")
			if msg == "" {
				http.Error(w, "Missing message parameter", http.StatusBadRequest)
				return
			}

			session := r.URL.Query().Get("session")
			if session == "" {
				session = "local:default"
			}

			fmt.Printf("🌐 Local Network Request: %s\n", msg)
			traceID := newTraceID()
			inMeta, _ := json.Marshal(map[string]any{
				"channel":      "local",
				"sender":       session,
				"message_type": "TEXT",
				"content":      msg,
			})
			_ = timeSvc.AddEvent(&timeline.TimelineEvent{
				EventID:        fmt.Sprintf("LOCAL_IN_%d", time.Now().UnixNano()),
				TraceID:        traceID,
				Timestamp:      time.Now(),
				SenderID:       session,
				SenderName:     "Local",
				EventType:      "TEXT",
				ContentText:    msg,
				Classification: "LOCAL_INBOUND",
				Authorized:     true,
				Metadata:       string(inMeta),
			})
			resp, err := loop.ProcessDirectWithTrace(ctx, msg, session, traceID)
			if err != nil {
				outErrMeta, _ := json.Marshal(map[string]any{
					"response_text":   err.Error(),
					"delivery_status": "error",
				})
				_ = timeSvc.AddEvent(&timeline.TimelineEvent{
					EventID:        fmt.Sprintf("LOCAL_OUT_%d", time.Now().UnixNano()),
					TraceID:        traceID,
					Timestamp:      time.Now(),
					SenderID:       "AGENT",
					SenderName:     "Agent",
					EventType:      "SYSTEM",
					ContentText:    err.Error(),
					Classification: "LOCAL_OUTBOUND status=error",
					Authorized:     true,
					Metadata:       string(outErrMeta),
				})
				fmt.Printf("📤 Local outbound status=error session=%s\n", session)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			outMeta, _ := json.Marshal(map[string]any{
				"response_text":   resp,
				"delivery_status": "sent",
			})
			_ = timeSvc.AddEvent(&timeline.TimelineEvent{
				EventID:        fmt.Sprintf("LOCAL_OUT_%d", time.Now().UnixNano()),
				TraceID:        traceID,
				Timestamp:      time.Now(),
				SenderID:       "AGENT",
				SenderName:     "Agent",
				EventType:      "SYSTEM",
				ContentText:    resp,
				Classification: "LOCAL_OUTBOUND status=sent",
				Authorized:     true,
				Metadata:       string(outMeta),
			})
			fmt.Printf("📤 Local outbound status=sent session=%s\n", session)
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			fmt.Fprint(w, resp)
		})

		addr := fmt.Sprintf("%s:%d", cfg.Gateway.Host, cfg.Gateway.Port)
		fmt.Printf("📡 API Server listening on http://%s\n", addr)
		if err := http.ListenAndServe(addr, mux); err != nil {
			fmt.Printf("API Server Error: %v\n", err)
		}
	}()

	// Start Dashboard Server
	go func() {
		mux := http.NewServeMux()

		// API: Status (unauthenticated health check)
		mux.HandleFunc("/api/v1/status", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Type", "application/json")

			agentID := cfg.Group.AgentID
			if agentID == "" {
				hostname, _ := os.Hostname()
				agentID = fmt.Sprintf("kafclaw-%s", hostname)
			}

			mode := getMode()

			orchEnabled := false
			if orch != nil {
				orchEnabled = true
			}

			json.NewEncoder(w).Encode(map[string]any{
				"version":              version,
				"mode":                 mode,
				"agent_id":             agentID,
				"uptime_seconds":       int(time.Since(gatewayStartTime).Seconds()),
				"group_enabled":        cfg.Group.Enabled,
				"orchestrator_enabled": orchEnabled,
			})
		})

		// API: Auth Verify (POST)
		mux.HandleFunc("/api/v1/auth/verify", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Type", "application/json")
			if cfg.Gateway.AuthToken == "" {
				json.NewEncoder(w).Encode(map[string]any{"valid": true, "auth_required": false})
				return
			}
			token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			valid := token == cfg.Gateway.AuthToken
			json.NewEncoder(w).Encode(map[string]any{"valid": valid, "auth_required": true})
		})

		type channelInboundRequest struct {
			AccountID      string `json:"account_id"`
			SenderID       string `json:"sender_id"`
			ChatID         string `json:"chat_id"`
			ThreadID       string `json:"thread_id"`
			MessageID      string `json:"message_id"`
			Text           string `json:"text"`
			IsGroup        bool   `json:"is_group"`
			WasMentioned   bool   `json:"was_mentioned"`
			GroupID        string `json:"group_id"`
			ChannelID      string `json:"channel_id"`
			HistoryLimit   int    `json:"history_limit"`
			DMHistoryLimit int    `json:"dm_history_limit"`
		}

		verifyChannelToken := func(r *http.Request, expected string) bool {
			expected = strings.TrimSpace(expected)
			if expected == "" {
				return true
			}
			h := strings.TrimSpace(r.Header.Get("X-Channel-Token"))
			if h == "" {
				h = strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
			}
			return h == expected
		}

		resolveSlackInboundToken := func(accountID string) string {
			id := strings.TrimSpace(strings.ToLower(accountID))
			if id == "" || id == "default" {
				return cfg.Channels.Slack.InboundToken
			}
			for _, acct := range cfg.Channels.Slack.Accounts {
				if strings.EqualFold(strings.TrimSpace(acct.ID), id) {
					if strings.TrimSpace(acct.InboundToken) != "" {
						return acct.InboundToken
					}
					return cfg.Channels.Slack.InboundToken
				}
			}
			return cfg.Channels.Slack.InboundToken
		}

		resolveMSTeamsInboundToken := func(accountID string) string {
			id := strings.TrimSpace(strings.ToLower(accountID))
			if id == "" || id == "default" {
				return cfg.Channels.MSTeams.InboundToken
			}
			for _, acct := range cfg.Channels.MSTeams.Accounts {
				if strings.EqualFold(strings.TrimSpace(acct.ID), id) {
					if strings.TrimSpace(acct.InboundToken) != "" {
						return acct.InboundToken
					}
					return cfg.Channels.MSTeams.InboundToken
				}
			}
			return cfg.Channels.MSTeams.InboundToken
		}

		// API: Slack inbound bridge (POST)
		mux.HandleFunc("/api/v1/channels/slack/inbound", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Channel-Token")
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "OPTIONS" {
				return
			}
			if r.Method != "POST" {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var body channelInboundRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
			if !verifyChannelToken(r, resolveSlackInboundToken(body.AccountID)) {
				http.Error(w, "invalid channel token", http.StatusUnauthorized)
				return
			}
			if strings.TrimSpace(body.SenderID) == "" || strings.TrimSpace(body.ChatID) == "" {
				http.Error(w, "sender_id and chat_id required", http.StatusBadRequest)
				return
			}
			if err := slack.HandleInboundWithAccountAndHints(
				body.AccountID,
				body.SenderID,
				body.ChatID,
				body.ThreadID,
				body.MessageID,
				body.Text,
				body.IsGroup,
				body.WasMentioned,
				body.HistoryLimit,
				body.DMHistoryLimit,
			); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"ok": true})
		})

		// API: MSTeams inbound bridge (POST)
		mux.HandleFunc("/api/v1/channels/msteams/inbound", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Channel-Token")
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "OPTIONS" {
				return
			}
			if r.Method != "POST" {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var body channelInboundRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
			if !verifyChannelToken(r, resolveMSTeamsInboundToken(body.AccountID)) {
				http.Error(w, "invalid channel token", http.StatusUnauthorized)
				return
			}
			if strings.TrimSpace(body.SenderID) == "" || strings.TrimSpace(body.ChatID) == "" {
				http.Error(w, "sender_id and chat_id required", http.StatusBadRequest)
				return
			}
			if err := msteams.HandleInboundWithContextAndHints(
				body.AccountID,
				body.SenderID,
				body.ChatID,
				body.ThreadID,
				body.MessageID,
				body.Text,
				body.IsGroup,
				body.WasMentioned,
				body.GroupID,
				body.ChannelID,
				body.HistoryLimit,
				body.DMHistoryLimit,
			); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"ok": true})
		})

		// Orchestrator API endpoints
		mux.HandleFunc("/api/v1/orchestrator/status", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Type", "application/json")
			if orch == nil {
				json.NewEncoder(w).Encode(map[string]any{"enabled": false})
				return
			}
			json.NewEncoder(w).Encode(orch.Status())
		})

		mux.HandleFunc("/api/v1/orchestrator/hierarchy", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Type", "application/json")
			if orch == nil {
				json.NewEncoder(w).Encode([]any{})
				return
			}
			json.NewEncoder(w).Encode(orch.GetHierarchy())
		})

		mux.HandleFunc("/api/v1/orchestrator/zones", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Type", "application/json")
			if orch == nil {
				json.NewEncoder(w).Encode([]any{})
				return
			}
			json.NewEncoder(w).Encode(orch.GetZones())
		})

		mux.HandleFunc("/api/v1/orchestrator/agents", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Type", "application/json")
			if orch == nil {
				json.NewEncoder(w).Encode([]any{})
				return
			}
			json.NewEncoder(w).Encode(orch.GetAgents())
		})

		mux.HandleFunc("/api/v1/orchestrator/dispatch", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "OPTIONS" {
				return
			}
			if r.Method != "POST" {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if orch == nil {
				http.Error(w, "orchestrator not enabled", http.StatusBadRequest)
				return
			}
			var body struct {
				Description string `json:"description"`
				TargetZone  string `json:"target_zone"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
			taskID := newTraceID()
			if err := orch.DispatchTask(ctx, taskID, body.Description, body.TargetZone); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"status": "dispatched", "task_id": taskID})
		})

		// API: Timeline
		mux.HandleFunc("/api/v1/timeline", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Type", "application/json")

			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			if limit == 0 {
				limit = 100
			}
			offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
			sender := r.URL.Query().Get("sender")
			traceID := r.URL.Query().Get("trace_id")

			events, err := timeSvc.GetEvents(timeline.FilterArgs{
				Limit:    limit,
				Offset:   offset,
				SenderID: sender,
				TraceID:  traceID,
			})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			json.NewEncoder(w).Encode(events)
		})

		// API: Trace (GET)
		mux.HandleFunc("/api/v1/trace/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Type", "application/json")

			traceID := strings.TrimPrefix(r.URL.Path, "/api/v1/trace/")
			traceID = strings.TrimSpace(traceID)
			if traceID == "" {
				http.Error(w, "trace_id required", http.StatusBadRequest)
				return
			}

			events, err := timeSvc.GetEvents(timeline.FilterArgs{
				Limit:   500,
				TraceID: traceID,
			})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			type span struct {
				ID       string         `json:"id"`
				Type     string         `json:"type"`
				Title    string         `json:"title"`
				Time     string         `json:"time"`
				Duration string         `json:"duration"`
				Output   string         `json:"output"`
				Metadata map[string]any `json:"metadata,omitempty"`
			}

			spans := make([]span, 0, len(events))
			for _, e := range events {
				spanType := "EVENT"
				switch {
				case strings.Contains(e.Classification, "INBOUND") || e.SenderName == "User":
					spanType = "INBOUND"
				case strings.Contains(e.Classification, "OUTBOUND") || e.SenderName == "Agent":
					spanType = "OUTBOUND"
				case strings.Contains(e.Classification, "LLM"):
					spanType = "LLM"
				case strings.Contains(e.Classification, "TOOL"):
					spanType = "TOOL"
				}

				// Parse metadata JSON if present
				var meta map[string]any
				if e.Metadata != "" {
					_ = json.Unmarshal([]byte(e.Metadata), &meta)
				}

				// Extract duration from metadata
				dur := ""
				if meta != nil {
					if ms, ok := meta["duration_ms"]; ok {
						switch v := ms.(type) {
						case float64:
							dur = fmt.Sprintf("%dms", int64(v))
						}
					}
				}

				// Build output preview
				output := ""
				switch spanType {
				case "INBOUND", "OUTBOUND":
					output = e.ContentText
					// Add basic metadata for INBOUND/OUTBOUND if not already present
					if meta == nil {
						meta = map[string]any{}
					}
					if spanType == "INBOUND" {
						meta["channel"] = e.SenderID
						meta["sender"] = e.SenderName
						meta["message_type"] = e.EventType
						meta["content"] = e.ContentText
					} else {
						meta["response_text"] = e.ContentText
					}
				case "LLM":
					if meta != nil {
						if rt, ok := meta["response_text"].(string); ok && rt != "" {
							if len(rt) > 200 {
								output = rt[:200] + "..."
							} else {
								output = rt
							}
						}
					}
				case "TOOL":
					if meta != nil {
						if tn, ok := meta["tool_name"].(string); ok {
							output = tn
						}
					}
				}

				spans = append(spans, span{
					ID:       e.EventID,
					Type:     spanType,
					Title:    e.Classification,
					Time:     e.Timestamp.Format("15:04:05"),
					Duration: dur,
					Output:   output,
					Metadata: meta,
				})
			}

			// Also fetch task + policy decisions for this trace
			var taskInfo map[string]any
			if task, err := timeSvc.GetTaskByTraceID(traceID); err == nil && task != nil {
				taskInfo = map[string]any{
					"task_id":           task.TaskID,
					"status":            task.Status,
					"delivery_status":   task.DeliveryStatus,
					"prompt_tokens":     task.PromptTokens,
					"completion_tokens": task.CompletionTokens,
					"total_tokens":      task.TotalTokens,
					"channel":           task.Channel,
					"created_at":        task.CreatedAt,
					"completed_at":      task.CompletedAt,
				}
			}

			var policyDecisions []map[string]any
			if decisions, err := timeSvc.ListPolicyDecisions(traceID); err == nil {
				for _, d := range decisions {
					policyDecisions = append(policyDecisions, map[string]any{
						"tool":    d.Tool,
						"tier":    d.Tier,
						"allowed": d.Allowed,
						"reason":  d.Reason,
						"time":    d.CreatedAt.Format("15:04:05"),
					})
				}
			}

			json.NewEncoder(w).Encode(map[string]any{
				"trace_id":         traceID,
				"spans":            spans,
				"task":             taskInfo,
				"policy_decisions": policyDecisions,
			})
		})

		// API: Policy Decisions (GET)
		mux.HandleFunc("/api/v1/policy-decisions", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Type", "application/json")

			traceID := r.URL.Query().Get("trace_id")
			if traceID == "" {
				http.Error(w, "trace_id required", http.StatusBadRequest)
				return
			}

			decisions, err := timeSvc.ListPolicyDecisions(traceID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(decisions)
		})

		// API: Trace Graph (GET)
		mux.HandleFunc("/api/v1/trace-graph/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Type", "application/json")

			traceID := strings.TrimPrefix(r.URL.Path, "/api/v1/trace-graph/")
			traceID = strings.TrimSpace(traceID)
			if traceID == "" {
				http.Error(w, "trace_id required", http.StatusBadRequest)
				return
			}

			graph, err := timeSvc.GetTraceGraph(traceID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(graph)
		})

		// API: Group Status (GET)
		mux.HandleFunc("/api/v1/group/status", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Type", "application/json")

			mgr := grpState.Manager()
			if mgr == nil {
				json.NewEncoder(w).Encode(map[string]any{
					"active":       false,
					"group_name":   "",
					"member_count": 0,
				})
				return
			}
			json.NewEncoder(w).Encode(mgr.Status())
		})

		// API: Group Members (GET)
		// Primary source: in-memory roster (always current).
		// Fallback: DB roster (covers members persisted across restarts).
		mux.HandleFunc("/api/v1/group/members", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Type", "application/json")

			mgr := grpState.Manager()
			if mgr != nil && mgr.Active() {
				liveMembers := mgr.Members()
				// Convert to the same shape the frontend expects.
				out := make([]map[string]any, 0, len(liveMembers))
				for _, m := range liveMembers {
					caps, _ := json.Marshal(m.Capabilities)
					chs, _ := json.Marshal(m.Channels)
					out = append(out, map[string]any{
						"agent_id":     m.AgentID,
						"agent_name":   m.AgentName,
						"soul_summary": m.SoulSummary,
						"capabilities": string(caps),
						"channels":     string(chs),
						"model":        m.Model,
						"role":         m.Role,
						"status":       m.Status,
						"last_seen":    m.LastSeen,
					})
				}
				json.NewEncoder(w).Encode(out)
				return
			}

			// Fallback: DB roster
			members, err := timeSvc.ListGroupMembers()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if members == nil {
				members = []timeline.GroupMemberRecord{}
			}
			json.NewEncoder(w).Encode(members)
		})

		// API: Group Join (POST)
		mux.HandleFunc("/api/v1/group/join", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "OPTIONS" {
				return
			}
			if isStandaloneBlocked(w) {
				return
			}
			if r.Method != "POST" {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}

			var body struct {
				GroupName    string `json:"group_name"`
				LFSProxyURL  string `json:"lfs_proxy_url"`
				KafkaBrokers string `json:"kafka_brokers"`
				AgentID      string `json:"agent_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
			groupName := strings.TrimSpace(body.GroupName)
			if groupName == "" {
				http.Error(w, "group_name required", http.StatusBadRequest)
				return
			}

			// Leave existing group if active
			if mgr := grpState.Manager(); mgr != nil && mgr.Active() {
				leaveCtx, leaveCancel := context.WithTimeout(ctx, 5*time.Second)
				_ = mgr.Leave(leaveCtx)
				leaveCancel()
				grpState.Clear()
			}

			// Build new config
			grpCfg := cfg.Group
			grpCfg.GroupName = groupName
			grpCfg.Enabled = true
			if body.LFSProxyURL != "" {
				grpCfg.LFSProxyURL = body.LFSProxyURL
			}
			if body.KafkaBrokers != "" {
				grpCfg.KafkaBrokers = body.KafkaBrokers
			}
			if body.AgentID != "" {
				grpCfg.AgentID = body.AgentID
			}

			mgr := buildGrpManager(grpCfg)

			joinCtx, joinCancel := context.WithTimeout(ctx, 15*time.Second)
			defer joinCancel()
			if err := mgr.Join(joinCtx); err != nil {
				http.Error(w, fmt.Sprintf("join failed: %v", err), http.StatusInternalServerError)
				return
			}

			setupGroupBusSubscription(mgr, msgBus)
			kafkaCancel := startGrpKafka(grpCfg, mgr, ctx, orchDiscoveryHandler(orch))
			grpState.SetManager(mgr, kafkaCancel)

			// Persist settings
			_ = timeSvc.SetSetting("group_name", groupName)
			_ = timeSvc.SetSetting("group_active", "true")
			if body.LFSProxyURL != "" {
				_ = timeSvc.SetSetting("kafscale_lfs_proxy_url", body.LFSProxyURL)
			}

			cfg.Group.Enabled = true
			recalcMode()
			json.NewEncoder(w).Encode(mgr.Status())
		})

		// API: Group Leave (POST)
		mux.HandleFunc("/api/v1/group/leave", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "OPTIONS" {
				return
			}
			if isStandaloneBlocked(w) {
				return
			}
			if r.Method != "POST" {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}

			mgr := grpState.Manager()
			if mgr == nil {
				http.Error(w, "not in a group", http.StatusBadRequest)
				return
			}

			leaveCtx, leaveCancel := context.WithTimeout(ctx, 10*time.Second)
			defer leaveCancel()
			if err := mgr.Leave(leaveCtx); err != nil {
				http.Error(w, fmt.Sprintf("leave failed: %v", err), http.StatusInternalServerError)
				return
			}

			grpState.Clear()
			_ = timeSvc.SetSetting("group_active", "false")
			cfg.Group.Enabled = false
			recalcMode()

			json.NewEncoder(w).Encode(map[string]string{"status": "left"})
		})

		// API: Group Config (GET/POST)
		mux.HandleFunc("/api/v1/group/config", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "OPTIONS" {
				return
			}
			if r.Method == "POST" && isStandaloneBlocked(w) {
				return
			}

			if r.Method == "GET" {
				mgr := grpState.Manager()
				if mgr == nil {
					json.NewEncoder(w).Encode(map[string]any{
						"enabled":          cfg.Group.Enabled,
						"group_name":       cfg.Group.GroupName,
						"lfs_proxy_url":    cfg.Group.LFSProxyURL,
						"api_key":          maskSecret(cfg.Group.LFSProxyAPIKey),
						"kafka_brokers":    cfg.Group.KafkaBrokers,
						"consumer_group":   cfg.Group.ConsumerGroup,
						"agent_id":         cfg.Group.AgentID,
						"poll_interval_ms": cfg.Group.PollIntervalMs,
					})
					return
				}
				grpCfg := mgr.Config()
				json.NewEncoder(w).Encode(map[string]any{
					"enabled":          grpCfg.Enabled,
					"group_name":       grpCfg.GroupName,
					"lfs_proxy_url":    grpCfg.LFSProxyURL,
					"api_key":          maskSecret(grpCfg.LFSProxyAPIKey),
					"kafka_brokers":    grpCfg.KafkaBrokers,
					"consumer_group":   grpCfg.ConsumerGroup,
					"agent_id":         mgr.AgentID(),
					"poll_interval_ms": grpCfg.PollIntervalMs,
				})
				return
			}

			if r.Method == "POST" {
				var body map[string]string
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, "invalid body", http.StatusBadRequest)
					return
				}
				requiresRejoin := false
				for key, val := range body {
					switch key {
					case "lfs_proxy_url":
						_ = timeSvc.SetSetting("kafscale_lfs_proxy_url", val)
						requiresRejoin = true
					case "api_key":
						_ = timeSvc.SetSetting("kafscale_lfs_proxy_api_key", val)
						requiresRejoin = true
					case "kafka_brokers":
						_ = timeSvc.SetSetting("kafka_brokers", val)
						requiresRejoin = true
					case "consumer_group":
						_ = timeSvc.SetSetting("kafka_consumer_group", val)
						requiresRejoin = true
					case "agent_id":
						_ = timeSvc.SetSetting("group_agent_id", val)
						requiresRejoin = true
					case "poll_interval_ms":
						_ = timeSvc.SetSetting("group_poll_interval_ms", val)
					}
				}
				json.NewEncoder(w).Encode(map[string]any{
					"status":          "ok",
					"requires_rejoin": requiresRejoin,
				})
				return
			}

			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		})

		// API: Group Tasks Submit (POST)
		mux.HandleFunc("/api/v1/group/tasks/submit", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "OPTIONS" {
				return
			}
			if isStandaloneBlocked(w) {
				return
			}
			if r.Method != "POST" {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}

			mgr := grpState.Manager()
			if mgr == nil || !mgr.Active() {
				http.Error(w, "not in a group", http.StatusBadRequest)
				return
			}

			var body struct {
				Description string `json:"description"`
				Content     string `json:"content"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
			if strings.TrimSpace(body.Description) == "" {
				http.Error(w, "description required", http.StatusBadRequest)
				return
			}

			taskID := newTraceID()
			submitCtx, submitCancel := context.WithTimeout(ctx, 10*time.Second)
			defer submitCancel()
			if err := mgr.SubmitTask(submitCtx, taskID, body.Description, body.Content); err != nil {
				http.Error(w, fmt.Sprintf("submit failed: %v", err), http.StatusInternalServerError)
				return
			}

			// Persist to local DB
			_ = timeSvc.InsertGroupTask(&timeline.GroupTaskRecord{
				TaskID:      taskID,
				Description: body.Description,
				Content:     body.Content,
				Direction:   "outgoing",
				RequesterID: mgr.AgentID(),
				Status:      "pending",
			})

			json.NewEncoder(w).Encode(map[string]string{"status": "submitted", "task_id": taskID})
		})

		// API: Group Tasks List (GET)
		mux.HandleFunc("/api/v1/group/tasks", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Type", "application/json")

			direction := r.URL.Query().Get("direction")
			status := r.URL.Query().Get("status")
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			if limit == 0 {
				limit = 50
			}
			offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

			tasks, err := timeSvc.ListGroupTasks(direction, status, limit, offset)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if tasks == nil {
				tasks = []timeline.GroupTaskRecord{}
			}
			json.NewEncoder(w).Encode(tasks)
		})

		// API: Group Traces (GET)
		mux.HandleFunc("/api/v1/group/traces", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Type", "application/json")

			agentID := r.URL.Query().Get("agent_id")
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			if limit == 0 {
				limit = 50
			}
			offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

			traces, err := timeSvc.ListAllGroupTraces(limit, offset, agentID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if traces == nil {
				traces = []timeline.GroupTrace{}
			}
			json.NewEncoder(w).Encode(traces)
		})

		// API: Group Memory (GET/POST)
		mux.HandleFunc("/api/v1/group/memory", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "OPTIONS" {
				return
			}
			if r.Method == "POST" && isStandaloneBlocked(w) {
				return
			}

			if r.Method == "POST" {
				mgr := grpState.Manager()
				if mgr == nil || !mgr.Active() {
					http.Error(w, "not in a group", http.StatusBadRequest)
					return
				}
				var body struct {
					Title       string   `json:"title"`
					ContentType string   `json:"content_type"`
					Content     string   `json:"content"`
					Tags        []string `json:"tags"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, "invalid body", http.StatusBadRequest)
					return
				}
				if body.ContentType == "" {
					body.ContentType = "text/plain"
				}
				if err := mgr.ShareMemory(ctx, body.Title, body.ContentType, []byte(body.Content), body.Tags); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				json.NewEncoder(w).Encode(map[string]string{"status": "shared"})
				return
			}

			// GET: list memory items
			authorID := r.URL.Query().Get("author_id")
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			if limit == 0 {
				limit = 50
			}
			offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

			items, err := timeSvc.ListGroupMemoryItems(authorID, limit, offset)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if items == nil {
				items = []timeline.GroupMemoryItemRecord{}
			}
			json.NewEncoder(w).Encode(items)
		})

		// API: Group Skills (GET/POST)
		mux.HandleFunc("/api/v1/group/skills", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "OPTIONS" {
				return
			}
			if r.Method == "POST" && isStandaloneBlocked(w) {
				return
			}

			if r.Method == "POST" {
				mgr := grpState.Manager()
				if mgr == nil || !mgr.Active() {
					http.Error(w, "not in a group", http.StatusBadRequest)
					return
				}
				var body struct {
					SkillName string `json:"skill_name"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, "invalid body", http.StatusBadRequest)
					return
				}
				if body.SkillName == "" {
					http.Error(w, "skill_name required", http.StatusBadRequest)
					return
				}
				if err := mgr.RegisterSkill(ctx, body.SkillName, grpState.Consumer()); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				json.NewEncoder(w).Encode(map[string]string{"status": "registered", "skill": body.SkillName})
				return
			}

			// GET: list skill channels
			groupName := r.URL.Query().Get("group_name")
			if groupName == "" {
				if mgr := grpState.Manager(); mgr != nil {
					groupName = mgr.GroupName()
				}
			}
			channels, err := timeSvc.ListGroupSkillChannels(groupName)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if channels == nil {
				channels = []timeline.GroupSkillChannelRecord{}
			}
			json.NewEncoder(w).Encode(channels)
		})

		// API: Submit Skill Task (POST)
		mux.HandleFunc("/api/v1/group/skills/task", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "OPTIONS" {
				return
			}
			if isStandaloneBlocked(w) {
				return
			}
			if r.Method != "POST" {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}

			mgr := grpState.Manager()
			if mgr == nil || !mgr.Active() {
				http.Error(w, "not in a group", http.StatusBadRequest)
				return
			}

			var body struct {
				SkillName   string `json:"skill_name"`
				Description string `json:"description"`
				Content     string `json:"content"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
			if body.SkillName == "" {
				http.Error(w, "skill_name required", http.StatusBadRequest)
				return
			}

			taskID := newTraceID()
			if err := mgr.SubmitSkillTask(ctx, taskID, body.SkillName, body.Description, body.Content); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"status": "submitted", "task_id": taskID, "skill": body.SkillName})
		})

		// API: Group Onboard (POST)
		mux.HandleFunc("/api/v1/group/onboard", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "OPTIONS" {
				return
			}
			if isStandaloneBlocked(w) {
				return
			}
			if r.Method != "POST" {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}

			mgr := grpState.Manager()
			if mgr == nil {
				http.Error(w, "group not configured", http.StatusBadRequest)
				return
			}

			if err := mgr.Onboard(ctx); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"status": "onboard_request_sent"})
		})

		// API: Group Membership History (GET)
		mux.HandleFunc("/api/v1/group/membership/history", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Type", "application/json")

			agentID := r.URL.Query().Get("agent_id")
			groupName := r.URL.Query().Get("group_name")
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			if limit == 0 {
				limit = 50
			}
			offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

			history, err := timeSvc.GetMembershipHistory(agentID, groupName, limit, offset)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if history == nil {
				history = []timeline.GroupMembershipHistoryRecord{}
			}
			json.NewEncoder(w).Encode(history)
		})

		// API: Previous Group Members (GET)
		mux.HandleFunc("/api/v1/group/members/previous", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Type", "application/json")

			members, err := timeSvc.ListPreviousGroupMembers()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if members == nil {
				members = []timeline.GroupMemberRecord{}
			}
			json.NewEncoder(w).Encode(members)
		})

		// API: Group Rejoin (POST)
		mux.HandleFunc("/api/v1/group/rejoin", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "OPTIONS" {
				return
			}
			if isStandaloneBlocked(w) {
				return
			}
			if r.Method != "POST" {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}

			var body struct {
				AgentID   string `json:"agent_id"`
				GroupName string `json:"group_name"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
			if body.AgentID == "" {
				http.Error(w, "agent_id required", http.StatusBadRequest)
				return
			}

			// Look up previous config from membership history
			groupName := body.GroupName
			if groupName == "" {
				if mgr := grpState.Manager(); mgr != nil {
					groupName = mgr.GroupName()
				}
			}

			prevConfig, err := timeSvc.GetLatestMembershipConfig(body.AgentID, groupName)
			if err != nil {
				http.Error(w, "no previous config found for this agent", http.StatusNotFound)
				return
			}

			// Reactivate the member in the roster
			if err := timeSvc.ReactivateGroupMember(body.AgentID); err != nil {
				http.Error(w, fmt.Sprintf("reactivate failed: %v", err), http.StatusInternalServerError)
				return
			}

			json.NewEncoder(w).Encode(map[string]any{
				"status":          "rejoined",
				"agent_id":        body.AgentID,
				"restored_config": prevConfig,
			})
		})

		// API: Group Stats (GET)
		mux.HandleFunc("/api/v1/group/stats", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Type", "application/json")

			stats, err := timeSvc.GetGroupStats()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(stats)
		})

		// API: Unified Audit Log (GET)
		mux.HandleFunc("/api/v1/group/audit", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Type", "application/json")

			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			if limit == 0 {
				limit = 50
			}
			offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

			filter := timeline.AuditFilter{
				Source:    r.URL.Query().Get("source"),
				EventType: r.URL.Query().Get("event_type"),
				AgentID:   r.URL.Query().Get("agent_id"),
				Limit:     limit,
				Offset:    offset,
			}

			entries, err := timeSvc.ListUnifiedAudit(filter)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if entries == nil {
				entries = []timeline.UnifiedAuditEntry{}
			}
			json.NewEncoder(w).Encode(entries)
		})

		// API: Group Topic Manifest (GET)
		mux.HandleFunc("/api/v1/group/manifest", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Type", "application/json")

			mgr := grpState.Manager()
			if mgr == nil {
				json.NewEncoder(w).Encode(map[string]any{"error": "no group manager"})
				return
			}
			tm := mgr.TopicManager()
			if tm == nil {
				json.NewEncoder(w).Encode(map[string]any{"error": "no topic manager"})
				return
			}
			json.NewEncoder(w).Encode(tm.Manifest())
		})

		// API: Group Topics — enriched topic list with stats (GET), browse messages (?browse=topicName)
		mux.HandleFunc("/api/v1/group/topics", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Type", "application/json")

			// Browse mode: return recent messages for a specific topic
			if browseTopic := r.URL.Query().Get("browse"); browseTopic != "" {
				limitStr := r.URL.Query().Get("limit")
				limit := 50
				if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
					limit = n
				}
				msgs, err := timeSvc.GetTopicMessages(browseTopic, limit)
				if err != nil {
					json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
					return
				}
				if msgs == nil {
					msgs = []timeline.TopicMessageLogRecord{}
				}
				json.NewEncoder(w).Encode(map[string]any{"messages": msgs})
				return
			}

			// Build enriched topic list: manifest + stats + health
			mgr := grpState.Manager()
			var coreTopics, skillTopics []map[string]any

			// Get manifest topics
			if mgr != nil {
				if tm := mgr.TopicManager(); tm != nil {
					manifest := tm.Manifest()
					for _, td := range manifest.CoreTopics {
						coreTopics = append(coreTopics, map[string]any{
							"name": td.Name, "category": td.Category,
							"description": td.Description, "consumers": td.Consumers,
						})
					}
					for _, td := range manifest.SkillTopics {
						skillTopics = append(skillTopics, map[string]any{
							"name": td.Name, "category": td.Category,
							"description": td.Description, "consumers": td.Consumers,
						})
					}
				}
			}
			if coreTopics == nil {
				coreTopics = []map[string]any{}
			}
			if skillTopics == nil {
				skillTopics = []map[string]any{}
			}

			// Merge stats into topics
			topicStats, _ := timeSvc.GetTopicStats()
			statsMap := make(map[string]timeline.TopicStat)
			for _, ts := range topicStats {
				statsMap[ts.TopicName] = ts
			}
			topicHealth, _ := timeSvc.GetTopicHealth()
			healthMap := make(map[string]timeline.TopicHealth)
			for _, th := range topicHealth {
				healthMap[th.TopicName] = th
			}

			// Track which topics are already in the manifest
			knownTopics := make(map[string]bool)
			for i, t := range coreTopics {
				name := t["name"].(string)
				knownTopics[name] = true
				if st, ok := statsMap[name]; ok {
					coreTopics[i]["stats"] = st
				}
				if h, ok := healthMap[name]; ok {
					coreTopics[i]["health"] = h
				}
			}
			for i, t := range skillTopics {
				name := t["name"].(string)
				knownTopics[name] = true
				if st, ok := statsMap[name]; ok {
					skillTopics[i]["stats"] = st
				}
				if h, ok := healthMap[name]; ok {
					skillTopics[i]["health"] = h
				}
			}

			// Add topics from stats that aren't in the manifest (fallback discovery)
			for topicName, st := range statsMap {
				if knownTopics[topicName] {
					continue
				}
				cat := inferTopicCategory(topicName)
				entry := map[string]any{
					"name": topicName, "category": cat,
					"description": "Discovered from message log", "consumers": []string{},
					"stats": st,
				}
				if h, ok := healthMap[topicName]; ok {
					entry["health"] = h
				}
				if strings.Contains(topicName, ".skill.") {
					skillTopics = append(skillTopics, entry)
				} else {
					coreTopics = append(coreTopics, entry)
				}
			}

			// XP leaderboard (deduplicated by agent_id)
			xpRaw, _ := timeSvc.GetAgentXP()
			seen := make(map[string]bool)
			xp := make([]timeline.AgentXP, 0, len(xpRaw))
			for _, a := range xpRaw {
				if !seen[a.AgentID] {
					seen[a.AgentID] = true
					xp = append(xp, a)
				}
			}

			json.NewEncoder(w).Encode(map[string]any{
				"topics":         coreTopics,
				"skill_topics":   skillTopics,
				"xp_leaderboard": xp,
			})
		})

		// API: Group Topic Flow Data (GET)
		mux.HandleFunc("/api/v1/group/topics/flow", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Type", "application/json")

			flow, err := timeSvc.GetTopicFlowData()
			if err != nil {
				json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
				return
			}
			if flow == nil {
				flow = []timeline.TopicFlowEdge{}
			}
			json.NewEncoder(w).Encode(map[string]any{"edges": flow})
		})

		// API: Group Topic Health (GET)
		mux.HandleFunc("/api/v1/group/topics/health", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Type", "application/json")

			health, err := timeSvc.GetTopicHealth()
			if err != nil {
				json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
				return
			}
			if health == nil {
				health = []timeline.TopicHealth{}
			}
			json.NewEncoder(w).Encode(map[string]any{"health": health})
		})

		// API: Group Topic Ensure (POST)
		mux.HandleFunc("/api/v1/group/topics/ensure", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Content-Type", "application/json")

			if r.Method == "OPTIONS" {
				return
			}
			if isStandaloneBlocked(w) {
				return
			}
			if r.Method != "POST" {
				http.Error(w, "POST only", http.StatusMethodNotAllowed)
				return
			}

			mgr := grpState.Manager()
			if mgr == nil {
				json.NewEncoder(w).Encode(map[string]any{"error": "no group manager"})
				return
			}

			var body struct {
				TopicName string `json:"topic_name"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.TopicName == "" {
				http.Error(w, "topic_name required", http.StatusBadRequest)
				return
			}

			if err := mgr.EnsureTopic(r.Context(), body.TopicName); err != nil {
				json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
				return
			}
			// Log the ensure event locally so stats are immediately visible
			// (the Kafka round-trip may not complete before the UI refreshes).
			_ = timeSvc.LogTopicMessage(&timeline.TopicMessageLogRecord{
				TopicName:     body.TopicName,
				SenderID:      mgr.AgentID(),
				EnvelopeType:  "heartbeat",
				CorrelationID: "ensure",
				PayloadSize:   0,
			})
			json.NewEncoder(w).Encode(map[string]any{"ok": true, "topic": body.TopicName})
		})

		// API: Group Agent XP Leaderboard (GET)
		mux.HandleFunc("/api/v1/group/topics/xp", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Type", "application/json")

			xp, err := timeSvc.GetAgentXP()
			if err != nil {
				json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
				return
			}
			if xp == nil {
				xp = []timeline.AgentXP{}
			}
			json.NewEncoder(w).Encode(map[string]any{"leaderboard": xp})
		})

		// API: Group Topic Density (GET) — hourly buckets + envelope types for sparkline popup
		mux.HandleFunc("/api/v1/group/topics/density", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Type", "application/json")

			topicName := r.URL.Query().Get("topic")
			if topicName == "" {
				http.Error(w, "topic parameter required", http.StatusBadRequest)
				return
			}
			hours := 48
			if h, err := strconv.Atoi(r.URL.Query().Get("hours")); err == nil && h > 0 {
				hours = h
			}

			buckets, err := timeSvc.GetTopicMessageDensity(topicName, hours)
			if err != nil {
				json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
				return
			}
			if buckets == nil {
				buckets = []timeline.TopicDensityBucket{}
			}

			envTypes, err := timeSvc.GetTopicEnvelopeTypeCounts(topicName)
			if err != nil {
				json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
				return
			}
			if envTypes == nil {
				envTypes = map[string]int{}
			}

			json.NewEncoder(w).Encode(map[string]any{
				"topic":          topicName,
				"hours":          hours,
				"buckets":        buckets,
				"envelope_types": envTypes,
			})
		})

		// API: Settings (GET/POST)
		mux.HandleFunc("/api/v1/settings", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Content-Type", "application/json")

			if r.Method == "OPTIONS" {
				return
			}

			if r.Method == "POST" {
				var body struct {
					Key   string `json:"key"`
					Value string `json:"value"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, "invalid body", http.StatusBadRequest)
					return
				}
				if err := timeSvc.SetSetting(body.Key, body.Value); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				fmt.Printf("⚙️ Setting changed: %s = %s\n", body.Key, body.Value)
				// Auto-reload WhatsApp auth when allowlist/denylist changes
				if body.Key == "whatsapp_allowlist" || body.Key == "whatsapp_denylist" || body.Key == "whatsapp_pair_token" {
					wa.ReloadAuth()
				}
				json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
				return
			}

			// GET: return all requested settings
			key := r.URL.Query().Get("key")
			if key != "" {
				val, err := timeSvc.GetSetting(key)
				if err != nil {
					json.NewEncoder(w).Encode(map[string]string{"key": key, "value": ""})
					return
				}
				json.NewEncoder(w).Encode(map[string]string{"key": key, "value": val})
				return
			}
			// Return silent_mode by default
			json.NewEncoder(w).Encode(map[string]bool{"silent_mode": timeSvc.IsSilentMode()})
		})

		// API: Memory Status (GET)
		mux.HandleFunc("/api/v1/memory/status", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "OPTIONS" {
				return
			}

			type layerInfo struct {
				Name         string `json:"name"`
				SourcePrefix string `json:"source_prefix"`
				Description  string `json:"description"`
				TTLDays      int    `json:"ttl_days"`
				ChunkCount   int    `json:"chunk_count"`
				Color        string `json:"color"`
			}

			// Get chunk stats from lifecycle manager
			stats, _ := lifecycleMgr.Stats()

			layers := []layerInfo{
				{Name: "soul", SourcePrefix: "soul:", Description: "Identity and personality files loaded at startup", TTLDays: 0, ChunkCount: stats.BySource["soul"], Color: "#a855f7"},
				{Name: "conversation", SourcePrefix: "conversation:", Description: "Auto-indexed Q&A pairs from your conversations", TTLDays: 30, ChunkCount: stats.BySource["conversation"], Color: "#58a6ff"},
				{Name: "tool", SourcePrefix: "tool:", Description: "Tool execution outputs and results", TTLDays: 14, ChunkCount: stats.BySource["tool"], Color: "#fb923c"},
				{Name: "group", SourcePrefix: "group:", Description: "Shared knowledge from group collaboration", TTLDays: 60, ChunkCount: stats.BySource["group"], Color: "#22c55e"},
				{Name: "er1", SourcePrefix: "er1:", Description: "Personal memories synced from ER1", TTLDays: 0, ChunkCount: stats.BySource["er1"], Color: "#fbbf24"},
				{Name: "observation", SourcePrefix: "observation:", Description: "Compressed observations from conversation analysis", TTLDays: 0, ChunkCount: stats.BySource["observation"], Color: "#67e8f9"},
			}

			// Working memory
			wmEntries := 0
			wmPreview := ""
			if workingMemoryStore != nil {
				if entries, err := workingMemoryStore.ListAll(); err == nil {
					wmEntries = len(entries)
					if len(entries) > 0 {
						wmPreview = entries[0].Content
						if len(wmPreview) > 500 {
							wmPreview = wmPreview[:500] + "..."
						}
					}
				}
			}

			// Observer status
			observerStatus := map[string]any{
				"enabled":           false,
				"observation_count": 0,
				"queue_depth":       0,
				"last_observation":  nil,
			}
			if observer != nil {
				os := observer.Status()
				observerStatus["enabled"] = os.Enabled
				observerStatus["observation_count"] = os.ObservationCount
				observerStatus["queue_depth"] = os.QueueDepth
				if !os.LastObservation.IsZero() {
					observerStatus["last_observation"] = os.LastObservation
				}
			}

			// Recent observations
			type obsJSON struct {
				ID       string `json:"id"`
				Content  string `json:"content"`
				Priority string `json:"priority"`
				Date     string `json:"date"`
			}
			var recentObs []obsJSON
			if observer != nil {
				if obs, err := observer.AllObservations(20); err == nil {
					for _, o := range obs {
						recentObs = append(recentObs, obsJSON{
							ID:       o.ID,
							Content:  o.Content,
							Priority: o.Priority,
							Date:     o.ObservedAt.Format(time.RFC3339),
						})
					}
				}
			}

			// ER1 status
			er1Status := map[string]any{
				"connected":    false,
				"last_sync":    nil,
				"synced_count": 0,
				"url":          "",
			}
			if er1Client != nil {
				es := er1Client.Status()
				er1Status["connected"] = es.Connected
				er1Status["url"] = es.URL
				er1Status["synced_count"] = stats.BySource["er1"]
				if !es.LastSync.IsZero() {
					er1Status["last_sync"] = es.LastSync
				}
			}

			// Expertise
			type expertiseJSON struct {
				Skill string  `json:"skill"`
				Score float64 `json:"score"`
				Trend string  `json:"trend"`
				Uses  int     `json:"uses"`
			}
			var expertise []expertiseJSON
			if expertiseTracker != nil {
				if skills, err := expertiseTracker.ListExpertise(); err == nil {
					for _, s := range skills {
						expertise = append(expertise, expertiseJSON{
							Skill: s.SkillName,
							Score: s.Score,
							Trend: s.Trend,
							Uses:  s.SuccessCount + s.FailureCount,
						})
					}
				}
			}

			// Totals
			totals := map[string]any{
				"total_chunks": stats.TotalChunks,
				"max_chunks":   50000,
			}
			if stats.OldestChunk != nil {
				totals["oldest"] = stats.OldestChunk
			}
			if stats.NewestChunk != nil {
				totals["newest"] = stats.NewestChunk
			}

			// Config
			observerEnabled := observer != nil
			observerThreshold := 50
			observerMaxObs := 200
			if observer != nil {
				observerThreshold = cfg.Observer.MessageThreshold
				observerMaxObs = cfg.Observer.MaxObservations
			}
			er1URL := ""
			er1SyncIntervalSec := 300
			if er1Client != nil {
				es := er1Client.Status()
				er1URL = es.URL
				er1SyncIntervalSec = int(cfg.ER1.SyncInterval.Seconds())
				if er1SyncIntervalSec <= 0 {
					er1SyncIntervalSec = 300
				}
			}

			memConfig := map[string]any{
				"observer_enabled":      observerEnabled,
				"observer_threshold":    observerThreshold,
				"observer_max_obs":      observerMaxObs,
				"er1_url":               er1URL,
				"er1_sync_interval_sec": er1SyncIntervalSec,
				"max_chunks":            50000,
			}

			json.NewEncoder(w).Encode(map[string]any{
				"layers":         layers,
				"working_memory": map[string]any{"entries": wmEntries, "preview": wmPreview},
				"observer":       observerStatus,
				"observations":   recentObs,
				"er1":            er1Status,
				"expertise":      expertise,
				"totals":         totals,
				"config":         memConfig,
			})
		})

		// API: Memory + Knowledge Metrics (GET)
		mux.HandleFunc("/api/v1/memory/metrics", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "OPTIONS" {
				return
			}
			if r.Method != http.MethodGet {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			payload, err := collectMemoryKnowledgeMetrics(timeSvc)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(payload)
		})

		// API: Memory Reset (POST)
		mux.HandleFunc("/api/v1/memory/reset", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "OPTIONS" {
				return
			}
			if r.Method != http.MethodPost {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}

			var body struct {
				Layer string `json:"layer"` // "soul", "conversation", "tool", "group", "er1", "observation", "working_memory", "all"
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}

			deleted := 0
			var resetErr error

			switch body.Layer {
			case "all":
				deleted, resetErr = lifecycleMgr.DeleteAll()
				if resetErr == nil && workingMemoryStore != nil {
					_ = workingMemoryStore.DeleteAll()
				}
			case "working_memory":
				if workingMemoryStore != nil {
					resetErr = workingMemoryStore.DeleteAll()
				}
			case "soul", "conversation", "tool", "group", "er1", "observation":
				deleted, resetErr = lifecycleMgr.DeleteBySource(body.Layer + ":")
			default:
				http.Error(w, "invalid layer", http.StatusBadRequest)
				return
			}

			if resetErr != nil {
				http.Error(w, resetErr.Error(), http.StatusInternalServerError)
				return
			}

			fmt.Printf("🧹 Memory reset: layer=%s deleted=%d\n", body.Layer, deleted)
			json.NewEncoder(w).Encode(map[string]any{"status": "ok", "deleted": deleted})
		})

		// API: Memory Config (POST)
		mux.HandleFunc("/api/v1/memory/config", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "OPTIONS" {
				return
			}
			if r.Method != http.MethodPost {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}

			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}

			// Save each config key as a setting
			for key, value := range body {
				strVal := fmt.Sprintf("%v", value)
				if err := timeSvc.SetSetting("memory_"+key, strVal); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				fmt.Printf("⚙️ Memory config changed: %s = %s\n", key, strVal)
			}

			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		})

		// API: Memory Prune (POST)
		mux.HandleFunc("/api/v1/memory/prune", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "OPTIONS" {
				return
			}
			if r.Method != http.MethodPost {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}

			deleted, err := lifecycleMgr.Prune()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			fmt.Printf("🧹 Memory prune triggered: deleted=%d\n", deleted)
			json.NewEncoder(w).Encode(map[string]any{"status": "ok", "deleted": deleted})
		})

		// API: Embedding Runtime Status (GET)
		mux.HandleFunc("/api/v1/memory/embedding/status", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "OPTIONS" {
				return
			}
			if r.Method != http.MethodGet {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}

			health := probeEmbeddingRuntime(cfg)
			embeddedCount, _ := countEmbeddedMemoryChunks()
			pendingInstallAt, _ := timeSvc.GetSetting("memory_embedding_install_requested_at")
			pendingInstallModel, _ := timeSvc.GetSetting("memory_embedding_install_model")

			json.NewEncoder(w).Encode(map[string]any{
				"status": "ok",
				"embedding": map[string]any{
					"enabled":         cfg.Memory.Embedding.Enabled,
					"provider":        cfg.Memory.Embedding.Provider,
					"model":           cfg.Memory.Embedding.Model,
					"dimension":       cfg.Memory.Embedding.Dimension,
					"normalize":       cfg.Memory.Embedding.Normalize,
					"cacheDir":        cfg.Memory.Embedding.CacheDir,
					"autoDownload":    cfg.Memory.Embedding.AutoDownload,
					"endpoint":        cfg.Memory.Embedding.Endpoint,
					"startupTimeoutS": cfg.Memory.Embedding.StartupTimeoutSec,
					"fingerprint":     memoryEmbeddingFingerprint(cfg),
				},
				"runtime": map[string]any{
					"ready":      health.Ready,
					"status":     health.Status,
					"detail":     health.Detail,
					"checkedAt":  health.CheckedAt,
					"httpStatus": health.HTTPStatus,
				},
				"index": map[string]any{
					"embeddedChunks": embeddedCount,
				},
				"install": map[string]any{
					"pending":            strings.TrimSpace(pendingInstallAt) != "",
					"requestedAt":        strings.TrimSpace(pendingInstallAt),
					"requestedModel":     strings.TrimSpace(pendingInstallModel),
					"cachePathAvailable": embeddingCachePresent(cfg.Memory.Embedding.CacheDir),
				},
			})
		})

		// API: Embedding Runtime Health (GET)
		mux.HandleFunc("/api/v1/memory/embedding/healthz", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "OPTIONS" {
				return
			}
			if r.Method != http.MethodGet {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			health := probeEmbeddingRuntime(cfg)
			code := http.StatusOK
			if !health.Ready {
				code = http.StatusServiceUnavailable
			}
			w.WriteHeader(code)
			json.NewEncoder(w).Encode(map[string]any{
				"ready":      health.Ready,
				"status":     health.Status,
				"detail":     health.Detail,
				"checkedAt":  health.CheckedAt,
				"httpStatus": health.HTTPStatus,
			})
		})

		// API: Embedding Runtime Install Bootstrap (POST)
		mux.HandleFunc("/api/v1/memory/embedding/install", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "OPTIONS" {
				return
			}
			if r.Method != http.MethodPost {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var body struct {
				Model string `json:"model"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			model := strings.TrimSpace(body.Model)
			if model == "" {
				model = strings.TrimSpace(cfg.Memory.Embedding.Model)
			}
			if model == "" {
				http.Error(w, "embedding model is required", http.StatusBadRequest)
				return
			}
			_ = timeSvc.SetSetting("memory_embedding_install_requested_at", time.Now().UTC().Format(time.RFC3339))
			_ = timeSvc.SetSetting("memory_embedding_install_model", model)

			cacheDir := strings.TrimSpace(cfg.Memory.Embedding.CacheDir)
			if cacheDir != "" {
				expanded := cacheDir
				if strings.HasPrefix(expanded, "~") {
					if home, err := os.UserHomeDir(); err == nil {
						expanded = filepath.Join(home, strings.TrimPrefix(expanded, "~"))
					}
				}
				_ = os.MkdirAll(expanded, 0o755)
			}

			json.NewEncoder(w).Encode(map[string]any{
				"status": "ok",
				"action": "install-requested",
				"model":  model,
			})
		})

		// API: Embedding Runtime Reindex (POST)
		mux.HandleFunc("/api/v1/memory/embedding/reindex", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "OPTIONS" {
				return
			}
			if r.Method != http.MethodPost {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var body struct {
				ConfirmWipe bool   `json:"confirmWipe"`
				Reason      string `json:"reason"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
			if !body.ConfirmWipe {
				http.Error(w, "confirmWipe must be true", http.StatusBadRequest)
				return
			}
			wiped, err := wipeAllMemoryChunks()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			reason := strings.TrimSpace(body.Reason)
			if reason == "" {
				reason = "manual_reindex"
			}
			_ = timeSvc.AddEvent(&timeline.TimelineEvent{
				EventID:        fmt.Sprintf("MEMORY_EMBED_REINDEX_%d", time.Now().UnixNano()),
				Timestamp:      time.Now(),
				SenderID:       "system",
				SenderName:     "KafClaw",
				EventType:      "SYSTEM",
				ContentText:    fmt.Sprintf("embedding reindex requested; wiped_chunks=%d reason=%s", wiped, reason),
				Classification: "MEMORY_EMBEDDING_REINDEX",
				Authorized:     true,
			})
			json.NewEncoder(w).Encode(map[string]any{
				"status":      "ok",
				"wipedChunks": wiped,
				"reason":      reason,
			})
		})

		// API: Work Repo (GET/POST)
		mux.HandleFunc("/api/v1/workrepo", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Content-Type", "application/json")

			if r.Method == "OPTIONS" {
				return
			}

			switch r.Method {
			case "GET":
				workRepoMu.RLock()
				current := workRepoPath
				workRepoMu.RUnlock()
				json.NewEncoder(w).Encode(map[string]string{"path": current})
			case "POST":
				var body struct {
					Path string `json:"path"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, "invalid body", http.StatusBadRequest)
					return
				}
				newPath := strings.TrimSpace(body.Path)
				if newPath == "" {
					http.Error(w, "path required", http.StatusBadRequest)
					return
				}
				// If multiple absolute paths got concatenated, keep the last one.
				if idx := strings.LastIndex(newPath, "/Users/"); idx > 0 {
					newPath = newPath[idx:]
				}
				if idx := strings.LastIndex(newPath, "C:\\"); idx > 0 {
					newPath = newPath[idx:]
				}
				if strings.HasPrefix(newPath, "~") {
					home, _ := os.UserHomeDir()
					newPath = filepath.Join(home, newPath[1:])
				}
				if !filepath.IsAbs(newPath) {
					if abs, err := filepath.Abs(newPath); err == nil {
						newPath = abs
					}
				}
				if warn, err := config.EnsureWorkRepo(newPath); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				} else if warn != "" {
					fmt.Printf("Work repo warning: %s\n", warn)
				}
				if err := timeSvc.SetSetting("work_repo_path", newPath); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				workRepoMu.Lock()
				workRepoPath = newPath
				workRepoMu.Unlock()
				json.NewEncoder(w).Encode(map[string]string{"status": "ok", "path": newPath})
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		})

		// API: Repo Tree (GET)
		mux.HandleFunc("/api/v1/repo/tree", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Type", "application/json")

			base := resolveRepo(r)
			repoPath := base
			sub := strings.TrimSpace(r.URL.Query().Get("path"))
			if sub != "" {
				repoPath = filepath.Join(repoPath, sub)
			}
			items, err := listRepoTree(repoPath, base)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(items)
		})

		// API: Repo File (GET)
		mux.HandleFunc("/api/v1/repo/file", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Type", "application/json")

			repo := resolveRepo(r)
			rel := filepath.Clean(strings.TrimSpace(r.URL.Query().Get("path")))
			if rel == "" || rel == "." || strings.Contains(rel, "..") {
				http.Error(w, "path required", http.StatusBadRequest)
				return
			}
			full := filepath.Join(repo, rel)
			if verified, err := filepath.Rel(repo, full); err != nil || strings.HasPrefix(verified, "..") {
				http.Error(w, "path outside repo", http.StatusBadRequest)
				return
			}
			data, err := os.ReadFile(full)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if !utf8.Valid(data) {
				json.NewEncoder(w).Encode(map[string]string{"path": rel, "content": "[binary file]"})
				return
			}
			if len(data) > 200_000 {
				json.NewEncoder(w).Encode(map[string]string{"path": rel, "content": string(data[:200_000]) + "\n... (truncated)"})
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"path": rel, "content": string(data)})
		})

		// API: Repo Status (GET)
		mux.HandleFunc("/api/v1/repo/status", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Type", "application/json")
			rp := resolveRepo(r)
			out, err := runGit(rp, "status", "-sb")
			if err != nil {
				json.NewEncoder(w).Encode(map[string]string{"status": "", "error": err.Error()})
				return
			}
			remote, _ := runGit(rp, "remote", "-v")
			json.NewEncoder(w).Encode(map[string]string{"status": out, "remote": remote})
		})

		// API: Repo Search (GET)
		mux.HandleFunc("/api/v1/repo/search", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Type", "application/json")

			root, _ := timeSvc.GetSetting("default_repo_search_path")
			root = strings.TrimSpace(root)
			if root == "" {
				json.NewEncoder(w).Encode(map[string]any{"root": "", "repos": []string{}})
				return
			}
			if strings.HasPrefix(root, "~") {
				home, _ := os.UserHomeDir()
				root = filepath.Join(home, root[1:])
			}
			if abs, err := filepath.Abs(root); err == nil {
				root = abs
			}

			entries, err := os.ReadDir(root)
			if err != nil {
				json.NewEncoder(w).Encode(map[string]any{"root": root, "repos": []string{}})
				return
			}

			repos := make([]string, 0, len(entries))
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				path := filepath.Join(root, e.Name())
				if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
					repos = append(repos, path)
				}
			}

			json.NewEncoder(w).Encode(map[string]any{"root": root, "repos": repos})
		})

		// API: GitHub Auth Status (GET)
		mux.HandleFunc("/api/v1/repo/gh-auth", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Type", "application/json")
			out, err := runGh(resolveRepo(r), "auth", "status", "-h", "github.com")
			if err != nil {
				json.NewEncoder(w).Encode(map[string]string{"status": "not_authenticated", "detail": err.Error()})
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"status": "ok", "detail": out})
		})

		// API: Repo Branches (GET)
		mux.HandleFunc("/api/v1/repo/branches", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Type", "application/json")
			out, err := runGit(resolveRepo(r), "branch", "--format=%(refname:short)")
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			lines := []string{}
			for _, line := range strings.Split(out, "\n") {
				line = strings.TrimSpace(line)
				if line != "" {
					lines = append(lines, line)
				}
			}
			json.NewEncoder(w).Encode(map[string]any{"branches": lines})
		})

		// API: Repo Checkout Branch (POST)
		mux.HandleFunc("/api/v1/repo/checkout", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "OPTIONS" {
				return
			}
			var body struct {
				Branch string `json:"branch"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
			branch := strings.TrimSpace(body.Branch)
			if branch == "" || strings.HasPrefix(branch, "-") {
				http.Error(w, "invalid branch name", http.StatusBadRequest)
				return
			}
			out, err := runGit(resolveRepo(r), "checkout", branch)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"result": out})
		})

		// API: Repo Log (GET)
		mux.HandleFunc("/api/v1/repo/log", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Type", "application/json")
			limit := strings.TrimSpace(r.URL.Query().Get("limit"))
			if limit == "" {
				limit = "20"
			}
			if n, err := strconv.Atoi(limit); err != nil || n < 1 || n > 500 {
				http.Error(w, "limit must be a number between 1 and 500", http.StatusBadRequest)
				return
			}
			out, err := runGit(resolveRepo(r), "log", "--oneline", "-n", limit)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			lines := []string{}
			for _, line := range strings.Split(out, "\n") {
				line = strings.TrimSpace(line)
				if line != "" {
					lines = append(lines, line)
				}
			}
			json.NewEncoder(w).Encode(map[string]any{"commits": lines})
		})

		// API: Repo File Diff (GET)
		mux.HandleFunc("/api/v1/repo/diff-file", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Type", "application/json")
			rel := filepath.Clean(strings.TrimSpace(r.URL.Query().Get("path")))
			if rel == "" || rel == "." || strings.HasPrefix(rel, "-") {
				http.Error(w, "invalid path", http.StatusBadRequest)
				return
			}
			out, err := runGit(resolveRepo(r), "diff", "--", rel)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"diff": out})
		})

		// API: Repo Diff (GET)
		mux.HandleFunc("/api/v1/repo/diff", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Type", "application/json")
			rel := filepath.Clean(strings.TrimSpace(r.URL.Query().Get("path")))
			args := []string{"diff"}
			if rel != "" && rel != "." && !strings.HasPrefix(rel, "-") {
				args = append(args, "--", rel)
			}
			out, err := runGit(resolveRepo(r), args...)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"diff": out})
		})

		// API: Repo Commit (POST)
		mux.HandleFunc("/api/v1/repo/commit", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "OPTIONS" {
				return
			}
			var body struct {
				Message string `json:"message"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
			msg := strings.TrimSpace(body.Message)
			if msg == "" {
				http.Error(w, "message required", http.StatusBadRequest)
				return
			}
			rp := resolveRepo(r)
			if _, err := runGit(rp, "add", "-A"); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			out, err := runGit(rp, "commit", "-m", msg)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"result": out})
		})

		// API: Repo Pull (POST)
		mux.HandleFunc("/api/v1/repo/pull", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "OPTIONS" {
				return
			}
			out, err := runGit(resolveRepo(r), "pull", "--ff-only")
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"result": out})
		})

		// API: Repo Push (POST)
		mux.HandleFunc("/api/v1/repo/push", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "OPTIONS" {
				return
			}
			out, err := runGit(resolveRepo(r), "push")
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"result": out})
		})

		// API: Repo Init (POST)
		mux.HandleFunc("/api/v1/repo/init", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "OPTIONS" {
				return
			}
			var body struct {
				RemoteURL string `json:"remote_url"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
			repo := resolveRepo(r)
			if warn, err := config.EnsureWorkRepo(repo); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			} else if warn != "" {
				fmt.Printf("Work repo warning: %s\n", warn)
			}
			remoteURL := strings.TrimSpace(body.RemoteURL)
			if remoteURL != "" && !strings.HasPrefix(remoteURL, "-") {
				_, _ = runGit(repo, "remote", "remove", "origin")
				if _, err := runGit(repo, "remote", "add", "origin", remoteURL); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
			}
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		})

		// API: Repo PR (POST) using gh
		mux.HandleFunc("/api/v1/repo/pr", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "OPTIONS" {
				return
			}
			var body struct {
				Title string `json:"title"`
				Body  string `json:"body"`
				Base  string `json:"base"`
				Head  string `json:"head"`
				Draft bool   `json:"draft"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
			if strings.TrimSpace(body.Title) == "" {
				http.Error(w, "title required", http.StatusBadRequest)
				return
			}
			args := []string{"pr", "create", "--title", body.Title, "--body", body.Body}
			if body.Base != "" {
				args = append(args, "--base", body.Base)
			}
			if body.Head != "" {
				args = append(args, "--head", body.Head)
			}
			if body.Draft {
				args = append(args, "--draft")
			}
			out, err := runGh(resolveRepo(r), args...)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"result": out})
		})

		// API: Web Users (GET/POST)
		mux.HandleFunc("/api/v1/webusers", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Content-Type", "application/json")

			if r.Method == "OPTIONS" {
				return
			}

			switch r.Method {
			case "GET":
				users, err := timeSvc.ListWebUsers()
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				if users == nil {
					users = []timeline.WebUser{}
				}
				if err := json.NewEncoder(w).Encode(users); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
			case "POST":
				var body struct {
					Name string `json:"name"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, "invalid body", http.StatusBadRequest)
					return
				}
				user, err := timeSvc.CreateWebUser(strings.TrimSpace(body.Name))
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				json.NewEncoder(w).Encode(user)
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		})

		// API: Web User Force Send (POST)
		mux.HandleFunc("/api/v1/webusers/force", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Content-Type", "application/json")

			if r.Method == "OPTIONS" {
				return
			}
			if r.Method != "POST" {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}

			var body struct {
				WebUserID int64 `json:"web_user_id"`
				ForceSend bool  `json:"force_send"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
			if body.WebUserID == 0 {
				http.Error(w, "web_user_id required", http.StatusBadRequest)
				return
			}
			if err := timeSvc.SetWebUserForceSend(body.WebUserID, body.ForceSend); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		})

		// API: Web Links (GET/POST)
		mux.HandleFunc("/api/v1/weblinks", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Content-Type", "application/json")

			if r.Method == "OPTIONS" {
				return
			}

			switch r.Method {
			case "GET":
				idStr := r.URL.Query().Get("web_user_id")
				webUserID, err := strconv.ParseInt(idStr, 10, 64)
				if err != nil {
					http.Error(w, "invalid web_user_id", http.StatusBadRequest)
					return
				}
				jid, ok, err := timeSvc.GetWebLink(webUserID)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				if !ok {
					jid = ""
				}
				json.NewEncoder(w).Encode(map[string]string{
					"web_user_id":  idStr,
					"whatsapp_jid": jid,
				})
			case "POST":
				var body struct {
					WebUserID   int64  `json:"web_user_id"`
					WhatsAppJID string `json:"whatsapp_jid"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, "invalid body", http.StatusBadRequest)
					return
				}
				if body.WebUserID == 0 {
					http.Error(w, "web_user_id required", http.StatusBadRequest)
					return
				}
				if strings.TrimSpace(body.WhatsAppJID) == "" {
					if err := timeSvc.UnlinkWebUser(body.WebUserID); err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
						return
					}
				} else {
					jid := normalizeWhatsAppJID(strings.TrimSpace(body.WhatsAppJID))
					if err := timeSvc.LinkWebUser(body.WebUserID, jid); err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
						return
					}
				}
				json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		})

		// API: Web Chat Send
		mux.HandleFunc("/api/v1/webchat/send", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Content-Type", "application/json")

			if r.Method == "OPTIONS" {
				return
			}
			if r.Method != "POST" {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}

			var body struct {
				WebUserID int64  `json:"web_user_id"`
				Message   string `json:"message"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
			if body.WebUserID == 0 || strings.TrimSpace(body.Message) == "" {
				http.Error(w, "web_user_id and message required", http.StatusBadRequest)
				return
			}

			user, err := timeSvc.GetWebUser(body.WebUserID)
			if err != nil {
				http.Error(w, "web user not found", http.StatusBadRequest)
				return
			}
			traceID := newTraceID()

			// Resolve link (optional) and maybe forward the input itself to WhatsApp
			jid, ok, err := timeSvc.GetWebLink(body.WebUserID)
			if err != nil {
				http.Error(w, "link lookup failed", http.StatusInternalServerError)
				return
			}
			if ok && jid != "" {
				jid = normalizeWhatsAppJID(jid)
				forceSend := user.ForceSend
				status := "queued"

				if timeSvc.IsSilentMode() && !forceSend {
					fmt.Printf("🔇 webui input suppressed (silent mode) to %s web_user_id=%d\n", jid, body.WebUserID)
					status = "suppressed"
				} else if timeSvc.IsSilentMode() && forceSend {
					sendCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()
					if err := wa.Send(sendCtx, &bus.OutboundMessage{
						Channel: "whatsapp",
						ChatID:  jid,
						TraceID: traceID,
						Content: body.Message,
					}); err != nil {
						fmt.Printf("⚠️ webui input direct send error: %v\n", err)
						status = "error"
					} else {
						status = "sent"
					}
				} else {
					msgBus.PublishOutbound(&bus.OutboundMessage{
						Channel: "whatsapp",
						ChatID:  jid,
						TraceID: traceID,
						Content: body.Message,
					})
					status = "queued"
				}

				_ = timeSvc.AddEvent(&timeline.TimelineEvent{
					EventID:        fmt.Sprintf("WEBUI_INPUT_ACK_%d", time.Now().UnixNano()),
					TraceID:        traceID,
					Timestamp:      time.Now(),
					SenderID:       "AGENT",
					SenderName:     "Agent",
					EventType:      "SYSTEM",
					ContentText:    body.Message,
					Classification: fmt.Sprintf("WEBUI_INPUT_OUTBOUND->%s force=%v status=%s", jid, forceSend, status),
					Authorized:     true,
				})
			}

			// Log inbound from Web UI
			webuiInMeta, _ := json.Marshal(map[string]any{
				"channel":      "webui",
				"sender":       user.Name,
				"message_type": "TEXT",
				"content":      body.Message,
			})
			_ = timeSvc.AddEvent(&timeline.TimelineEvent{
				EventID:        fmt.Sprintf("WEBUI_IN_%d", time.Now().UnixNano()),
				TraceID:        traceID,
				Timestamp:      time.Now(),
				SenderID:       fmt.Sprintf("webui:%s", user.Name),
				SenderName:     user.Name,
				EventType:      "TEXT",
				ContentText:    body.Message,
				Classification: "WEBUI_INBOUND",
				Authorized:     true,
				Metadata:       string(webuiInMeta),
			})

			// Publish inbound to agent
			msgBus.PublishInbound(&bus.InboundMessage{
				Channel:        "webui",
				SenderID:       fmt.Sprintf("webui:%s", user.Name),
				ChatID:         fmt.Sprintf("%d", body.WebUserID),
				TraceID:        traceID,
				IdempotencyKey: "web:" + traceID,
				Content:        body.Message,
				Timestamp:      time.Now(),
				Metadata: map[string]any{
					bus.MetaKeyMessageType: bus.MessageTypeExternal,
				},
			})

			json.NewEncoder(w).Encode(map[string]string{"status": "queued"})
		})

		// API: Tasks List (GET)
		mux.HandleFunc("/api/v1/tasks", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Type", "application/json")

			status := r.URL.Query().Get("status")
			channel := r.URL.Query().Get("channel")
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			if limit == 0 {
				limit = 50
			}
			offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

			tasks, err := timeSvc.ListTasks(status, channel, limit, offset)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if tasks == nil {
				tasks = []timeline.AgentTask{}
			}
			json.NewEncoder(w).Encode(tasks)
		})

		// API: Task Detail (GET)
		mux.HandleFunc("/api/v1/tasks/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Type", "application/json")

			taskID := strings.TrimPrefix(r.URL.Path, "/api/v1/tasks/")
			taskID = strings.TrimSpace(taskID)
			if taskID == "" {
				http.Error(w, "task_id required", http.StatusBadRequest)
				return
			}

			task, err := timeSvc.GetTask(taskID)
			if err != nil {
				http.Error(w, "task not found", http.StatusNotFound)
				return
			}
			json.NewEncoder(w).Encode(task)
		})

		// API: Pending Approvals (GET)
		mux.HandleFunc("/api/v1/approvals/pending", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Type", "application/json")

			approvals, err := timeSvc.GetPendingApprovals()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if approvals == nil {
				approvals = []timeline.ApprovalRecord{}
			}
			json.NewEncoder(w).Encode(approvals)
		})

		// API: Respond to Approval (POST)
		mux.HandleFunc("/api/v1/approvals/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Type", "application/json")

			if r.Method == "OPTIONS" {
				w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
				w.WriteHeader(http.StatusOK)
				return
			}
			if r.Method != "POST" {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}

			approvalID := strings.TrimPrefix(r.URL.Path, "/api/v1/approvals/")
			approvalID = strings.TrimSpace(approvalID)
			if approvalID == "" || approvalID == "pending" {
				http.Error(w, "approval_id required", http.StatusBadRequest)
				return
			}

			var body struct {
				Approved bool `json:"approved"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid JSON body", http.StatusBadRequest)
				return
			}

			// Inject approval response into the bus
			action := "deny"
			if body.Approved {
				action = "approve"
			}
			msgBus.PublishInbound(&bus.InboundMessage{
				Channel:   "webui",
				SenderID:  "webui:admin",
				ChatID:    "approval",
				TraceID:   newTraceID(),
				Content:   fmt.Sprintf("%s:%s", action, approvalID),
				Timestamp: time.Now(),
				Metadata: map[string]any{
					bus.MetaKeyMessageType: bus.MessageTypeInternal,
				},
			})

			json.NewEncoder(w).Encode(map[string]string{"status": "sent", "approval_id": approvalID})
		})

		// Static: Media
		mediaDir := filepath.Join(cfg.Paths.Workspace, "media")
		fs := http.FileServer(http.Dir(mediaDir))
		mux.Handle("/media/", http.StripPrefix("/media/", fs))

		// SPA: Timeline
		mux.HandleFunc("/timeline", func(w http.ResponseWriter, r *http.Request) {
			serveDashboardAsset(w, "timeline.html")
		})

		// SPA: Group Management (blocked in standalone mode)
		mux.HandleFunc("/group", func(w http.ResponseWriter, r *http.Request) {
			if getMode() == "standalone" {
				http.Redirect(w, r, "/timeline", http.StatusTemporaryRedirect)
				return
			}
			serveDashboardAsset(w, "group.html")
		})

		// SPA: Approvals
		mux.HandleFunc("/approvals", func(w http.ResponseWriter, r *http.Request) {
			serveDashboardAsset(w, "approvals.html")
		})

		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" {
				serveDashboardAsset(w, "index.html")
			}
		})

		if cfg.Gateway.DashboardPort == 0 {
			cfg.Gateway.DashboardPort = 18791
		}
		addr := fmt.Sprintf("%s:%d", cfg.Gateway.Host, cfg.Gateway.DashboardPort)

		// Wrap mux with auth middleware if AuthToken is configured
		var handler http.Handler = mux
		if cfg.Gateway.AuthToken != "" {
			authToken := cfg.Gateway.AuthToken
			handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Skip auth for status endpoint (health check) and CORS preflight
				if r.URL.Path == "/api/v1/status" || r.Method == "OPTIONS" {
					mux.ServeHTTP(w, r)
					return
				}
				token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
				if token != authToken {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
				mux.ServeHTTP(w, r)
			})
			fmt.Println("🔒 Auth token required for dashboard API")
		}

		// TLS support
		if cfg.Gateway.TLSCert != "" && cfg.Gateway.TLSKey != "" {
			fmt.Printf("🖥️  Dashboard listening on https://%s\n", addr)
			cert, err := tls.LoadX509KeyPair(cfg.Gateway.TLSCert, cfg.Gateway.TLSKey)
			if err != nil {
				fmt.Printf("❌ TLS cert load failed: %v\n", err)
				cancel()
				return
			}
			server := &http.Server{
				Addr:    addr,
				Handler: handler,
				TLSConfig: &tls.Config{
					Certificates: []tls.Certificate{cert},
				},
			}
			if err := server.ListenAndServeTLS("", ""); err != nil {
				fmt.Printf("❌ Dashboard Server FAILED to start: %v\n", err)
				cancel()
			}
		} else {
			fmt.Printf("🖥️  Dashboard listening on http://%s\n", addr)
			if err := http.ListenAndServe(addr, handler); err != nil {
				fmt.Printf("❌ Dashboard Server FAILED to start: %v\n", err)
				cancel()
			}
		}
	}()

	// Start Agent Loop in background
	go func() {
		if err := loop.Run(ctx); err != nil {
			fmt.Printf("Agent loop crashed: %v\n", err)
			cancel()
		}
	}()

	// Start Orchestrator (if configured)
	if orch != nil {
		go func() {
			if err := orch.Start(ctx); err != nil {
				fmt.Printf("⚠️ Orchestrator start failed: %v\n", err)
			}
		}()
	}

	// Start Group Collaboration (if configured)
	if mgr := grpState.Manager(); mgr != nil {
		// Subscribe bus for group outbound
		setupGroupBusSubscription(mgr, msgBus)

		// Join group
		go func() {
			joinCtx, joinCancel := context.WithTimeout(ctx, 15*time.Second)
			defer joinCancel()
			if err := mgr.Join(joinCtx); err != nil {
				fmt.Printf("⚠️ Group join failed: %v\n", err)
			} else {
				fmt.Printf("🤝 Joined group: %s\n", mgr.GroupName())
			}
		}()

		// Start Kafka consumer if brokers are configured
		kafkaCancel := startGrpKafka(cfg.Group, mgr, ctx, orchDiscoveryHandler(orch))
		grpState.SetManager(mgr, kafkaCancel)
	}
	startKnowledgeAnnouncements(ctx, cfg, timeSvc)

	fmt.Println("Gateway running. Press Ctrl+C to stop.")
	<-sigChan

	fmt.Println("Shutting down...")
	// Stop orchestrator
	if orch != nil {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = orch.Stop(stopCtx)
		stopCancel()
	}
	// Leave group cleanly
	if mgr := grpState.Manager(); mgr != nil && mgr.Active() {
		leaveCtx, leaveCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = mgr.Leave(leaveCtx)
		leaveCancel()
	}
	grpState.Clear()
	wa.Stop()
	loop.Stop()
	timeSvc.Close()
}

func normalizeWhatsAppJID(jid string) string {
	jid = strings.TrimSpace(jid)
	if jid == "" {
		return jid
	}
	if strings.Contains(jid, "@") {
		return jid
	}
	// Default to user JID.
	return jid + "@s.whatsapp.net"
}

type embeddingRuntimeHealth struct {
	Ready      bool      `json:"ready"`
	Status     string    `json:"status"`
	Detail     string    `json:"detail"`
	CheckedAt  time.Time `json:"checkedAt"`
	HTTPStatus int       `json:"httpStatus,omitempty"`
}

func probeEmbeddingRuntime(cfg *config.Config) embeddingRuntimeHealth {
	out := embeddingRuntimeHealth{
		Ready:     false,
		Status:    "degraded",
		Detail:    "embedding configuration not ready",
		CheckedAt: time.Now().UTC(),
	}
	if cfg == nil {
		out.Detail = "config is nil"
		return out
	}
	if err := validateEmbeddingHardGate(cfg); err != nil {
		out.Detail = err.Error()
		return out
	}
	providerID := strings.ToLower(strings.TrimSpace(cfg.Memory.Embedding.Provider))
	if providerID != "local-hf" {
		out.Ready = true
		out.Status = "ok"
		out.Detail = "embedding provider configured (no local runtime probe required)"
		return out
	}
	endpoint := strings.TrimSpace(cfg.Memory.Embedding.Endpoint)
	if endpoint == "" {
		out.Detail = "memory.embedding.endpoint is empty"
		return out
	}
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	healthURL := strings.TrimRight(endpoint, "/") + "/healthz"
	resp, err := client.Get(healthURL)
	if err != nil {
		out.Detail = fmt.Sprintf("local embedding runtime unreachable: %v", err)
		return out
	}
	defer resp.Body.Close()
	out.HTTPStatus = resp.StatusCode
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		out.Ready = true
		out.Status = "ok"
		out.Detail = "local embedding runtime healthy"
		return out
	}
	out.Detail = fmt.Sprintf("local embedding runtime unhealthy (status=%d)", resp.StatusCode)
	return out
}

func embeddingCachePresent(cacheDir string) bool {
	p := strings.TrimSpace(cacheDir)
	if p == "" {
		return false
	}
	if strings.HasPrefix(p, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			p = filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	if fi, err := os.Stat(p); err == nil && fi.IsDir() {
		return true
	}
	return false
}

func startKnowledgeAnnouncements(ctx context.Context, cfg *config.Config, timeSvc *timeline.TimelineService) {
	if cfg == nil || timeSvc == nil || !cfg.Knowledge.Enabled {
		return
	}
	if strings.TrimSpace(cfg.Node.ClawID) == "" || strings.TrimSpace(cfg.Node.InstanceID) == "" {
		return
	}
	if strings.TrimSpace(cfg.Knowledge.Topics.Presence) == "" && strings.TrimSpace(cfg.Knowledge.Topics.Capabilities) == "" {
		return
	}
	go func() {
		_ = publishKnowledgeCapabilitiesAnnouncement(cfg, timeSvc)
		_ = publishKnowledgePresenceAnnouncement(cfg, timeSvc, "active")

		presenceTicker := time.NewTicker(45 * time.Second)
		capabilityTicker := time.NewTicker(5 * time.Minute)
		defer presenceTicker.Stop()
		defer capabilityTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				_ = publishKnowledgePresenceAnnouncement(cfg, timeSvc, "stopping")
				return
			case <-presenceTicker.C:
				_ = publishKnowledgePresenceAnnouncement(cfg, timeSvc, "active")
			case <-capabilityTicker.C:
				_ = publishKnowledgeCapabilitiesAnnouncement(cfg, timeSvc)
			}
		}
	}()
}

func publishKnowledgePresenceAnnouncement(cfg *config.Config, timeSvc *timeline.TimelineService, status string) error {
	topic := strings.TrimSpace(cfg.Knowledge.Topics.Presence)
	if topic == "" {
		return nil
	}
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "" {
		status = "active"
	}
	now := time.Now().UTC()
	env := knowledge.Envelope{
		SchemaVersion:  knowledge.CurrentSchemaVersion,
		Type:           knowledge.TypePresence,
		TraceID:        newTraceID(),
		Timestamp:      now,
		IdempotencyKey: fmt.Sprintf("knowledge:presence:%s:%s:%d", cfg.Node.ClawID, cfg.Node.InstanceID, now.Unix()),
		ClawID:         strings.TrimSpace(cfg.Node.ClawID),
		InstanceID:     strings.TrimSpace(cfg.Node.InstanceID),
		Payload: map[string]any{
			"group":       strings.TrimSpace(cfg.Knowledge.Group),
			"status":      status,
			"displayName": strings.TrimSpace(cfg.Node.DisplayName),
			"updatedAt":   now.Format(time.RFC3339),
		},
	}
	if err := publishKnowledgeEnvelope(cfg, timeSvc, topic, env); err != nil {
		return err
	}
	_ = timeSvc.SetSetting("knowledge_presence_last_at", now.Format(time.RFC3339))
	return nil
}

func publishKnowledgeCapabilitiesAnnouncement(cfg *config.Config, timeSvc *timeline.TimelineService) error {
	topic := strings.TrimSpace(cfg.Knowledge.Topics.Capabilities)
	if topic == "" {
		return nil
	}
	now := time.Now().UTC()
	env := knowledge.Envelope{
		SchemaVersion:  knowledge.CurrentSchemaVersion,
		Type:           knowledge.TypeCapabilities,
		TraceID:        newTraceID(),
		Timestamp:      now,
		IdempotencyKey: fmt.Sprintf("knowledge:capabilities:%s:%s:%d", cfg.Node.ClawID, cfg.Node.InstanceID, now.Unix()),
		ClawID:         strings.TrimSpace(cfg.Node.ClawID),
		InstanceID:     strings.TrimSpace(cfg.Node.InstanceID),
		Payload: map[string]any{
			"group":        strings.TrimSpace(cfg.Knowledge.Group),
			"displayName":  strings.TrimSpace(cfg.Node.DisplayName),
			"model":        strings.TrimSpace(cfg.Model.Name),
			"capabilities": inferNodeCapabilities(cfg),
			"updatedAt":    now.Format(time.RFC3339),
		},
	}
	if err := publishKnowledgeEnvelope(cfg, timeSvc, topic, env); err != nil {
		return err
	}
	_ = timeSvc.SetSetting("knowledge_capabilities_last_at", now.Format(time.RFC3339))
	return nil
}

func inferNodeCapabilities(cfg *config.Config) []string {
	if cfg == nil {
		return nil
	}
	out := []string{"memory.search", "memory.semantic", "knowledge.governance"}
	if cfg.Knowledge.Voting.Enabled {
		out = append(out, "knowledge.vote")
	}
	if cfg.Tools.Subagents.MaxConcurrent > 0 {
		out = append(out, "subagents")
	}
	if cfg.Channels.Slack.Enabled {
		out = append(out, "channel.slack")
	}
	if cfg.Channels.MSTeams.Enabled {
		out = append(out, "channel.msteams")
	}
	if cfg.Channels.WhatsApp.Enabled {
		out = append(out, "channel.whatsapp")
	}
	seen := map[string]struct{}{}
	filtered := make([]string, 0, len(out))
	for _, v := range out {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		filtered = append(filtered, v)
	}
	return filtered
}

func collectMemoryKnowledgeMetrics(timeSvc *timeline.TimelineService) (map[string]any, error) {
	if timeSvc == nil {
		return map[string]any{
			"status": "ok",
			"slo":    map[string]any{},
		}, nil
	}
	var totalChunks int64
	_ = timeSvc.DB().QueryRow(`SELECT COUNT(*) FROM memory_chunks`).Scan(&totalChunks)
	embeddedChunks, _ := countEmbeddedMemoryChunks()
	overflowTotal := parseSettingInt(timeSvc, "memory_overflow_events_total")

	factAccepted := countTimelineClassifications(timeSvc, "KNOWLEDGE_FACT_ACCEPTED")
	factStale := countTimelineClassifications(timeSvc, "KNOWLEDGE_FACT_STALE")
	factConflict := countTimelineClassifications(timeSvc, "KNOWLEDGE_FACT_CONFLICT")

	approved, _ := timeSvc.ListKnowledgeProposals("approved", 10000, 0)
	rejected, _ := timeSvc.ListKnowledgeProposals("rejected", 10000, 0)
	expired, _ := timeSvc.ListKnowledgeProposals("expired", 10000, 0)
	factsCount, _ := timeSvc.CountKnowledgeFacts("")

	decisionCount := len(approved) + len(rejected) + len(expired)
	precisionProxy := safeRatio(float64(len(approved)), float64(decisionCount))
	recallProxy := safeRatio(float64(factsCount), float64(maxInt(1, len(approved))))
	conflictRate := safeRatio(float64(factConflict), float64(maxInt(1, factAccepted+factStale+factConflict)))

	return map[string]any{
		"status": "ok",
		"memory": map[string]any{
			"chunksTotal":     totalChunks,
			"chunksEmbedded":  embeddedChunks,
			"overflowEvents":  overflowTotal,
			"overflowPer1000": safeRatio(float64(overflowTotal*1000), float64(maxInt64(1, totalChunks))),
		},
		"knowledge": map[string]any{
			"factsAccepted": factAccepted,
			"factsStale":    factStale,
			"factsConflict": factConflict,
			"factsLatest":   factsCount,
			"decisions": map[string]int{
				"approved": len(approved),
				"rejected": len(rejected),
				"expired":  len(expired),
			},
		},
		"slo": map[string]any{
			"precisionProxy": precisionProxy,
			"recallProxy":    recallProxy,
			"conflictRate":   conflictRate,
		},
	}, nil
}

func countTimelineClassifications(timeSvc *timeline.TimelineService, classification string) int {
	if timeSvc == nil || strings.TrimSpace(classification) == "" {
		return 0
	}
	var count int
	if err := timeSvc.DB().QueryRow(`SELECT COUNT(*) FROM timeline_events WHERE classification = ?`, classification).Scan(&count); err != nil {
		return 0
	}
	return count
}

func parseSettingInt(timeSvc *timeline.TimelineService, key string) int {
	if timeSvc == nil || strings.TrimSpace(key) == "" {
		return 0
	}
	raw, err := timeSvc.GetSetting(key)
	if err != nil {
		return 0
	}
	n, convErr := strconv.Atoi(strings.TrimSpace(raw))
	if convErr != nil || n < 0 {
		return 0
	}
	return n
}

func safeRatio(num, den float64) float64 {
	if den <= 0 {
		return 0
	}
	return num / den
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

type RepoItem struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	Type  string `json:"type"`
	Depth int    `json:"depth"`
	Size  int64  `json:"size"`
}

func listRepoTree(root, repoRoot string) ([]RepoItem, error) {
	items := []RepoItem{}
	base := repoRoot
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if path == root {
			return nil
		}
		rel, _ := filepath.Rel(base, path)
		rel = filepath.ToSlash(rel)
		depth := strings.Count(rel, "/")
		info, _ := d.Info()
		size := int64(0)
		if info != nil {
			size = info.Size()
		}
		itemType := "file"
		if d.IsDir() {
			itemType = "dir"
		}
		items = append(items, RepoItem{
			Path:  rel,
			Name:  d.Name(),
			Type:  itemType,
			Depth: depth,
			Size:  size,
		})
		return nil
	})
	return items, err
}

// gitSubcommands is the allowlist of git subcommands accepted by runGit.
var gitSubcommands = map[string]bool{
	"status": true, "branch": true, "checkout": true, "log": true,
	"diff": true, "add": true, "commit": true, "pull": true,
	"push": true, "remote": true, "init": true,
}

// safeGitArg matches characters safe for git arguments.
var safeGitArg = regexp.MustCompile(`^[a-zA-Z0-9_./:@=, +\-~^]+$`)

func runGit(repo string, args ...string) (string, error) {
	if repo == "" {
		return "", fmt.Errorf("work repo not configured")
	}
	if len(args) == 0 || !gitSubcommands[args[0]] {
		return "", fmt.Errorf("git subcommand not allowed: %v", args)
	}
	for _, a := range args[1:] {
		if !safeGitArg.MatchString(a) {
			return "", fmt.Errorf("git arg contains unsafe characters: %q", a)
		}
	}
	gitBin, err := exec.LookPath("git")
	if err != nil {
		return "", fmt.Errorf("git not found: %w", err)
	}
	cmd := &exec.Cmd{
		Path: gitBin,
		Args: append([]string{gitBin}, args...),
		Dir:  repo,
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s failed: %s", args[0], strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func runGh(repo string, args ...string) (string, error) {
	if repo == "" {
		return "", fmt.Errorf("work repo not configured")
	}
	cmd := exec.Command("gh", args...)
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gh %s failed: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func isWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".."
}

// orchDiscoveryHandler builds an OrchestratorHandler that routes envelope payloads
// to the orchestrator's HandleDiscovery. Returns nil if orch is nil.
func orchDiscoveryHandler(orch *orchestrator.Orchestrator) group.OrchestratorHandler {
	if orch == nil {
		return nil
	}
	return func(env *group.GroupEnvelope) {
		data, err := json.Marshal(env.Payload)
		if err != nil {
			return
		}
		var payload orchestrator.DiscoveryPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			return
		}
		orch.HandleDiscovery(payload)
	}
}

// groupState manages the lifecycle of the group manager at runtime.
type groupState struct {
	mu       sync.RWMutex
	mgr      *group.Manager
	consumer group.Consumer
	cancel   context.CancelFunc // cancels Kafka consumer goroutine
}

func (gs *groupState) Manager() *group.Manager {
	gs.mu.RLock()
	defer gs.mu.RUnlock()
	return gs.mgr
}

func (gs *groupState) Consumer() group.Consumer {
	gs.mu.RLock()
	defer gs.mu.RUnlock()
	return gs.consumer
}

func (gs *groupState) SetManager(mgr *group.Manager, cancel context.CancelFunc) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	gs.mgr = mgr
	gs.cancel = cancel
}

func (gs *groupState) SetConsumer(c group.Consumer) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	gs.consumer = c
}

func (gs *groupState) Clear() {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	if gs.cancel != nil {
		gs.cancel()
	}
	gs.mgr = nil
	gs.consumer = nil
	gs.cancel = nil
}

// groupTraceAdapter adapts group.Manager to the agent.GroupTracePublisher interface.
type groupTraceAdapter struct {
	mgr *group.Manager
}

func (a *groupTraceAdapter) Active() bool {
	return a.mgr.Active()
}

func maskSecret(s string) string {
	if len(s) <= 4 {
		return strings.Repeat("*", len(s))
	}
	return s[:2] + strings.Repeat("*", len(s)-4) + s[len(s)-2:]
}

func (a *groupTraceAdapter) PublishTrace(ctx context.Context, payload interface{}) error {
	// Accept either TracePayload or map[string]string
	switch p := payload.(type) {
	case group.TracePayload:
		return a.mgr.PublishTrace(ctx, p)
	case map[string]string:
		tp := group.TracePayload{
			TraceID:  p["trace_id"],
			SpanType: p["span_type"],
			Title:    p["title"],
			Content:  p["content"],
		}
		return a.mgr.PublishTrace(ctx, tp)
	default:
		return fmt.Errorf("unsupported trace payload type: %T", payload)
	}
}

func (a *groupTraceAdapter) PublishAudit(ctx context.Context, eventType, traceID, detail string) error {
	return a.mgr.PublishAudit(ctx, eventType, traceID, detail)
}

// inferTopicCategory guesses the category from a topic name when the manifest is incomplete.
func inferTopicCategory(name string) string {
	switch {
	case strings.Contains(name, ".control.") || strings.Contains(name, ".announce") ||
		strings.Contains(name, ".roster") || strings.Contains(name, ".onboarding") ||
		strings.Contains(name, ".orchestrator"):
		return "control"
	case strings.Contains(name, ".observe.") || strings.Contains(name, ".traces") ||
		strings.Contains(name, ".audit"):
		return "observe"
	case strings.Contains(name, ".tasks.") || strings.Contains(name, ".requests") ||
		strings.Contains(name, ".responses"):
		return "tasks"
	case strings.Contains(name, ".memory."):
		return "memory"
	case strings.Contains(name, ".skill."):
		return "skill"
	default:
		return "control"
	}
}

func collectKnowledgeTopics(cfg *config.Config) []string {
	if cfg == nil || !cfg.Knowledge.Enabled {
		return nil
	}
	topics := []string{
		strings.TrimSpace(cfg.Knowledge.Topics.Capabilities),
		strings.TrimSpace(cfg.Knowledge.Topics.Presence),
		strings.TrimSpace(cfg.Knowledge.Topics.Proposals),
		strings.TrimSpace(cfg.Knowledge.Topics.Votes),
		strings.TrimSpace(cfg.Knowledge.Topics.Decisions),
		strings.TrimSpace(cfg.Knowledge.Topics.Facts),
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(topics))
	for _, t := range topics {
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}
