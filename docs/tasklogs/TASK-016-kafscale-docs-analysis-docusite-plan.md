# TASK-016: KafScale.io Docs Analysis & KafClaw Docusite Plan

**Date:** 2026-02-18
**Status:** Done

---

## 1. KafScale.io — Analysis

### 1.1 Website Structure

The [KafScale.io](https://kafscale.io/) site serves as the public-facing documentation and marketing site for the KafScale platform. Discovered pages:

| URL Path | Title | Purpose |
|----------|-------|---------|
| `/` | KafScale — Stateless Kafka on S3 | Landing / hero page |
| `/architecture/` | Architecture | System design, write/read paths, caching |
| `/rationale/` | Rationale | Why separation of compute and storage matters |
| `/comparison/` | Comparison | KafScale vs alternatives |
| `/processors/iceberg/` | Iceberg Processor | Addon documentation |

Clean URL pattern with trailing slashes — consistent with Hugo, Docusaurus, or similar SSG output.

### 1.2 Where the Site Lives

The kafscale.io documentation site is **not** in the [KafScale/platform](https://github.com/KafScale/platform) GitHub repo. Evidence:

- **No SSG config** in the repo (no `hugo.toml`, `docusaurus.config.js`, `mkdocs.yml`, etc.)
- **No docs-build CI workflow** — only `ci.yml`, `docker.yml`, `fuzz.yml`, `scorecard.yml`
- **No Makefile target** for docs generation
- **`ui/public/`** contains an admin dashboard ("Kafscale Console"), not the website
- **`docs/`** contains raw markdown (no frontmatter, no SSG metadata)
- **No separate docs repo** in the [KafScale GitHub org](https://github.com/orgs/KafScale/repositories) (5 repos: platform, k6suite, kaf-mirror, onboarding-tutorials, discussions)

The site is likely maintained privately by the NovaTechflow/Scalytics team, or deployed from a private repo.

### 1.3 Content Architecture in the Repo

The `docs/` directory in KafScale/platform contains **16 markdown files** + 3 subdirs:

```
docs/
├── .gitkeep
├── architecture.md      # 3.5 KB — Components, write/read paths, caching, multi-region
├── development.md        # Build/test workflows
├── mcp.md                # MCP integration
├── metrics.md            # Observability metrics
├── operations.md         # etcd/S3 HA, backup
├── ops-api.md            # Ops API reference
├── ops-mcp.md            # Ops MCP reference
├── overview.md           # Platform overview, design priorities, non-goals
├── protocol.md           # Kafka protocol compatibility
├── quickstart.md         # Helm-based K8s installation guide
├── roadmap.md            # Feature roadmap
├── security.md           # Security approach
├── storage.md            # S3 storage format details
├── supported.md          # Supported Kafka APIs
├── user-guide.md         # Runtime behavior, limits
├── benchmarks/           # Performance benchmark data
├── grafana/              # Monitoring dashboards
└── releases/             # Release notes
```

Key observations:
- **No frontmatter** on any docs — pure markdown, no `title:`, `weight:`, `slug:` YAML
- **No sidebar config** — no `_sidebar.md`, `sidebars.js`, or `mkdocs.yml` nav tree
- Content is written for developers/operators, not generated from code
- Docs reference each other via relative paths (`docs/operations.md`)

### 1.4 What Can We Learn

Even though the exact framework behind kafscale.io isn't confirmed, the **content strategy** is clear and worth emulating:

1. **Separation of concerns** — Landing page, architecture deep-dive, rationale/philosophy, comparisons, and addon-specific docs each get their own page
2. **Progressive disclosure** — Overview → Architecture → Detailed operations/security
3. **Clean URL hierarchy** — `/architecture/`, `/processors/iceberg/` (nested for addons)
4. **Concise pages** — Each page has a focused scope; architecture is ~3.5 KB, not a monolith
5. **Repo docs ≠ site docs** — Raw markdown in repo serves developers; the polished site serves evaluators and operators

---

## 2. KafClaw — Current Documentation State

### 2.1 Inventory

KafClaw has **~185 KB of documentation** across **24 markdown files**:

| Location | Files | Purpose |
|----------|-------|---------|
| `docs/v2/` | 13 guides | Architecture, admin, ops, user manual, security, deployment |
| `docs/bugs/` | 3 reports | BUG-001 through BUG-003 |
| `docs/tasklogs/` | 5 logs | TASK-001 through TASK-015 |
| `docs/security/` | 2 files | Risk evaluation, security roadmap |
| `gomikrobot/ARCHITECTURE.md` | 1 (31 KB) | Comprehensive system reference (1031 lines) |
| `gomikrobot/MEMORY.md` | 1 | Memory context notes |
| Root `README.md` | 1 (11 KB) | Project overview |
| Root `CLAUDE.md` | 1 (8 KB) | Claude Code guidance |

### 2.2 Content Quality

**Strengths:**
- `gomikrobot/ARCHITECTURE.md` is a 31 KB masterpiece covering the entire system
- `docs/v2/architecture-detailed.md` (27 KB) is equally comprehensive
- Clear guide separation: admin, operations, user manual, security
- Established bug/tasklog conventions (BUG-xxx, TASK-xxx)
- ASCII diagrams, code snippets, tables throughout

**Gaps for a public docs site:**
- **No navigation structure** — flat list of files, no hierarchy or ordering
- **No frontmatter** — no titles, descriptions, ordering metadata
- **Monolithic references** — 27-31 KB files are great for devs, too dense for a docs site
- **No landing page** — no hero, no "what is KafClaw", no getting-started funnel
- **No search** — GitHub native rendering only
- **No versioning** — v2/ directory exists but no version switcher
- **Duplicate content** — `ARCHITECTURE.md` and `architecture-detailed.md` overlap significantly

### 2.3 Documentation Tooling

**None configured.** All documentation is static markdown rendered by GitHub.

---

## 3. Docusite Subproject Plan

### 3.1 Recommended Framework: Hugo

| Criterion | Hugo | Docusaurus | VitePress | MkDocs Material |
|-----------|------|------------|-----------|-----------------|
| Language ecosystem | Go (matches KafClaw) | Node/React | Node/Vue | Python |
| Build speed | ~50ms for 100 pages | ~10s | ~3s | ~5s |
| Markdown-first | Yes | Yes | Yes | Yes |
| No runtime deps at deploy | Yes (single binary) | Yes (static output) | Yes | Yes |
| Learning curve for Go devs | Low | Medium | Medium | Medium |
| Theme ecosystem | Excellent (Doks, Hextra, etc.) | Good | Good | Excellent |
| Versioning | Manual/branch | Built-in | Manual | mike plugin |
| Search | Built-in (Pagefind, Fuse.js) | Built-in (Algolia) | Built-in (MiniSearch) | Built-in |
| i18n | Built-in | Built-in | Built-in | Plugin |

**Why Hugo:**
- KafClaw is a Go project — Hugo is the Go-native SSG, single binary, no Node/Python deps
- KafScale itself is Go-based — its docs site likely uses Hugo (consistent ecosystem)
- Build times are instant even for large sites
- The [Hextra](https://imfing.github.io/hextra/) theme provides a modern docs experience (search, dark mode, sidebar, responsive) out of the box
- Content can be plain markdown — existing docs migrate with minimal changes
- Can be installed via `go install` or Homebrew, consistent with Go toolchain

### 3.2 Recommended Theme: Hextra

[Hextra](https://imfing.github.io/hextra/) is a Hugo theme purpose-built for documentation:
- Full-text search (FlexSearch)
- Dark/light mode
- Sidebar navigation from directory structure
- Syntax highlighting
- Responsive design
- Callout/admonition blocks
- Mermaid diagram support
- No Node.js dependency

### 3.3 Proposed Directory Structure

```
KafClaw/
├── docsite/                          # NEW — Hugo documentation site
│   ├── hugo.toml                     # Hugo configuration
│   ├── go.mod                        # Hugo module (for Hextra theme)
│   ├── go.sum
│   ├── .hugo_build.lock
│   ├── Makefile                      # docs-specific: serve, build, deploy
│   ├── content/                      # Markdown content (Hugo convention)
│   │   ├── _index.md                 # Landing page (hero, features, CTA)
│   │   ├── docs/                     # Documentation section
│   │   │   ├── _index.md             # Docs landing / overview
│   │   │   ├── getting-started/
│   │   │   │   ├── _index.md         # Getting started overview
│   │   │   │   ├── installation.md   # Build from source, make install
│   │   │   │   ├── quickstart.md     # First run, onboard, send message
│   │   │   │   └── configuration.md  # Config file, env vars, defaults
│   │   │   ├── architecture/
│   │   │   │   ├── _index.md         # Architecture overview (from architecture.md)
│   │   │   │   ├── agent-loop.md     # Agent loop deep-dive
│   │   │   │   ├── message-bus.md    # Pub-sub bus design
│   │   │   │   ├── memory.md         # 6-layer memory architecture
│   │   │   │   ├── timeline.md       # SQLite timeline DB
│   │   │   │   └── three-repos.md    # Identity/Work/System model
│   │   │   ├── guides/
│   │   │   │   ├── _index.md
│   │   │   │   ├── admin.md          # From admin-guide.md
│   │   │   │   ├── operations.md     # From operations-guide.md
│   │   │   │   ├── whatsapp.md       # From whatsapp-setup.md
│   │   │   │   ├── docker.md         # From docker-deployment.md
│   │   │   │   └── workspace.md      # From workspace-policy.md
│   │   │   ├── reference/
│   │   │   │   ├── _index.md
│   │   │   │   ├── cli.md            # CLI command reference
│   │   │   │   ├── api.md            # HTTP API reference (ports 18790/18791)
│   │   │   │   ├── tools.md          # Tool registry reference
│   │   │   │   └── config.md         # Full config struct reference
│   │   │   ├── security/
│   │   │   │   ├── _index.md
│   │   │   │   ├── model.md          # Security model (3-tier auth)
│   │   │   │   ├── risks.md          # Threat model, mitigations
│   │   │   │   └── roadmap.md        # Security roadmap
│   │   │   └── extending/
│   │   │       ├── _index.md
│   │   │       ├── tools.md          # Adding new tools
│   │   │       ├── channels.md       # Adding new channels
│   │   │       └── commands.md       # Adding CLI commands
│   │   ├── blog/                     # Optional: release announcements, devlogs
│   │   │   └── _index.md
│   │   └── changelog/                # Release history
│   │       └── _index.md
│   ├── static/                       # Static assets (images, diagrams)
│   │   └── images/
│   ├── layouts/                      # Custom layout overrides (if needed)
│   └── assets/                       # Custom CSS/JS overrides (if needed)
├── docs/                             # KEEP — operational docs (bugs, tasklogs, security)
├── gomikrobot/                       # KEEP — source code
└── ...
```

### 3.4 Content Migration Plan

Existing docs would be restructured, not copied verbatim. The monolithic files need splitting:

| Source | Target(s) | Action |
|--------|-----------|--------|
| `gomikrobot/ARCHITECTURE.md` (31 KB) | `architecture/*` (5-6 pages) | Split by section |
| `docs/v2/architecture-detailed.md` (27 KB) | Merge into architecture/* | Deduplicate with above |
| `docs/v2/architecture.md` (3.4 KB) | `architecture/_index.md` | Use as overview |
| `docs/v2/user-manual.md` (12 KB) | `getting-started/*` + `reference/cli.md` | Split |
| `docs/v2/admin-guide.md` (17 KB) | `guides/admin.md` + `reference/config.md` | Split |
| `docs/v2/operations-guide.md` (14 KB) | `guides/operations.md` + `reference/api.md` | Split |
| `docs/v2/security-risks.md` | `security/risks.md` | Add frontmatter |
| `docs/v2/whatsapp-setup.md` | `guides/whatsapp.md` | Add frontmatter |
| `docs/v2/docker-deployment.md` | `guides/docker.md` | Add frontmatter |
| `docs/v2/workspace-policy.md` | `guides/workspace.md` | Add frontmatter |
| `docs/v2/release.md` | `changelog/_index.md` | Reformat |
| `README.md` | `content/_index.md` (landing) | Rewrite as hero page |
| `docs/security/*` | `security/*` | Add frontmatter |
| `docs/bugs/*`, `docs/tasklogs/*` | **Stay in `docs/`** | Not part of docsite |

### 3.5 Hugo Configuration Skeleton

```toml
# docsite/hugo.toml

baseURL = "https://kafclaw.io/"  # or GitHub Pages URL
title = "KafClaw"
languageCode = "en"
defaultContentLanguage = "en"

[module]
  [[module.imports]]
    path = "github.com/imfing/hextra"

[markup.goldmark.renderer]
  unsafe = true  # allow raw HTML in markdown

[markup.highlight]
  noClasses = false

[params]
  description = "Multi-agent AI assistant framework — Go, Kafka-style coordination, distributed brain"

  [params.navbar]
    displayTitle = true
    displayLogo = true

  [params.footer]
    displayPoweredBy = false

  [params.docs]
    sidebar = true
    toc = true
    breadcrumb = true

  [params.search]
    enable = true
    type = "flexsearch"
```

### 3.6 Makefile for the Docsite

```makefile
# docsite/Makefile

.PHONY: serve build clean

# Local dev server with live reload
serve:
	hugo server --buildDrafts --port 1313

# Production build
build:
	hugo --minify

# Clean build artifacts
clean:
	rm -rf public/

# Create new content page
new:
	@read -p "Path (e.g. docs/guides/new-page.md): " path; \
	hugo new content/$$path
```

### 3.7 Implementation Phases

**Phase 1 — Scaffold (1 session)**
- Initialize `docsite/` with Hugo + Hextra theme
- Create `hugo.toml`, `go.mod`, `Makefile`
- Build empty site skeleton with placeholder `_index.md` files
- Verify `hugo server` runs locally

**Phase 2 — Core Content Migration (2-3 sessions)**
- Migrate getting-started (installation, quickstart, configuration)
- Migrate architecture section (split the monoliths)
- Migrate guides (admin, operations, whatsapp, docker)
- Add frontmatter to all pages (title, weight, description)

**Phase 3 — Reference & Polish (1-2 sessions)**
- Migrate reference docs (CLI, API, tools, config)
- Migrate security docs
- Add the landing page with hero section
- Add custom CSS if needed for branding

**Phase 4 — Deploy (1 session)**
- Add GitHub Actions workflow for building and deploying
- Options: GitHub Pages, Cloudflare Pages, or Netlify (all free for OSS)
- Add to CI: build docs on PR to catch broken links

### 3.8 What Stays in `docs/`

The existing `docs/` directory keeps its role for **operational records**:
- `docs/bugs/` — Bug reports (BUG-xxx)
- `docs/tasklogs/` — Task completion logs (TASK-xxx)
- `docs/security/` — Security evaluations
- `docs/v2/` — Source-of-truth markdown (docsite content derives from these)

The `docsite/` is a **presentation layer** over the project's documentation, not a replacement for the operational docs workflow.

---

## 4. Key Takeaways from KafScale.io

1. **Separate the docs site from raw repo docs** — KafScale does this; we should too
2. **Keep pages focused** — One topic per page, not 30 KB monoliths
3. **Progressive disclosure** — Landing → Overview → Deep-dive → Reference
4. **Go-native tooling** — Hugo fits the Go ecosystem without adding Node/Python deps
5. **Content-first** — KafScale's repo docs have zero SSG configuration; the site is a layer on top

---

## 5. Commits

- Initial analysis and plan created in this task log
