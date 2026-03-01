# Reasoning Tools MCP Server v3.3

An MCP (Model Context Protocol) server that provides advanced reasoning tools using configurable LLM backends.

## What's New in v3.3

- **Auto-Reasoning**: New `auto_reason` tool automatically selects the best algorithm for your problem
- **Provider Intelligence**: Tracks provider metrics, scores, and smart fallback
- **YAML Configuration**: Configure defaults via `reasoning-tools.yaml` file
- **Enhanced Reflexion Memory**: Uses semantic similarity for finding relevant past lessons
- **Better Error Handling**: Categorized errors (config/transient/terminal) with smart retry
- **Configurable GoT Merging**: Similarity threshold for path merging is now configurable

## Architecture

```
┌─────────────┐      ┌──────────────────────┐      ┌─────────────────┐
│ Main Client │ ───► │ This MCP Server      │ ───► │ LLM Provider    │
│ (Claude)    │      │                      │      │ (zai/openai/    │
└─────────────┘      │ • auto_reason (NEW!) │      │  groq/deepseek/ │
                     │ • sequential_thinking │      │  ollama/etc)    │
                     │ • graph_of_thoughts  │      └─────────────────┘
                     │ • reflexion          │           │
                     │ • dialectic_reason   │    ┌──────┴──────┐
                     │ • list_providers     │    │ Provider    │
                     │ • memory_stats       │    │ Intelligence│
                     └──────────────────────┘    └─────────────┘
                            │
                     ┌──────┴──────┐      ┌─────────────────┐
                     │ YAML Config │      │ Built-in Tools  │
                     │ (optional)  │      │ • calculator    │
                     └─────────────┘      │ • code_exec     │
                                          │ • web_fetch     │
                                          │ • string_ops    │
                                          └─────────────────┘
```

## Tools Available

### 1. `auto_reason` (NEW in v3.3)
Automatically selects and runs the best reasoning algorithm for your problem. Uses either quick heuristic-based selection or LLM-powered analysis to choose between sequential, graph-of-thoughts, reflexion, or dialectic reasoning.

```json
{
  "problem": "What are the pros and cons of microservices vs monoliths?",
  "quick_select": false
}
```

**Key Features:**
- **Quick Select Mode**: Fast keyword-based selection using patterns (debate→dialectic, puzzle→reflexion, brainstorm→GoT)
- **LLM Analysis Mode**: Deep problem analysis considering complexity, controversy, exploration needs
- **Automatic Execution**: Runs the selected algorithm and returns results
- **Selection Transparency**: Logs which algorithm was chosen and why

**Selection Criteria:**
| Problem Type | Algorithm Selected |
|--------------|-------------------|
| Controversial/debate topics | Dialectic |
| Multiple perspectives | Dialectic |
| Puzzles, debugging, iteration needed | Reflexion |
| Creative exploration, brainstorming | Graph of Thoughts |
| Simple, factual questions | Sequential |

### 2. `sequential_thinking`
Simple linear chain-of-thought reasoning. Good for straightforward problems.

```json
{
  "problem": "What is 17 * 23?",
  "max_thoughts": 5,
  "stream": false
}
```

### 3. `graph_of_thoughts`
Graph-based reasoning with path merging and optional tool integration. Unlike Tree of Thoughts, GoT can merge similar reasoning paths, combining insights from converging approaches.

```json
{
  "problem": "Design a cache invalidation strategy for a distributed system",
  "branching_factor": 3,
  "max_nodes": 30,
  "max_depth": 8,
  "enable_merging": true,
  "enable_tools": true,
  "max_tool_calls": 10,
  "enabled_tools": "calculator,code_exec",
  "stream": true
}
```

**Key Features:**
- Nodes can have multiple parents
- Similar thoughts are merged using LLM similarity detection
- Merged nodes get score boosts (converging evidence)
- UCB1 formula guides exploration vs exploitation
- **Tool Integration (v3.2)**: Can use calculator, code execution, and web fetch during reasoning

### 4. `reflexion`
Reasoning with episodic memory and optional tool integration. Makes multiple attempts, learns from failures, and applies lessons from past similar problems.

```json
{
  "problem": "Solve: What is the next number in the sequence 2, 6, 12, 20, 30, ?",
  "max_attempts": 3,
  "learn_from_past": true,
  "enable_tools": true,
  "max_tool_calls": 5,
  "stream": true
}
```

**Key Features:**
- Stores lessons in persistent memory (`~/.local/share/reasoning-tools/memory.json`)
- Failed attempts trigger reflection: "What went wrong?"
- Future similar problems query past lessons
- Lessons inform new reasoning attempts
- **Tool Integration (v3.2)**: Can use tools during reasoning for computation and verification

### 5. `dialectic_reason`
Thesis-antithesis-synthesis reasoning combining Debate and Chain of Verification with optional tool-backed fact-checking.

```json
{
  "problem": "Should companies adopt a 4-day work week?",
  "max_rounds": 3,
  "confidence_target": 0.85,
  "enable_tools": true,
  "max_tool_calls": 10,
  "enabled_tools": "calculator,web_fetch",
  "stream": true
}
```

**Key Features:**
- Thesis: Propose a solution
- Antithesis: Challenge the thesis (find flaws)
- Synthesis: Integrate valid points from both
- Each claim is verified for logical soundness
- **Tool-Backed Verification (v3.2)**: Uses tools to fact-check claims during verification

### 6. `list_providers`
List available providers and their configuration status. Now includes **Provider Intelligence** metrics showing performance scores, success rates, and latency.

### 7. `memory_stats`
Show reflexion episodic memory statistics.

## Built-in Tools

When `enable_tools: true` is set, reasoning methods can use these tools:

| Tool | Description | Example |
|------|-------------|---------|
| `calculator` | Math expressions | `17 * 23`, `sqrt(144)`, `sin(pi/4)` |
| `code_exec` | Python code execution | `print(sum([1,2,3]))` |
| `web_fetch` | URL fetch / web search | `https://api.github.com/users/...` |
| `string_ops` | String operations | `len:hello`, `upper:text` |

## Streaming Output

All reasoning tools support streaming output via the `stream: true` parameter:

```
💭 [t1] (d1) (0.85) First reasoning step...
💭 [t2] (d2) (0.78) Second reasoning step...
🔀 Merged thought into existing node n5
🔧 calculator(17*23) → 391
📊 [n7] (0.92) Evaluation complete
✅ Solution found!
```

## Supported Providers

| Provider | Env Key | Default Model | Notes |
|----------|---------|---------------|-------|
| **zai** (GLM) | `ZAI_API_KEY` | glm-4.7 | z.ai/Zhipu |
| **openai** | `OPENAI_API_KEY` | gpt-4o-mini | |
| **anthropic** | `ANTHROPIC_API_KEY` | claude-sonnet-4-6 | |
| **groq** | `GROQ_API_KEY` | llama-3.1-70b | Very fast |
| **deepseek** | `DEEPSEEK_API_KEY` | deepseek-chat | Cheap, good reasoning |
| **openrouter** | `OPENROUTER_API_KEY` | llama-3.1-70b | Many models |
| **together** | `TOGETHER_API_KEY` | llama-3.1-70b | |
| **ollama** | (none) | llama3.1 | Local |

## Setup

### 1. Build

```bash
cd /path/to/reasoning-tools
go build -o reasoning-tools .
cp reasoning-tools ~/.local/bin/
```

### 2. Environment Variables

```bash
# Set at least one provider key
export ZAI_API_KEY="your-key"        # or
export GROQ_API_KEY="your-key"       # or
export DEEPSEEK_API_KEY="your-key"   # etc.

# Optional overrides
export LLM_PROVIDER="groq"           # Force specific provider
export LLM_MODEL="mixtral-8x7b"      # Force specific model
export ZAI_BASE_URL="..."            # Custom endpoint for z.ai
```

### 2.5 Configuration File (Optional)

Create `reasoning-tools.yaml` in the current directory, `~/.config/reasoning-tools/`, or set `REASONING_TOOLS_CONFIG` to point to your config file.

```yaml
# reasoning-tools.yaml
providers:
  default: "groq"                    # Default provider if no env var set
  fallbacks:                         # Fallback order on failure
    - "deepseek"
    - "ollama"

algorithms:
  sequential:
    max_thoughts: 10
  graph_of_thoughts:
    branching_factor: 3
    max_nodes: 30
    max_depth: 8
    enable_merging: true
    merge_threshold: 0.7             # NEW: Configurable similarity threshold
  reflexion:
    max_attempts: 3
  dialectic:
    max_rounds: 5
    confidence_target: 0.85
    fast_mode: false

timeouts:                            # Per-provider timeouts (seconds)
  openai: 120
  anthropic: 120
  groq: 60
  ollama: 300
  deepseek: 120

rate_limiting:
  max_concurrent: 2                  # Max parallel LLM requests
  max_tokens_cap: 2048               # Default max tokens per request

memory:
  path: ""                           # Custom memory path (default: ~/.local/share/...)
  max_episodes: 100                  # Max episodes in reflexion memory
  ttl_days: 7                        # Episode expiration

tools:
  calculator: true
  code_exec: true
  web_fetch: true
  string_ops: true
```

Generate an example config:
```bash
reasoning-tools -generate-config > reasoning-tools.yaml
```

**Priority Order**: Environment variables override config file values.

### 3. MCP Configuration

Most MCP desktop/CLI clients use stdio, so pass `-transport=stdio` in their args.

**Codex** (`~/.codex/config.toml`):
```toml
[mcp_servers.reasoning-tools]
command = "reasoning-tools"
args = ["-transport=stdio"]
disabled = false

[mcp_servers.reasoning-tools.env]
ZAI_API_KEY = "your-key"
ZAI_BASE_URL = "https://api.z.ai/api/paas/v4"
GLM_MODEL = "glm-4.7"
```

**OpenCode** (`~/.config/opencode/opencode.json`):
```json
{
  "mcp": {
    "reasoning-tools": {
      "type": "local",
      "command": ["reasoning-tools"],
      "args": ["-transport=stdio"],
      "environment": {
        "ZAI_API_KEY": "your-key",
        "ZAI_BASE_URL": "https://api.z.ai/api/paas/v4"
      }
    }
  }
}
```

**Claude Code** (`.mcp.json` in project root):
```json
{
  "mcpServers": {
    "reasoning-tools": {
      "command": "reasoning-tools",
      "args": ["-transport=stdio"],
      "env": {
        "ZAI_API_KEY": "your-key",
        "ZAI_BASE_URL": "https://api.z.ai/api/paas/v4"
      }
    }
  }
}
```

Or add via CLI:
```bash
claude mcp add reasoning-tools -- reasoning-tools -transport=stdio
```

**Gemini CLI** (`~/.gemini/settings.json`):
```json
{
  "mcpServers": {
    "reasoning-tools": {
      "command": "reasoning-tools",
      "args": ["-transport=stdio"],
      "env": {
        "ZAI_API_KEY": "your-key",
        "ZAI_BASE_URL": "https://api.z.ai/api/paas/v4"
      }
    }
  }
}
```

Or for project-specific config (`.gemini/settings.json`):
```json
{
  "mcpServers": {
    "reasoning-tools": {
      "command": "reasoning-tools",
      "args": ["-transport=stdio"],
      "env": {
        "ZAI_API_KEY": "your-key",
        "ZAI_BASE_URL": "https://api.z.ai/api/paas/v4"
      }
    }
  }
}
```

### 4. Transports

This server supports three transports (default: **sse**), plus a dual mode:
- **stdio** (use `-transport=stdio` for stdio-based clients)
- **sse** (Server-Sent Events; `/sse` + `/message`)
- **streamable-http** (single endpoint, default `/mcp`)
- **dual** (SSE + streamable-http on the same port)

When `-transport` is not provided and stdin/stdout are non-interactive, the server auto-selects **stdio** to support stdio-based MCP clients. Set `-transport` or `MCP_TRANSPORT` to override.

Examples:

```bash
# SSE
./reasoning-tools -transport=sse -port=9847 -base-url http://localhost:9847

# Streamable HTTP
./reasoning-tools -transport=streamable-http -port=9847 -http-path /mcp

# Dual (SSE + Streamable HTTP)
./reasoning-tools -transport=dual -port=9847 -base-url http://localhost:9847 -http-path /mcp
```

Environment overrides:

```bash
export MCP_TRANSPORT=sse
export MCP_PORT=9847
export MCP_BASE_URL="http://localhost:9847"   # SSE only
export MCP_HTTP_PATH="/mcp"                   # Streamable HTTP only
```

## Algorithm Details

### Graph of Thoughts (GoT)
```
         Problem
            │
    ┌───────┼───────┐
    ▼       ▼       ▼
  Path A  Path B  Path C   ← Generate 3 candidates
  (0.8)   (0.6)   (0.9)    ← Score each
    │       │       │
    └───────┼───────┘       ← Merge similar paths
            │
    ┌───────┼───────┐
    ...continue...
```

With tools enabled, nodes can be tool actions:
```
    [thought] ──► [tool:calc] ──► [thought] ──► [answer]
                      │
                  Result: 391
```

### Reflexion
```
Attempt 1 → Fail → Reflect → Store lesson
     ↓
Attempt 2 → Apply lesson → Fail → Reflect → Store
     ↓
Attempt 3 → Apply lessons → Success!
     ↓
Future similar problems → Query lessons → Better first attempt
```

With tools enabled, reasoning can use calculator/code/web during each attempt.

### Dialectical Reasoning
```
Round 1:
  Thesis (propose) → Verify [+ tool evidence]
  Antithesis (challenge) → Verify [+ tool evidence]
  Synthesis (integrate) → Verify [+ tool evidence]
     ↓
Round 2: Build on synthesis...
     ↓
Continue until confidence >= target
```

Tool-backed verification gathers evidence using calculator, web_fetch, etc.

## Config Parameters

### Graph of Thoughts
| Param | Default | Description |
|-------|---------|-------------|
| `branching_factor` | 3 | Candidates per expansion |
| `max_nodes` | 30 | Maximum nodes to explore |
| `max_depth` | 8 | Maximum reasoning depth |
| `enable_merging` | true | Allow path merging |
| `enable_tools` | false | Enable tool usage during reasoning |
| `max_tool_calls` | 10 | Maximum tool calls |
| `enabled_tools` | (all) | Comma-separated: calculator,code_exec,web_fetch,string_ops |

### Reflexion
| Param | Default | Description |
|-------|---------|-------------|
| `max_attempts` | 3 | Maximum reasoning attempts |
| `learn_from_past` | true | Query episodic memory |
| `enable_tools` | false | Enable tool usage during reasoning |
| `max_tool_calls` | 5 | Maximum tool calls per attempt |
| `enabled_tools` | (all) | Comma-separated: calculator,code_exec,web_fetch,string_ops |

### Dialectical Reasoning
| Param | Default | Description |
|-------|---------|-------------|
| `max_rounds` | 5 | Maximum debate rounds |
| `confidence_target` | 0.85 | Stop when reached |
| `enable_tools` | false | Enable tool-backed verification |
| `max_tool_calls` | 10 | Maximum tool calls for verification |
| `enabled_tools` | (all) | Comma-separated: calculator,code_exec,web_fetch,string_ops |

## Provider Intelligence (NEW in v3.3)

The server tracks provider performance metrics and provides smart fallback:

**Metrics Tracked:**
- Success/failure rates
- Average latency
- Rate limit hits
- Consecutive failures

**Scoring Algorithm:**
- Base: 100 points
- Success rate penalty: up to -40 points
- Latency penalty: up to -30 points (if avg > 1000ms)
- Rate limit penalty: up to -15 points
- Recency bonus: up to +15 points for recent success

**Smart Fallback:**
When a provider fails, the system:
1. Checks if error is retryable (transient vs terminal)
2. Tries configured fallback providers in order
3. Falls back to highest-scoring available provider

After 3 consecutive failures, a provider is marked unavailable until reset.

View metrics via `list_providers`:
```
Provider Intelligence Summary:
============================

groq (Score: 95.0, available)
  Requests: 50 total, 49 success (98.0%)
  Latency: avg 120ms, min 80ms, max 450ms

openai (Score: 85.0, available)
  Requests: 30 total, 28 success (93.3%)
  Latency: avg 350ms, min 200ms, max 800ms
```

## Error Categorization (v3.3)

Errors are now categorized for better handling:

| Category | Retryable | Examples |
|----------|-----------|----------|
| **Config** | No | Invalid API key, model not found |
| **Transient** | Yes | Rate limit, timeout, 503 errors |
| **Terminal** | No | Bad request, content policy, 401 |

This enables smart retry logic that only retries transient errors.

## Version History

- **v3.3.0** - Auto-reasoning, Provider Intelligence, YAML config, enhanced memory, error categorization
- **v3.2.0** - Unified tool integration across GoT, Dialectics, and Reflexion (replaces standalone LATS)
- **v3.1.0** - Added LATS (Language Agent Tree Search) with built-in tools
- **v3.0.0** - Added Graph of Thoughts, Reflexion, Streaming Output
- **v2.0.0** - Added Tree of Thoughts, Dialectical Reasoning, multi-provider support
- **v1.0.0** - Initial release with sequential thinking

## License

MIT
