// Captured once from the UX harness's throwaway panel (framework/internal/panel/testdata/ux/up.sh),
// 2026-10-15, never the installed panel (design D4). The run's temp paths read ~/dev, its token is removed.
window.PANEL_DATA = {
  "projects": [
    {
      "id": "alpha",
      "name": "Alpha",
      "default": true,
      "ok": true,
      "trusted": true,
      "working": 3,
      "waiting": 1,
      "idle": 5,
      "needsYou": 1,
      "blocking": 1,
      "oldestWait": 1792064128571,
      "restorable": 1
    },
    {
      "id": "beta",
      "name": "Beta",
      "ok": true,
      "trusted": true,
      "working": 0,
      "waiting": 0,
      "idle": 1,
      "needsYou": 0,
      "blocking": 0,
      "restorable": 0
    },
    {
      "id": "gamma",
      "name": "gamma",
      "ok": false,
      "error": "panel config: unexpected EOF",
      "trusted": false,
      "working": 0,
      "waiting": 0,
      "idle": 0,
      "needsYou": 0,
      "blocking": 0,
      "restorable": 0
    },
    {
      "id": "zeta",
      "name": "Zeta",
      "ok": true,
      "trusted": false,
      "working": 0,
      "waiting": 0,
      "idle": 0,
      "needsYou": 0,
      "blocking": 0,
      "restorable": 0
    }
  ],
  "state": {
    "alpha": {
      "name": "Alpha",
      "projectId": "alpha",
      "root": "~/dev/alpha",
      "now": 1792064140007,
      "startedAt": 1792064093904,
      "lanes": [
        {
          "id": "~/dev/alpha",
          "path": "~/dev/alpha",
          "branch": "main",
          "type": "orchestrator",
          "name": "idle",
          "status": "idle",
          "ctxPct": 12,
          "subagents": [],
          "finishedSubagents": [],
          "metrics": {
            "ctxPct": 12,
            "ctxWindow": 200000,
            "inputTokens": 128000,
            "outputTokens": 9100,
            "cacheReadTokens": 26394,
            "cacheWriteTokens": 10579,
            "cacheHitRatio": 0.92,
            "cacheWarm": true,
            "cacheExpiresAt": 1792065938000,
            "cacheMisses": 1,
            "cacheRequests": 14,
            "recacheTokens": 36983,
            "linesAdded": 412,
            "linesRemoved": 97,
            "durationMs": 1800000,
            "apiDurationMs": 600000,
            "costUsd": 0.2,
            "thinking": true,
            "fastMode": false,
            "outputStyle": "default",
            "statusAt": 1792064138324,
            "turns": 1,
            "subagentsRunning": 0,
            "subagentsFinished": 0,
            "state": "idle",
            "stateSince": 1792064102114
          },
          "timeline": [
            {
              "state": "idle",
              "from": 1792064098925
            }
          ],
          "head": "4b63679b7c8d281d7846ce402d546e34989a3f5b",
          "lastEvent": "Stop",
          "lastEventAt": 1792064098134,
          "lastHookAt": 1792064098134,
          "stale": false,
          "sessions": [
            {
              "id": "3fc6218c-d421-490a-a386-1d3b39c199aa",
              "name": "idle",
              "pid": 95828,
              "kind": "interactive",
              "status": "idle",
              "ctxPct": 12,
              "model": "Opus 4.1",
              "estCostUsd": 0.2,
              "startedAt": 1792064094084,
              "metrics": {
                "ctxPct": 12,
                "ctxWindow": 200000,
                "inputTokens": 128000,
                "outputTokens": 9100,
                "cacheReadTokens": 26394,
                "cacheWriteTokens": 10579,
                "cacheHitRatio": 0.92,
                "cacheWarm": true,
                "cacheExpiresAt": 1792065938000,
                "cacheMisses": 1,
                "cacheRequests": 14,
                "recacheTokens": 36983,
                "linesAdded": 412,
                "linesRemoved": 97,
                "durationMs": 1800000,
                "apiDurationMs": 600000,
                "costUsd": 0.2,
                "thinking": true,
                "fastMode": false,
                "outputStyle": "default",
                "statusAt": 1792064138324,
                "turns": 1,
                "subagentsRunning": 0,
                "subagentsFinished": 0,
                "state": "idle",
                "stateSince": 1792064102114
              },
              "stale": false
            }
          ],
          "terminal": "idle"
        },
        {
          "id": "~/dev/alpha/.claude/worktrees/a-rather-long-lane-name-for-layout",
          "path": "~/dev/alpha/.claude/worktrees/a-rather-long-lane-name-for-layout",
          "branch": "change/a-rather-long-lane-name-for-layout",
          "type": "build",
          "name": "a-rather-long-lane-name-for-layout",
          "status": "idle",
          "ctxPct": 12,
          "subagents": [],
          "finishedSubagents": [],
          "metrics": {
            "ctxPct": 12,
            "ctxWindow": 200000,
            "inputTokens": 128000,
            "outputTokens": 9100,
            "cacheReadTokens": 26394,
            "cacheWriteTokens": 10579,
            "cacheHitRatio": 0.92,
            "cacheWarm": true,
            "cacheExpiresAt": 1792065939000,
            "cacheMisses": 1,
            "cacheRequests": 14,
            "recacheTokens": 36983,
            "linesAdded": 412,
            "linesRemoved": 97,
            "durationMs": 1800000,
            "apiDurationMs": 600000,
            "costUsd": 0.2,
            "thinking": true,
            "fastMode": false,
            "outputStyle": "default",
            "statusAt": 1792064139456,
            "turns": 1,
            "subagentsRunning": 0,
            "subagentsFinished": 0,
            "state": "idle",
            "stateSince": 1792064102114
          },
          "timeline": [
            {
              "state": "idle",
              "from": 1792064103925
            }
          ],
          "readiness": {
            "ready": false,
            "reasons": [
              "no open pull request"
            ],
            "rows": [
              {
                "name": "Pull request",
                "text": "none open for change/a-rather-long-lane-name-for-layout",
                "ok": false
              }
            ]
          },
          "head": "4b63679b7c8d281d7846ce402d546e34989a3f5b",
          "lastEvent": "Stop",
          "lastEventAt": 1792064099303,
          "lastHookAt": 1792064099303,
          "stale": false,
          "sessions": [
            {
              "id": "abedcd1a-bd1f-4824-ab9d-b70d003de441",
              "name": "a-rather-long-lane-name-for-layout",
              "pid": 97180,
              "kind": "interactive",
              "status": "idle",
              "ctxPct": 12,
              "model": "Opus 4.1",
              "estCostUsd": 0.2,
              "startedAt": 1792064095262,
              "metrics": {
                "ctxPct": 12,
                "ctxWindow": 200000,
                "inputTokens": 128000,
                "outputTokens": 9100,
                "cacheReadTokens": 26394,
                "cacheWriteTokens": 10579,
                "cacheHitRatio": 0.92,
                "cacheWarm": true,
                "cacheExpiresAt": 1792065939000,
                "cacheMisses": 1,
                "cacheRequests": 14,
                "recacheTokens": 36983,
                "linesAdded": 412,
                "linesRemoved": 97,
                "durationMs": 1800000,
                "apiDurationMs": 600000,
                "costUsd": 0.2,
                "thinking": true,
                "fastMode": false,
                "outputStyle": "default",
                "statusAt": 1792064139456,
                "turns": 1,
                "subagentsRunning": 0,
                "subagentsFinished": 0,
                "state": "idle",
                "stateSince": 1792064102114
              },
              "stale": false
            }
          ],
          "terminal": "a-rather-long-lane-name-for-layout"
        },
        {
          "id": "~/dev/alpha/.claude/worktrees/budget",
          "path": "~/dev/alpha/.claude/worktrees/budget",
          "branch": "fix/budget",
          "type": "fix",
          "name": "budget",
          "status": "busy",
          "ctxPct": 71,
          "subagents": [],
          "finishedSubagents": [],
          "metrics": {
            "ctxPct": 71,
            "ctxWindow": 200000,
            "inputTokens": 128000,
            "outputTokens": 9100,
            "cacheReadTokens": 26394,
            "cacheWriteTokens": 10579,
            "cacheHitRatio": 0.92,
            "cacheWarm": true,
            "cacheExpiresAt": 1792065939000,
            "cacheMisses": 1,
            "cacheRequests": 14,
            "recacheTokens": 36983,
            "linesAdded": 412,
            "linesRemoved": 97,
            "durationMs": 1800000,
            "apiDurationMs": 600000,
            "costUsd": 48.25,
            "thinking": true,
            "fastMode": false,
            "outputStyle": "default",
            "statusAt": 1792064139621,
            "turns": 1,
            "subagentsRunning": 0,
            "subagentsFinished": 0,
            "state": "busy",
            "stateSince": 1792064102114
          },
          "timeline": [
            {
              "state": "busy",
              "from": 1792064103925
            }
          ],
          "budget": {
            "change": "budget",
            "usd": 20,
            "spent": 0
          },
          "readiness": {
            "ready": false,
            "reasons": [
              "no open pull request"
            ],
            "rows": [
              {
                "name": "Pull request",
                "text": "none open for fix/budget",
                "ok": false
              }
            ]
          },
          "head": "4b63679b7c8d281d7846ce402d546e34989a3f5b",
          "lastEvent": "UserPromptSubmit",
          "lastEventAt": 1792064099439,
          "lastHookAt": 1792064099439,
          "stale": false,
          "sessions": [
            {
              "id": "41415f2c-6574-4fe6-9d57-e2cc8abdeb49",
              "name": "budget",
              "pid": 97410,
              "kind": "interactive",
              "status": "busy",
              "ctxPct": 71,
              "model": "Opus 4.1",
              "estCostUsd": 48.25,
              "busySince": 1792064102114,
              "startedAt": 1792064095418,
              "metrics": {
                "ctxPct": 71,
                "ctxWindow": 200000,
                "inputTokens": 128000,
                "outputTokens": 9100,
                "cacheReadTokens": 26394,
                "cacheWriteTokens": 10579,
                "cacheHitRatio": 0.92,
                "cacheWarm": true,
                "cacheExpiresAt": 1792065939000,
                "cacheMisses": 1,
                "cacheRequests": 14,
                "recacheTokens": 36983,
                "linesAdded": 412,
                "linesRemoved": 97,
                "durationMs": 1800000,
                "apiDurationMs": 600000,
                "costUsd": 48.25,
                "thinking": true,
                "fastMode": false,
                "outputStyle": "default",
                "statusAt": 1792064139621,
                "turns": 1,
                "subagentsRunning": 0,
                "subagentsFinished": 0,
                "state": "busy",
                "stateSince": 1792064102114
              },
              "stale": false
            }
          ],
          "terminal": "budget"
        },
        {
          "id": "~/dev/alpha/.claude/worktrees/merged",
          "path": "~/dev/alpha/.claude/worktrees/merged",
          "branch": "change/merged",
          "type": "build",
          "name": "merged",
          "status": "idle",
          "ctxPct": 12,
          "subagents": [],
          "finishedSubagents": [],
          "metrics": {
            "ctxPct": 12,
            "ctxWindow": 200000,
            "inputTokens": 128000,
            "outputTokens": 9100,
            "cacheReadTokens": 26394,
            "cacheWriteTokens": 10579,
            "cacheHitRatio": 0.92,
            "cacheWarm": true,
            "cacheExpiresAt": 1792065938000,
            "cacheMisses": 1,
            "cacheRequests": 14,
            "recacheTokens": 36983,
            "linesAdded": 412,
            "linesRemoved": 97,
            "durationMs": 1800000,
            "apiDurationMs": 600000,
            "costUsd": 0.2,
            "thinking": true,
            "fastMode": false,
            "outputStyle": "default",
            "statusAt": 1792064138830,
            "turns": 1,
            "subagentsRunning": 0,
            "subagentsFinished": 0,
            "state": "idle",
            "stateSince": 1792064102114
          },
          "timeline": [
            {
              "state": "idle",
              "from": 1792064098925
            }
          ],
          "readiness": {
            "ready": false,
            "reasons": [
              "no open pull request"
            ],
            "rows": [
              {
                "name": "Pull request",
                "text": "none open for change/merged",
                "ok": false
              }
            ]
          },
          "head": "5da12da5be5d263235a67281b2401950bc0aa4d4",
          "lastEvent": "Stop",
          "lastEventAt": 1792064098678,
          "lastHookAt": 1792064098678,
          "stale": false,
          "sessions": [
            {
              "id": "9a1bded1-1402-4b27-beb9-b83153f512c5",
              "name": "merged",
              "pid": 96288,
              "kind": "interactive",
              "status": "idle",
              "ctxPct": 12,
              "model": "Opus 4.1",
              "estCostUsd": 0.2,
              "startedAt": 1792064094641,
              "metrics": {
                "ctxPct": 12,
                "ctxWindow": 200000,
                "inputTokens": 128000,
                "outputTokens": 9100,
                "cacheReadTokens": 26394,
                "cacheWriteTokens": 10579,
                "cacheHitRatio": 0.92,
                "cacheWarm": true,
                "cacheExpiresAt": 1792065938000,
                "cacheMisses": 1,
                "cacheRequests": 14,
                "recacheTokens": 36983,
                "linesAdded": 412,
                "linesRemoved": 97,
                "durationMs": 1800000,
                "apiDurationMs": 600000,
                "costUsd": 0.2,
                "thinking": true,
                "fastMode": false,
                "outputStyle": "default",
                "statusAt": 1792064138830,
                "turns": 1,
                "subagentsRunning": 0,
                "subagentsFinished": 0,
                "state": "idle",
                "stateSince": 1792064102114
              },
              "stale": false
            }
          ],
          "terminal": "merged"
        },
        {
          "id": "~/dev/alpha/.claude/worktrees/quiet",
          "path": "~/dev/alpha/.claude/worktrees/quiet",
          "branch": "change/quiet",
          "type": "build",
          "name": "quiet",
          "status": "idle",
          "ctxPct": 12,
          "subagents": [],
          "finishedSubagents": [],
          "metrics": {
            "ctxPct": 12,
            "ctxWindow": 200000,
            "inputTokens": 128000,
            "outputTokens": 9100,
            "cacheReadTokens": 26394,
            "cacheWriteTokens": 10579,
            "cacheHitRatio": 0.92,
            "cacheWarm": true,
            "cacheExpiresAt": 1792065939000,
            "cacheMisses": 1,
            "cacheRequests": 14,
            "recacheTokens": 36983,
            "linesAdded": 412,
            "linesRemoved": 97,
            "durationMs": 1800000,
            "apiDurationMs": 600000,
            "costUsd": 0.2,
            "thinking": true,
            "fastMode": false,
            "outputStyle": "default",
            "statusAt": 1792064139143,
            "turns": 1,
            "subagentsRunning": 0,
            "subagentsFinished": 0,
            "state": "idle",
            "stateSince": 1792064102114
          },
          "timeline": [
            {
              "state": "idle",
              "from": 1792064103925
            }
          ],
          "readiness": {
            "ready": false,
            "reasons": [
              "no open pull request"
            ],
            "rows": [
              {
                "name": "Pull request",
                "text": "none open for change/quiet",
                "ok": false
              }
            ]
          },
          "head": "9f4b57de49f3ad3972907e98a366851dcb5f55f0",
          "lastEvent": "Stop",
          "lastEventAt": 1792064098985,
          "lastHookAt": 1792064098985,
          "stale": false,
          "sessions": [
            {
              "id": "bb90a693-7c5d-4598-a4cc-9f4e51555fea",
              "name": "quiet",
              "pid": 96645,
              "kind": "interactive",
              "status": "idle",
              "ctxPct": 12,
              "model": "Opus 4.1",
              "estCostUsd": 0.2,
              "startedAt": 1792064094944,
              "metrics": {
                "ctxPct": 12,
                "ctxWindow": 200000,
                "inputTokens": 128000,
                "outputTokens": 9100,
                "cacheReadTokens": 26394,
                "cacheWriteTokens": 10579,
                "cacheHitRatio": 0.92,
                "cacheWarm": true,
                "cacheExpiresAt": 1792065939000,
                "cacheMisses": 1,
                "cacheRequests": 14,
                "recacheTokens": 36983,
                "linesAdded": 412,
                "linesRemoved": 97,
                "durationMs": 1800000,
                "apiDurationMs": 600000,
                "costUsd": 0.2,
                "thinking": true,
                "fastMode": false,
                "outputStyle": "default",
                "statusAt": 1792064139143,
                "turns": 1,
                "subagentsRunning": 0,
                "subagentsFinished": 0,
                "state": "idle",
                "stateSince": 1792064102114
              },
              "stale": false
            }
          ],
          "terminal": "quiet"
        },
        {
          "id": "~/dev/alpha/.claude/worktrees/ready",
          "path": "~/dev/alpha/.claude/worktrees/ready",
          "branch": "change/ready",
          "type": "build",
          "name": "ready",
          "status": "idle",
          "ctxPct": 12,
          "subagents": [],
          "finishedSubagents": [],
          "metrics": {
            "ctxPct": 12,
            "ctxWindow": 200000,
            "inputTokens": 128000,
            "outputTokens": 9100,
            "cacheReadTokens": 26394,
            "cacheWriteTokens": 10579,
            "cacheHitRatio": 0.92,
            "cacheWarm": true,
            "cacheExpiresAt": 1792065939000,
            "cacheMisses": 1,
            "cacheRequests": 14,
            "recacheTokens": 36983,
            "linesAdded": 412,
            "linesRemoved": 97,
            "durationMs": 1800000,
            "apiDurationMs": 600000,
            "costUsd": 0.2,
            "thinking": true,
            "fastMode": false,
            "outputStyle": "default",
            "statusAt": 1792064139295,
            "turns": 1,
            "subagentsRunning": 0,
            "subagentsFinished": 0,
            "state": "idle",
            "stateSince": 1792064102114
          },
          "timeline": [
            {
              "state": "idle",
              "from": 1792064103925
            }
          ],
          "readiness": {
            "ready": false,
            "reasons": [
              "review threads not read yet"
            ],
            "pr": {
              "number": 44,
              "title": "Ready: all green",
              "headRefName": "change/ready",
              "author": "ux",
              "isDraft": false,
              "checksPass": 2,
              "checksFail": 0,
              "checksPending": 0,
              "review": "APPROVED"
            },
            "rows": [
              {
                "name": "Pull request",
                "text": "#44, open",
                "ok": true
              },
              {
                "name": "Checks",
                "text": "2 passed, 0 failed, 0 pending",
                "ok": true
              },
              {
                "name": "Review",
                "text": "approved",
                "ok": true
              },
              {
                "name": "Review threads",
                "text": "not read yet",
                "ok": false,
                "unknown": true
              }
            ]
          },
          "head": "879545963f501170dca6cb4054e9b3464f45ec05",
          "lastEvent": "Stop",
          "lastEventAt": 1792064099127,
          "lastHookAt": 1792064099127,
          "stale": false,
          "sessions": [
            {
              "id": "6b5951a0-e832-48c8-8419-2e761aeabacc",
              "name": "ready",
              "pid": 96867,
              "kind": "interactive",
              "status": "idle",
              "ctxPct": 12,
              "model": "Opus 4.1",
              "estCostUsd": 0.2,
              "startedAt": 1792064095083,
              "metrics": {
                "ctxPct": 12,
                "ctxWindow": 200000,
                "inputTokens": 128000,
                "outputTokens": 9100,
                "cacheReadTokens": 26394,
                "cacheWriteTokens": 10579,
                "cacheHitRatio": 0.92,
                "cacheWarm": true,
                "cacheExpiresAt": 1792065939000,
                "cacheMisses": 1,
                "cacheRequests": 14,
                "recacheTokens": 36983,
                "linesAdded": 412,
                "linesRemoved": 97,
                "durationMs": 1800000,
                "apiDurationMs": 600000,
                "costUsd": 0.2,
                "thinking": true,
                "fastMode": false,
                "outputStyle": "default",
                "statusAt": 1792064139295,
                "turns": 1,
                "subagentsRunning": 0,
                "subagentsFinished": 0,
                "state": "idle",
                "stateSince": 1792064102114
              },
              "stale": false
            }
          ],
          "terminal": "ready"
        },
        {
          "id": "~/dev/alpha/.claude/worktrees/stream",
          "path": "~/dev/alpha/.claude/worktrees/stream",
          "branch": "change/stream",
          "type": "build",
          "name": "stream",
          "status": "busy",
          "ctxPct": 88,
          "subagents": [
            {
              "id": "s8",
              "type": "general-purpose",
              "since": 1792064138763,
              "session": "c5abd51b-9519-44a8-8ecf-caaa6eac200b"
            }
          ],
          "finishedSubagents": [
            {
              "id": "s4",
              "type": "general-purpose",
              "since": 1792064118654,
              "ended": 1792064138755,
              "session": "c5abd51b-9519-44a8-8ecf-caaa6eac200b"
            },
            {
              "id": "s0",
              "type": "general-purpose",
              "since": 1792064098545,
              "ended": 1792064118646,
              "session": "c5abd51b-9519-44a8-8ecf-caaa6eac200b"
            }
          ],
          "metrics": {
            "ctxPct": 88,
            "ctxWindow": 200000,
            "inputTokens": 128000,
            "outputTokens": 9100,
            "cacheReadTokens": 26394,
            "cacheWriteTokens": 10579,
            "cacheHitRatio": 0.92,
            "cacheWarm": true,
            "cacheExpiresAt": 1792065938000,
            "cacheMisses": 1,
            "cacheRequests": 14,
            "recacheTokens": 36983,
            "linesAdded": 412,
            "linesRemoved": 97,
            "durationMs": 1800000,
            "apiDurationMs": 600000,
            "costUsd": 7.4,
            "thinking": true,
            "fastMode": false,
            "outputStyle": "default",
            "statusAt": 1792064138748,
            "turns": 1,
            "subagentsRunning": 1,
            "subagentsFinished": 2,
            "state": "busy",
            "stateSince": 1792064102114
          },
          "timeline": [
            {
              "state": "busy",
              "from": 1792064098925
            }
          ],
          "readiness": {
            "ready": false,
            "reasons": [
              "no open pull request"
            ],
            "rows": [
              {
                "name": "Pull request",
                "text": "none open for change/stream",
                "ok": false
              }
            ]
          },
          "head": "4b63679b7c8d281d7846ce402d546e34989a3f5b",
          "lastEvent": "SubagentStart",
          "lastEventAt": 1792064138763,
          "lastHookAt": 1792064138763,
          "stale": false,
          "sessions": [
            {
              "id": "c5abd51b-9519-44a8-8ecf-caaa6eac200b",
              "name": "stream",
              "pid": 96202,
              "kind": "interactive",
              "status": "busy",
              "ctxPct": 88,
              "model": "Opus 4.1",
              "estCostUsd": 7.4,
              "busySince": 1792064102114,
              "startedAt": 1792064094501,
              "metrics": {
                "ctxPct": 88,
                "ctxWindow": 200000,
                "inputTokens": 128000,
                "outputTokens": 9100,
                "cacheReadTokens": 26394,
                "cacheWriteTokens": 10579,
                "cacheHitRatio": 0.92,
                "cacheWarm": true,
                "cacheExpiresAt": 1792065938000,
                "cacheMisses": 1,
                "cacheRequests": 14,
                "recacheTokens": 36983,
                "linesAdded": 412,
                "linesRemoved": 97,
                "durationMs": 1800000,
                "apiDurationMs": 600000,
                "costUsd": 7.4,
                "thinking": true,
                "fastMode": false,
                "outputStyle": "default",
                "statusAt": 1792064138748,
                "turns": 1,
                "subagentsRunning": 1,
                "subagentsFinished": 2,
                "state": "busy",
                "stateSince": 1792064102114
              },
              "stale": false
            }
          ],
          "terminal": "stream"
        },
        {
          "id": "~/dev/alpha/.claude/worktrees/waiting",
          "path": "~/dev/alpha/.claude/worktrees/waiting",
          "branch": "change/waiting",
          "type": "build",
          "name": "waiting",
          "status": "waiting",
          "waitingFor": "permission: Bash(npm run test:e2e)",
          "ctxPct": 31,
          "subagents": [],
          "finishedSubagents": [],
          "metrics": {
            "ctxPct": 31,
            "ctxWindow": 200000,
            "inputTokens": 128000,
            "outputTokens": 9100,
            "cacheReadTokens": 26394,
            "cacheWriteTokens": 10579,
            "cacheHitRatio": 0.92,
            "cacheWarm": true,
            "cacheExpiresAt": 1792065938000,
            "cacheMisses": 1,
            "cacheRequests": 14,
            "recacheTokens": 36983,
            "linesAdded": 412,
            "linesRemoved": 97,
            "durationMs": 1800000,
            "apiDurationMs": 600000,
            "costUsd": 0.8,
            "thinking": true,
            "fastMode": false,
            "outputStyle": "default",
            "statusAt": 1792064138610,
            "turns": 1,
            "asks": 1,
            "subagentsRunning": 0,
            "subagentsFinished": 0,
            "state": "waiting",
            "stateSince": 1792064102114
          },
          "timeline": [
            {
              "state": "waiting",
              "from": 1792064098925
            }
          ],
          "readiness": {
            "ready": false,
            "reasons": [
              "#42 is a draft",
              "1 check(s) failed",
              "review threads not read yet"
            ],
            "pr": {
              "number": 42,
              "title": "Waiting: needs a permission",
              "headRefName": "change/waiting",
              "author": "ux",
              "isDraft": true,
              "checksPass": 0,
              "checksFail": 1,
              "checksPending": 0
            },
            "rows": [
              {
                "name": "Pull request",
                "text": "#42, a draft",
                "ok": false
              },
              {
                "name": "Checks",
                "text": "0 passed, 1 failed, 0 pending",
                "ok": false
              },
              {
                "name": "Review",
                "text": "no review decision",
                "ok": true
              },
              {
                "name": "Review threads",
                "text": "not read yet",
                "ok": false,
                "unknown": true
              }
            ]
          },
          "head": "4b63679b7c8d281d7846ce402d546e34989a3f5b",
          "lastEvent": "PermissionRequest",
          "lastEventAt": 1792064128571,
          "lastHookAt": 1792064128571,
          "stale": false,
          "sessions": [
            {
              "id": "0a189007-0058-49f5-9351-dbbb2ccec9c7",
              "name": "waiting",
              "pid": 96116,
              "kind": "interactive",
              "status": "waiting",
              "waitingFor": "permission: Bash(npm run test:e2e)",
              "ctxPct": 31,
              "model": "Opus 4.1",
              "estCostUsd": 0.8,
              "startedAt": 1792064094359,
              "metrics": {
                "ctxPct": 31,
                "ctxWindow": 200000,
                "inputTokens": 128000,
                "outputTokens": 9100,
                "cacheReadTokens": 26394,
                "cacheWriteTokens": 10579,
                "cacheHitRatio": 0.92,
                "cacheWarm": true,
                "cacheExpiresAt": 1792065938000,
                "cacheMisses": 1,
                "cacheRequests": 14,
                "recacheTokens": 36983,
                "linesAdded": 412,
                "linesRemoved": 97,
                "durationMs": 1800000,
                "apiDurationMs": 600000,
                "costUsd": 0.8,
                "thinking": true,
                "fastMode": false,
                "outputStyle": "default",
                "statusAt": 1792064138610,
                "turns": 1,
                "asks": 1,
                "subagentsRunning": 0,
                "subagentsFinished": 0,
                "state": "waiting",
                "stateSince": 1792064102114
              },
              "stale": false,
              "waitingKind": "permission"
            }
          ],
          "terminal": "waiting"
        },
        {
          "id": "~/dev/alpha/.claude/worktrees/working",
          "path": "~/dev/alpha/.claude/worktrees/working",
          "branch": "change/working",
          "type": "build",
          "name": "working",
          "status": "busy",
          "ctxPct": 64,
          "subagents": [
            {
              "id": "a0d75f6b6",
              "type": "Explore",
              "since": 1792064128461,
              "session": "cf559cb3-f4ad-413b-9a80-67e7af0a6566",
              "description": "Survey the code"
            }
          ],
          "finishedSubagents": [
            {
              "id": "a0a344984",
              "type": "Explore",
              "since": 1792064113371,
              "ended": 1792064128476,
              "session": "cf559cb3-f4ad-413b-9a80-67e7af0a6566",
              "description": "Survey the code"
            },
            {
              "id": "a1c7f62a6",
              "type": "Explore",
              "since": 1792064098272,
              "ended": 1792064113389,
              "session": "cf559cb3-f4ad-413b-9a80-67e7af0a6566",
              "description": "Survey the code"
            }
          ],
          "metrics": {
            "ctxPct": 64,
            "ctxWindow": 200000,
            "inputTokens": 128000,
            "outputTokens": 9100,
            "cacheReadTokens": 26394,
            "cacheWriteTokens": 10579,
            "cacheHitRatio": 0.92,
            "cacheWarm": true,
            "cacheExpiresAt": 1792065938000,
            "cacheMisses": 1,
            "cacheRequests": 14,
            "recacheTokens": 36983,
            "linesAdded": 412,
            "linesRemoved": 97,
            "durationMs": 1800000,
            "apiDurationMs": 600000,
            "costUsd": 3.1,
            "thinking": true,
            "fastMode": false,
            "outputStyle": "default",
            "statusAt": 1792064138512,
            "turns": 1,
            "subagentsRunning": 1,
            "subagentsFinished": 2,
            "state": "busy",
            "stateSince": 1792064102114
          },
          "timeline": [
            {
              "state": "busy",
              "from": 1792064098925
            }
          ],
          "budget": {
            "change": "working",
            "usd": 3.5,
            "spent": 0
          },
          "readiness": {
            "ready": false,
            "reasons": [
              "1 check(s) pending",
              "a review is required",
              "review threads not read yet"
            ],
            "pr": {
              "number": 41,
              "title": "Working: add the thing",
              "headRefName": "change/working",
              "author": "ux",
              "isDraft": false,
              "checksPass": 2,
              "checksFail": 0,
              "checksPending": 1,
              "review": "REVIEW_REQUIRED"
            },
            "rows": [
              {
                "name": "Pull request",
                "text": "#41, open",
                "ok": true
              },
              {
                "name": "Checks",
                "text": "2 passed, 0 failed, 1 pending",
                "ok": false
              },
              {
                "name": "Review",
                "text": "review required",
                "ok": false
              },
              {
                "name": "Review threads",
                "text": "not read yet",
                "ok": false,
                "unknown": true
              }
            ]
          },
          "head": "5bdd623ec44cf85c7940c2a95712569da0ec832c",
          "lastEvent": "SubagentStop",
          "lastEventAt": 1792064128476,
          "lastHookAt": 1792064128476,
          "stale": false,
          "sessions": [
            {
              "id": "cf559cb3-f4ad-413b-9a80-67e7af0a6566",
              "name": "working",
              "pid": 96046,
              "kind": "interactive",
              "status": "busy",
              "ctxPct": 64,
              "model": "Opus 4.1",
              "estCostUsd": 3.1,
              "busySince": 1792064102114,
              "startedAt": 1792064094222,
              "metrics": {
                "ctxPct": 64,
                "ctxWindow": 200000,
                "inputTokens": 128000,
                "outputTokens": 9100,
                "cacheReadTokens": 26394,
                "cacheWriteTokens": 10579,
                "cacheHitRatio": 0.92,
                "cacheWarm": true,
                "cacheExpiresAt": 1792065938000,
                "cacheMisses": 1,
                "cacheRequests": 14,
                "recacheTokens": 36983,
                "linesAdded": 412,
                "linesRemoved": 97,
                "durationMs": 1800000,
                "apiDurationMs": 600000,
                "costUsd": 3.1,
                "thinking": true,
                "fastMode": false,
                "outputStyle": "default",
                "statusAt": 1792064138512,
                "turns": 1,
                "subagentsRunning": 1,
                "subagentsFinished": 2,
                "state": "busy",
                "stateSince": 1792064102114
              },
              "stale": false
            }
          ],
          "terminal": "working"
        }
      ],
      "quietWorktrees": [
        {
          "id": "~/dev/alpha/.claude/worktrees/leftover-clean",
          "path": "~/dev/alpha/.claude/worktrees/leftover-clean",
          "branch": "",
          "type": "detached",
          "name": "(detached)",
          "status": "none",
          "ctxPct": null,
          "subagents": [],
          "finishedSubagents": [],
          "metrics": {
            "subagentsRunning": 0,
            "subagentsFinished": 0
          },
          "head": "4b63679b7c8d281d7846ce402d546e34989a3f5b",
          "stale": false,
          "sessions": []
        },
        {
          "id": "~/dev/alpha/.claude/worktrees/leftover-dirty",
          "path": "~/dev/alpha/.claude/worktrees/leftover-dirty",
          "branch": "",
          "type": "detached",
          "name": "(detached)",
          "status": "none",
          "ctxPct": null,
          "subagents": [],
          "finishedSubagents": [],
          "metrics": {
            "subagentsRunning": 0,
            "subagentsFinished": 0
          },
          "head": "4b63679b7c8d281d7846ce402d546e34989a3f5b",
          "stale": false,
          "sessions": []
        },
        {
          "id": "~/dev/alpha/.claude/worktrees/orphan",
          "path": "~/dev/alpha/.claude/worktrees/orphan",
          "branch": "change/orphan",
          "type": "build",
          "name": "orphan",
          "status": "none",
          "ctxPct": null,
          "subagents": [],
          "finishedSubagents": [],
          "metrics": {
            "subagentsRunning": 0,
            "subagentsFinished": 0
          },
          "readiness": {
            "ready": false,
            "reasons": [
              "no open pull request"
            ],
            "rows": [
              {
                "name": "Pull request",
                "text": "none open for change/orphan",
                "ok": false
              }
            ]
          },
          "head": "4b63679b7c8d281d7846ce402d546e34989a3f5b",
          "lastHookAt": 1792064096954,
          "stale": false,
          "sessions": []
        }
      ],
      "quota": {
        "windows": [
          {
            "key": "five_hour",
            "label": "5-hour",
            "pct": 42,
            "resetsAt": 1792071339
          },
          {
            "key": "seven_day",
            "label": "7-day",
            "pct": 61,
            "resetsAt": 1792323339
          }
        ],
        "at": 1792064139675,
        "fromSession": "e6479329-1558-4693-9395-785138ed3bc2",
        "account": "eaa3c51c8f4c",
        "fiveHour": 42,
        "sevenDay": 61,
        "fiveHourResetsAt": 1792071339,
        "sevenDayResetsAt": 1792323339
      },
      "account": {
        "mode": "subscription",
        "plan": "Max",
        "method": "claude.ai",
        "quotaMode": "windows",
        "source": {
          "ok": true,
          "pending": false,
          "at": 1792064094045
        }
      },
      "estCostUsd": 60.550000000000004,
      "needsYou": [
        {
          "lane": "~/dev/alpha/.claude/worktrees/waiting",
          "name": "waiting",
          "session": "0a189007-0058-49f5-9351-dbbb2ccec9c7",
          "kind": "permission_prompt",
          "label": "Permission",
          "severity": "block",
          "text": "Bash(npm run test:e2e)",
          "at": 1792064128571,
          "terminal": "waiting"
        }
      ],
      "cards": [
        {
          "id": "founder",
          "title": "Founder queue",
          "source": {
            "ok": true,
            "pending": false,
            "at": 1792064096957
          },
          "output": {
            "kind": "text",
            "lines": [
              "3 item(s) need the founder:",
              "- **Box cleanup** (queued 2026-09-27). On the box, prune images and old volumes before the disk fills",
              "- **Ideas page** open it once, then invite a designer",
              "- **A deliberately long row title that keeps going to see how the pinned card box wraps or clips it at narrow widths**"
            ]
          },
          "pin": true
        },
        {
          "id": "queue",
          "title": "Change queue",
          "source": {
            "ok": true,
            "pending": false,
            "at": 1792064096957
          },
          "output": {
            "kind": "text",
            "lines": [
              "2C.10 add-score-photo — photograph a scorecard — in flight",
              "2C.27 add-group-card — one golfer enters the card — queued",
              "2C.31 fix-offline-sync — sync after airplane mode — queued"
            ]
          },
          "pin": true
        },
        {
          "id": "health",
          "title": "Health",
          "source": {
            "ok": true,
            "pending": false,
            "at": 1792064096943
          },
          "output": {
            "kind": "text",
            "lines": [
              "gate: green",
              "coverage: 81%"
            ]
          }
        }
      ],
      "prs": [
        {
          "number": 41,
          "title": "Working: add the thing",
          "headRefName": "change/working",
          "author": "ux",
          "isDraft": false,
          "checksPass": 2,
          "checksFail": 0,
          "checksPending": 1,
          "review": "REVIEW_REQUIRED"
        },
        {
          "number": 42,
          "title": "Waiting: needs a permission",
          "headRefName": "change/waiting",
          "author": "ux",
          "isDraft": true,
          "checksPass": 0,
          "checksFail": 1,
          "checksPending": 0
        },
        {
          "number": 43,
          "title": "deps: bump the routine group with 2 updates",
          "headRefName": "dependabot/npm/routine",
          "author": "app/dependabot",
          "isDraft": false,
          "checksPass": 0,
          "checksFail": 0,
          "checksPending": 0,
          "review": "APPROVED"
        },
        {
          "number": 44,
          "title": "Ready: all green",
          "headRefName": "change/ready",
          "author": "ux",
          "isDraft": false,
          "checksPass": 2,
          "checksFail": 0,
          "checksPending": 0,
          "review": "APPROVED"
        }
      ],
      "feed": [
        {
          "at": 1792064138763,
          "lane": "~/dev/alpha/.claude/worktrees/stream",
          "name": "stream",
          "type": "build",
          "session": "c5abd51b-9519-44a8-8ecf-caaa6eac200b",
          "event": "SubagentStart",
          "detail": "general-purpose s8"
        },
        {
          "at": 1792064138755,
          "lane": "~/dev/alpha/.claude/worktrees/stream",
          "name": "stream",
          "type": "build",
          "session": "c5abd51b-9519-44a8-8ecf-caaa6eac200b",
          "event": "SubagentStop",
          "detail": "general-purpose s4"
        },
        {
          "at": 1792064128571,
          "lane": "~/dev/alpha/.claude/worktrees/waiting",
          "name": "waiting",
          "type": "build",
          "session": "0a189007-0058-49f5-9351-dbbb2ccec9c7",
          "event": "PermissionRequest",
          "detail": "Bash"
        },
        {
          "at": 1792064128563,
          "lane": "~/dev/alpha/.claude/worktrees/waiting",
          "name": "waiting",
          "type": "build",
          "session": "0a189007-0058-49f5-9351-dbbb2ccec9c7",
          "event": "Notification",
          "detail": "permission_prompt Claude needs your permission to use Bash"
        },
        {
          "at": 1792064128476,
          "lane": "~/dev/alpha/.claude/worktrees/working",
          "name": "working",
          "type": "build",
          "session": "cf559cb3-f4ad-413b-9a80-67e7af0a6566",
          "event": "SubagentStop",
          "detail": "Explore a0a3449"
        },
        {
          "at": 1792064128468,
          "lane": "~/dev/alpha/.claude/worktrees/working",
          "name": "working",
          "type": "build",
          "session": "cf559cb3-f4ad-413b-9a80-67e7af0a6566",
          "event": "PostToolUse",
          "detail": "started (untyped) a0d75f6"
        },
        {
          "at": 1792064128461,
          "lane": "~/dev/alpha/.claude/worktrees/working",
          "name": "working",
          "type": "build",
          "session": "cf559cb3-f4ad-413b-9a80-67e7af0a6566",
          "event": "SubagentStart",
          "detail": "Explore a0d75f6"
        },
        {
          "at": 1792064128453,
          "lane": "~/dev/alpha/.claude/worktrees/working",
          "name": "working",
          "type": "build",
          "session": "cf559cb3-f4ad-413b-9a80-67e7af0a6566",
          "event": "PreToolUse",
          "detail": "Agent Explore: Survey the code"
        },
        {
          "at": 1792064118654,
          "lane": "~/dev/alpha/.claude/worktrees/stream",
          "name": "stream",
          "type": "build",
          "session": "c5abd51b-9519-44a8-8ecf-caaa6eac200b",
          "event": "SubagentStart",
          "detail": "general-purpose s4"
        },
        {
          "at": 1792064118646,
          "lane": "~/dev/alpha/.claude/worktrees/stream",
          "name": "stream",
          "type": "build",
          "session": "c5abd51b-9519-44a8-8ecf-caaa6eac200b",
          "event": "SubagentStop",
          "detail": "general-purpose s0"
        },
        {
          "at": 1792064113389,
          "lane": "~/dev/alpha/.claude/worktrees/working",
          "name": "working",
          "type": "build",
          "session": "cf559cb3-f4ad-413b-9a80-67e7af0a6566",
          "event": "SubagentStop",
          "detail": "Explore a1c7f62"
        },
        {
          "at": 1792064113380,
          "lane": "~/dev/alpha/.claude/worktrees/working",
          "name": "working",
          "type": "build",
          "session": "cf559cb3-f4ad-413b-9a80-67e7af0a6566",
          "event": "PostToolUse",
          "detail": "started (untyped) a0a3449"
        },
        {
          "at": 1792064113371,
          "lane": "~/dev/alpha/.claude/worktrees/working",
          "name": "working",
          "type": "build",
          "session": "cf559cb3-f4ad-413b-9a80-67e7af0a6566",
          "event": "SubagentStart",
          "detail": "Explore a0a3449"
        },
        {
          "at": 1792064113363,
          "lane": "~/dev/alpha/.claude/worktrees/working",
          "name": "working",
          "type": "build",
          "session": "cf559cb3-f4ad-413b-9a80-67e7af0a6566",
          "event": "PreToolUse",
          "detail": "Agent Explore: Survey the code"
        },
        {
          "at": 1792064102114,
          "lane": "~/dev/alpha/.claude/worktrees/working",
          "name": "working",
          "type": "build",
          "session": "cf559cb3-f4ad-413b-9a80-67e7af0a6566",
          "event": "session",
          "detail": "seen · busy"
        },
        {
          "at": 1792064102114,
          "lane": "~/dev/alpha/.claude/worktrees/stream",
          "name": "stream",
          "type": "build",
          "session": "c5abd51b-9519-44a8-8ecf-caaa6eac200b",
          "event": "session",
          "detail": "seen · busy"
        },
        {
          "at": 1792064102114,
          "lane": "~/dev/alpha/.claude/worktrees/quiet",
          "name": "quiet",
          "type": "build",
          "session": "bb90a693-7c5d-4598-a4cc-9f4e51555fea",
          "event": "session",
          "detail": "seen · idle"
        },
        {
          "at": 1792064102114,
          "lane": "~/dev/alpha/.claude/worktrees/a-rather-long-lane-name-for-layout",
          "name": "a-rather-long-lane-name-for-layout",
          "type": "build",
          "session": "abedcd1a-bd1f-4824-ab9d-b70d003de441",
          "event": "session",
          "detail": "seen · idle"
        },
        {
          "at": 1792064102114,
          "lane": "~/dev/alpha/.claude/worktrees/merged",
          "name": "merged",
          "type": "build",
          "session": "9a1bded1-1402-4b27-beb9-b83153f512c5",
          "event": "session",
          "detail": "seen · idle"
        },
        {
          "at": 1792064102114,
          "lane": "~/dev/alpha/.claude/worktrees/ready",
          "name": "ready",
          "type": "build",
          "session": "6b5951a0-e832-48c8-8419-2e761aeabacc",
          "event": "session",
          "detail": "seen · idle"
        },
        {
          "at": 1792064102114,
          "lane": "~/dev/alpha/.claude/worktrees/budget",
          "name": "budget",
          "type": "fix",
          "session": "41415f2c-6574-4fe6-9d57-e2cc8abdeb49",
          "event": "session",
          "detail": "seen · busy"
        },
        {
          "at": 1792064102114,
          "lane": "~/dev/alpha",
          "name": "idle",
          "type": "orchestrator",
          "session": "3fc6218c-d421-490a-a386-1d3b39c199aa",
          "event": "session",
          "detail": "seen · idle"
        },
        {
          "at": 1792064102114,
          "lane": "~/dev/alpha/.claude/worktrees/waiting",
          "name": "waiting",
          "type": "build",
          "session": "0a189007-0058-49f5-9351-dbbb2ccec9c7",
          "event": "session",
          "detail": "seen · waiting"
        },
        {
          "at": 1792064099439,
          "lane": "~/dev/alpha/.claude/worktrees/budget",
          "name": "budget",
          "type": "fix",
          "session": "41415f2c-6574-4fe6-9d57-e2cc8abdeb49",
          "event": "UserPromptSubmit",
          "detail": "(ux harness)"
        },
        {
          "at": 1792064099303,
          "lane": "~/dev/alpha/.claude/worktrees/a-rather-long-lane-name-for-layout",
          "name": "a-rather-long-lane-name-for-layout",
          "type": "build",
          "session": "abedcd1a-bd1f-4824-ab9d-b70d003de441",
          "event": "Stop",
          "detail": "Done."
        },
        {
          "at": 1792064099284,
          "lane": "~/dev/alpha/.claude/worktrees/a-rather-long-lane-name-for-layout",
          "name": "a-rather-long-lane-name-for-layout",
          "type": "build",
          "session": "abedcd1a-bd1f-4824-ab9d-b70d003de441",
          "event": "UserPromptSubmit",
          "detail": "(ux harness)"
        },
        {
          "at": 1792064099127,
          "lane": "~/dev/alpha/.claude/worktrees/ready",
          "name": "ready",
          "type": "build",
          "session": "6b5951a0-e832-48c8-8419-2e761aeabacc",
          "event": "Stop",
          "detail": "Done."
        },
        {
          "at": 1792064099107,
          "lane": "~/dev/alpha/.claude/worktrees/ready",
          "name": "ready",
          "type": "build",
          "session": "6b5951a0-e832-48c8-8419-2e761aeabacc",
          "event": "UserPromptSubmit",
          "detail": "(ux harness)"
        },
        {
          "at": 1792064098985,
          "lane": "~/dev/alpha/.claude/worktrees/quiet",
          "name": "quiet",
          "type": "build",
          "session": "bb90a693-7c5d-4598-a4cc-9f4e51555fea",
          "event": "Stop",
          "detail": "Done."
        },
        {
          "at": 1792064098965,
          "lane": "~/dev/alpha/.claude/worktrees/quiet",
          "name": "quiet",
          "type": "build",
          "session": "bb90a693-7c5d-4598-a4cc-9f4e51555fea",
          "event": "UserPromptSubmit",
          "detail": "(ux harness)"
        },
        {
          "at": 1792064098678,
          "lane": "~/dev/alpha/.claude/worktrees/merged",
          "name": "merged",
          "type": "build",
          "session": "9a1bded1-1402-4b27-beb9-b83153f512c5",
          "event": "Stop",
          "detail": "Done."
        },
        {
          "at": 1792064098658,
          "lane": "~/dev/alpha/.claude/worktrees/merged",
          "name": "merged",
          "type": "build",
          "session": "9a1bded1-1402-4b27-beb9-b83153f512c5",
          "event": "UserPromptSubmit",
          "detail": "(ux harness)"
        },
        {
          "at": 1792064098545,
          "lane": "~/dev/alpha/.claude/worktrees/stream",
          "name": "stream",
          "type": "build",
          "session": "c5abd51b-9519-44a8-8ecf-caaa6eac200b",
          "event": "SubagentStart",
          "detail": "general-purpose s0"
        },
        {
          "at": 1792064098524,
          "lane": "~/dev/alpha/.claude/worktrees/stream",
          "name": "stream",
          "type": "build",
          "session": "c5abd51b-9519-44a8-8ecf-caaa6eac200b",
          "event": "UserPromptSubmit",
          "detail": "(ux harness)"
        },
        {
          "at": 1792064098411,
          "lane": "~/dev/alpha/.claude/worktrees/waiting",
          "name": "waiting",
          "type": "build",
          "session": "0a189007-0058-49f5-9351-dbbb2ccec9c7",
          "event": "PermissionRequest",
          "detail": "Bash"
        },
        {
          "at": 1792064098403,
          "lane": "~/dev/alpha/.claude/worktrees/waiting",
          "name": "waiting",
          "type": "build",
          "session": "0a189007-0058-49f5-9351-dbbb2ccec9c7",
          "event": "Notification",
          "detail": "permission_prompt Claude needs your permission to use Bash"
        },
        {
          "at": 1792064098384,
          "lane": "~/dev/alpha/.claude/worktrees/waiting",
          "name": "waiting",
          "type": "build",
          "session": "0a189007-0058-49f5-9351-dbbb2ccec9c7",
          "event": "UserPromptSubmit",
          "detail": "(ux harness)"
        },
        {
          "at": 1792064098281,
          "lane": "~/dev/alpha/.claude/worktrees/working",
          "name": "working",
          "type": "build",
          "session": "cf559cb3-f4ad-413b-9a80-67e7af0a6566",
          "event": "PostToolUse",
          "detail": "started (untyped) a1c7f62"
        },
        {
          "at": 1792064098272,
          "lane": "~/dev/alpha/.claude/worktrees/working",
          "name": "working",
          "type": "build",
          "session": "cf559cb3-f4ad-413b-9a80-67e7af0a6566",
          "event": "SubagentStart",
          "detail": "Explore a1c7f62"
        },
        {
          "at": 1792064098263,
          "lane": "~/dev/alpha/.claude/worktrees/working",
          "name": "working",
          "type": "build",
          "session": "cf559cb3-f4ad-413b-9a80-67e7af0a6566",
          "event": "PreToolUse",
          "detail": "Agent Explore: Survey the code"
        },
        {
          "at": 1792064098244,
          "lane": "~/dev/alpha/.claude/worktrees/working",
          "name": "working",
          "type": "build",
          "session": "cf559cb3-f4ad-413b-9a80-67e7af0a6566",
          "event": "UserPromptSubmit",
          "detail": "(ux harness)"
        },
        {
          "at": 1792064098134,
          "lane": "~/dev/alpha",
          "name": "idle",
          "type": "orchestrator",
          "session": "3fc6218c-d421-490a-a386-1d3b39c199aa",
          "event": "Stop",
          "detail": "Done."
        },
        {
          "at": 1792064098107,
          "lane": "~/dev/alpha",
          "name": "idle",
          "type": "orchestrator",
          "session": "3fc6218c-d421-490a-a386-1d3b39c199aa",
          "event": "UserPromptSubmit",
          "detail": "(ux harness)"
        },
        {
          "at": 1792064096954,
          "lane": "~/dev/alpha/.claude/worktrees/orphan",
          "name": "orphan",
          "type": "build",
          "session": "85308fac-b2b6-4fee-b227-b456f50280ed",
          "event": "SessionEnd",
          "detail": "other"
        }
      ],
      "banners": [
        "1 lane(s) lost their tmux session (a reboot, or the tmux server ended): orphan. RESTORE ALL resumes each on its own session id."
      ],
      "bannerItems": [
        {
          "kind": "restore",
          "text": "1 lane(s) lost their tmux session (a reboot, or the tmux server ended): orphan. RESTORE ALL resumes each on its own session id."
        }
      ],
      "agentsReadAt": 1792064137926,
      "sources": {
        "agents": {
          "ok": true,
          "pending": false,
          "at": 1792064137926
        },
        "prs": {
          "ok": true,
          "pending": false,
          "at": 1792064096985
        },
        "tmux": {
          "ok": true,
          "pending": false,
          "at": 1792064139124
        },
        "worktrees": {
          "ok": true,
          "pending": false,
          "at": 1792064133969
        }
      },
      "hookEvents": 35,
      "statusPosts": 81,
      "dropped": 0,
      "terminals": [
        {
          "id": "a-rather-long-lane-name-for-layout",
          "sessionId": "abedcd1a-bd1f-4824-ab9d-b70d003de441",
          "path": "~/dev/alpha/.claude/worktrees/a-rather-long-lane-name-for-layout",
          "type": "build",
          "worktree": "~/dev/alpha/.claude/worktrees/a-rather-long-lane-name-for-layout",
          "branch": "change/a-rather-long-lane-name-for-layout",
          "status": "idle",
          "ctxPct": 12,
          "created": 1792064095,
          "attached": 0,
          "running": true,
          "dead": false,
          "registered": true,
          "port": 39180
        },
        {
          "id": "budget",
          "sessionId": "41415f2c-6574-4fe6-9d57-e2cc8abdeb49",
          "path": "~/dev/alpha/.claude/worktrees/budget",
          "type": "fix",
          "worktree": "~/dev/alpha/.claude/worktrees/budget",
          "branch": "fix/budget",
          "status": "busy",
          "ctxPct": 71,
          "created": 1792064095,
          "attached": 0,
          "running": true,
          "dead": false,
          "registered": true,
          "port": 39190
        },
        {
          "id": "idle",
          "sessionId": "3fc6218c-d421-490a-a386-1d3b39c199aa",
          "path": "~/dev/alpha",
          "type": "orchestrator",
          "worktree": "~/dev/alpha",
          "branch": "main",
          "status": "idle",
          "ctxPct": 12,
          "created": 1792064094,
          "attached": 0,
          "running": true,
          "dead": false,
          "registered": true,
          "port": 39100
        },
        {
          "id": "merged",
          "sessionId": "9a1bded1-1402-4b27-beb9-b83153f512c5",
          "path": "~/dev/alpha/.claude/worktrees/merged",
          "type": "build",
          "worktree": "~/dev/alpha/.claude/worktrees/merged",
          "branch": "change/merged",
          "status": "idle",
          "ctxPct": 12,
          "created": 1792064094,
          "attached": 0,
          "running": true,
          "dead": false,
          "registered": true,
          "port": 39140
        },
        {
          "id": "orphan",
          "sessionId": "85308fac-b2b6-4fee-b227-b456f50280ed",
          "path": "~/dev/alpha/.claude/worktrees/orphan",
          "type": "build",
          "worktree": "~/dev/alpha/.claude/worktrees/orphan",
          "branch": "change/orphan",
          "status": "orphaned",
          "ctxPct": null,
          "created": 1792064094,
          "attached": 0,
          "running": false,
          "dead": false,
          "registered": true,
          "port": 39150,
          "orphan": "its tmux session is gone (a reboot, or the tmux server ended)",
          "restorable": true
        },
        {
          "id": "quiet",
          "sessionId": "bb90a693-7c5d-4598-a4cc-9f4e51555fea",
          "path": "~/dev/alpha/.claude/worktrees/quiet",
          "type": "build",
          "worktree": "~/dev/alpha/.claude/worktrees/quiet",
          "branch": "change/quiet",
          "status": "idle",
          "ctxPct": 12,
          "created": 1791632092,
          "attached": 0,
          "running": true,
          "dead": false,
          "registered": true,
          "port": 39160
        },
        {
          "id": "ready",
          "sessionId": "6b5951a0-e832-48c8-8419-2e761aeabacc",
          "path": "~/dev/alpha/.claude/worktrees/ready",
          "type": "build",
          "worktree": "~/dev/alpha/.claude/worktrees/ready",
          "branch": "change/ready",
          "status": "idle",
          "ctxPct": 12,
          "created": 1792064094,
          "attached": 0,
          "running": true,
          "dead": false,
          "registered": true,
          "port": 39170
        },
        {
          "id": "stream",
          "sessionId": "c5abd51b-9519-44a8-8ecf-caaa6eac200b",
          "path": "~/dev/alpha/.claude/worktrees/stream",
          "type": "build",
          "worktree": "~/dev/alpha/.claude/worktrees/stream",
          "branch": "change/stream",
          "status": "busy",
          "ctxPct": 88,
          "created": 1792064094,
          "attached": 0,
          "running": true,
          "dead": false,
          "registered": true,
          "port": 39130
        },
        {
          "id": "waiting",
          "sessionId": "0a189007-0058-49f5-9351-dbbb2ccec9c7",
          "path": "~/dev/alpha/.claude/worktrees/waiting",
          "type": "build",
          "worktree": "~/dev/alpha/.claude/worktrees/waiting",
          "branch": "change/waiting",
          "status": "waiting",
          "waitingFor": "permission: Bash(npm run test:e2e)",
          "ctxPct": 31,
          "created": 1792064094,
          "attached": 0,
          "running": true,
          "dead": false,
          "registered": true,
          "port": 39120
        },
        {
          "id": "working",
          "sessionId": "cf559cb3-f4ad-413b-9a80-67e7af0a6566",
          "path": "~/dev/alpha/.claude/worktrees/working",
          "type": "build",
          "worktree": "~/dev/alpha/.claude/worktrees/working",
          "branch": "change/working",
          "status": "busy",
          "ctxPct": 64,
          "created": 1792064094,
          "attached": 0,
          "running": true,
          "dead": false,
          "registered": true,
          "port": 39110
        }
      ],
      "laneTypes": [
        {
          "name": "build",
          "prefix": "change/",
          "model": "opus",
          "effort": "high"
        },
        {
          "name": "fix",
          "prefix": "fix/",
          "model": "sonnet"
        },
        {
          "name": "orchestrator",
          "prefix": ""
        },
        {
          "name": "ship",
          "prefix": "ship/",
          "model": "sonnet"
        }
      ],
      "tmuxSocket": "ux-p45-1",
      "laneBase": "main",
      "worktreeRoot": "~/dev/alpha/.claude/worktrees",
      "trends": {
        "burnWindow": "five_hour",
        "costToday": 0,
        "autocompactPct": 95
      },
      "flow": {
        "window": "30d",
        "items": [
          {
            "key": "flow.cycle_time",
            "value": 7.5,
            "series": [
              9,
              8,
              7,
              7,
              6,
              8,
              7,
              8,
              7,
              7
            ],
            "source": "project"
          },
          {
            "key": "flow.merge_frequency",
            "value": 2.8,
            "series": [
              2.3,
              4.7,
              2.3,
              0,
              2.3,
              4.7,
              2.3,
              2.3,
              4.7,
              2.3
            ],
            "source": "project"
          },
          {
            "key": "flow.change_fail_rate",
            "value": 8.3,
            "series": [
              0,
              0,
              50,
              0,
              0,
              0,
              0,
              0,
              0,
              0
            ],
            "source": "project"
          },
          {
            "key": "cost.per_week",
            "value": 37.9,
            "series": [
              30,
              35,
              41,
              38,
              40,
              36,
              39,
              37,
              42,
              38
            ],
            "source": "project"
          }
        ],
        "any": true
      },
      "economy": {
        "active": true,
        "since": 1792064098000,
        "reason": "5-hour quota 42% ≥ 40%",
        "threshold": 40,
        "roles": [
          {
            "role": "mechanic",
            "to": "haiku, low"
          },
          {
            "role": "scribe",
            "to": "sonnet, low"
          }
        ]
      },
      "approvals": {
        "add-group-card": false,
        "budget": true,
        "ready": true,
        "working": true
      },
      "remoteControl": "lanes",
      "done": [],
      "alerts": [
        {
          "key": "context:c5abd51b-9519-44a8-8ecf-caaa6eac200b",
          "kind": "context",
          "severity": "warn",
          "lane": "~/dev/alpha/.claude/worktrees/stream",
          "name": "stream",
          "terminal": "stream",
          "session": "c5abd51b-9519-44a8-8ecf-caaa6eac200b",
          "text": "context at 88% (alert at 85%)",
          "since": 1792064138748
        },
        {
          "key": "approval_wait:add-group-card",
          "kind": "approval_wait",
          "severity": "warn",
          "text": "change add-group-card has waited 3d for approval (no Approved line; alert at 1d)",
          "since": 1791804840000
        }
      ],
      "templates": [
        {
          "id": "propose",
          "title": "Propose a roadmap row",
          "laneType": "build",
          "branchPattern": "change/{name}",
          "firstPrompt": "propose {name}",
          "needsIssue": false,
          "suggests": true
        },
        {
          "id": "fix",
          "title": "Fix an issue",
          "laneType": "fix",
          "branchPattern": "fix/{name}",
          "firstPrompt": "fix {issue}",
          "needsIssue": true
        }
      ],
      "suggestions": {
        "propose": {
          "source": {
            "ok": true,
            "pending": false,
            "at": 1792064096959
          },
          "items": [
            {
              "name": "add-group-card",
              "title": "one golfer enters the card",
              "detail": "2C.27"
            },
            {
              "name": "fix-offline-sync",
              "title": "sync after airplane mode",
              "detail": "2C.31"
            }
          ]
        }
      },
      "queues": [
        {
          "id": "gate",
          "title": "Gate",
          "lock": "~/dev/alpha/.git/stridal/gate.lock",
          "hasCommand": false,
          "held": false,
          "waiters": []
        }
      ],
      "queuesSource": {
        "ok": true,
        "pending": false,
        "at": 1792064093942
      },
      "thresholds": {
        "idleMinutes": 30,
        "contextPct": 85,
        "fiveHourPct": 90,
        "waitingSeconds": 120,
        "notify": false,
        "quotaGuardPct": 95,
        "approvalWaitHours": 24,
        "staleDays": 3
      },
      "restorable": [
        "orphan"
      ],
      "observe": {
        "overflowDrops": 0,
        "malformedDrops": 0,
        "agentsPolls": 20,
        "agentsPollMs": 96,
        "agentsPollAvgMs": 68,
        "agentsPollMaxMs": 187,
        "agentsIntervalMs": 5000,
        "agentsFilter": "--cwd ~/dev/alpha (cross-checked every 5m0s)",
        "notifySent": 0,
        "notifyFailed": 0,
        "hookEvents": 35,
        "statusPosts": 81,
        "droppedForeign": 0,
        "droppedUnknownEvent": 0,
        "droppedEnded": 0,
        "unknownNotifications": 0,
        "claudeVersion": "2.1.284",
        "verifiedOn": "2.1.284",
        "versionSource": {
          "ok": true,
          "pending": false,
          "at": 1792064094048
        },
        "notifier": {
          "day": "2026-10-15",
          "interrupts": 0,
          "suppressed": 0,
          "deferred": 0
        }
      },
      "trust": {
        "trusted": true,
        "hash": "446fa0f183dd48c70f926c48cce4f6731e0e693a3b66b03e9bebaa4cb91ddc40",
        "prev": "446fa0f183dd48c70f926c48cce4f6731e0e693a3b66b03e9bebaa4cb91ddc40",
        "path": "~/dev/alpha/.stridal/panel.json"
      },
      "warnings": [],
      "warningItems": [],
      "verification": {
        "confirmed": 0,
        "needed": 3
      }
    },
    "beta": {
      "name": "Beta",
      "projectId": "beta",
      "root": "~/dev/beta",
      "now": 1792064148710,
      "startedAt": 1792064093904,
      "lanes": [
        {
          "id": "~/dev/beta",
          "path": "~/dev/beta",
          "branch": "main",
          "type": "orchestrator",
          "name": "beta-one",
          "status": "idle",
          "ctxPct": 12,
          "subagents": [],
          "finishedSubagents": [],
          "metrics": {
            "ctxPct": 12,
            "ctxWindow": 200000,
            "inputTokens": 128000,
            "outputTokens": 9100,
            "cacheReadTokens": 26394,
            "cacheWriteTokens": 10579,
            "cacheHitRatio": 0.92,
            "cacheWarm": true,
            "cacheExpiresAt": 1792065944000,
            "cacheMisses": 1,
            "cacheRequests": 14,
            "recacheTokens": 36983,
            "linesAdded": 412,
            "linesRemoved": 97,
            "durationMs": 1800000,
            "apiDurationMs": 600000,
            "costUsd": 0.2,
            "thinking": true,
            "fastMode": false,
            "outputStyle": "default",
            "statusAt": 1792064144694,
            "turns": 1,
            "subagentsRunning": 0,
            "subagentsFinished": 0,
            "state": "idle",
            "stateSince": 1792064099704
          },
          "timeline": [
            {
              "state": "idle",
              "from": 1792064103925
            }
          ],
          "head": "57c7c23e8fb38c032ce781fe54e91f81a44156b1",
          "lastEvent": "Stop",
          "lastEventAt": 1792064099513,
          "lastHookAt": 1792064099513,
          "stale": false,
          "sessions": [
            {
              "id": "e6479329-1558-4693-9395-785138ed3bc2",
              "name": "beta-one",
              "pid": 97463,
              "kind": "interactive",
              "status": "idle",
              "ctxPct": 12,
              "model": "Opus 4.1",
              "estCostUsd": 0.2,
              "startedAt": 1792064095472,
              "metrics": {
                "ctxPct": 12,
                "ctxWindow": 200000,
                "inputTokens": 128000,
                "outputTokens": 9100,
                "cacheReadTokens": 26394,
                "cacheWriteTokens": 10579,
                "cacheHitRatio": 0.92,
                "cacheWarm": true,
                "cacheExpiresAt": 1792065944000,
                "cacheMisses": 1,
                "cacheRequests": 14,
                "recacheTokens": 36983,
                "linesAdded": 412,
                "linesRemoved": 97,
                "durationMs": 1800000,
                "apiDurationMs": 600000,
                "costUsd": 0.2,
                "thinking": true,
                "fastMode": false,
                "outputStyle": "default",
                "statusAt": 1792064144694,
                "turns": 1,
                "subagentsRunning": 0,
                "subagentsFinished": 0,
                "state": "idle",
                "stateSince": 1792064099704
              },
              "stale": false
            }
          ],
          "terminal": "beta-one"
        }
      ],
      "quietWorktrees": [],
      "quota": {
        "windows": [
          {
            "key": "five_hour",
            "label": "5-hour",
            "pct": 42,
            "resetsAt": 1792071348
          },
          {
            "key": "seven_day",
            "label": "7-day",
            "pct": 61,
            "resetsAt": 1792323348
          }
        ],
        "at": 1792064148643,
        "fromSession": "0a189007-0058-49f5-9351-dbbb2ccec9c7",
        "account": "eaa3c51c8f4c",
        "fiveHour": 42,
        "sevenDay": 61,
        "fiveHourResetsAt": 1792071348,
        "sevenDayResetsAt": 1792323348
      },
      "account": {
        "mode": "subscription",
        "plan": "Max",
        "method": "claude.ai",
        "quotaMode": "windows",
        "source": {
          "ok": true,
          "pending": false,
          "at": 1792064094045
        }
      },
      "estCostUsd": 0.2,
      "needsYou": [],
      "cards": [
        {
          "id": "todo",
          "title": "To do",
          "source": {
            "ok": true,
            "pending": false,
            "at": 1792064093930
          },
          "output": {
            "kind": "text",
            "lines": [
              "Ship the thing",
              "Write the docs"
            ]
          },
          "pin": true
        }
      ],
      "prs": [],
      "feed": [
        {
          "at": 1792064099704,
          "lane": "~/dev/beta",
          "name": "beta-one",
          "type": "orchestrator",
          "session": "e6479329-1558-4693-9395-785138ed3bc2",
          "event": "session",
          "detail": "seen · idle"
        },
        {
          "at": 1792064099513,
          "lane": "~/dev/beta",
          "name": "beta-one",
          "type": "orchestrator",
          "session": "e6479329-1558-4693-9395-785138ed3bc2",
          "event": "Stop",
          "detail": "Done."
        },
        {
          "at": 1792064099491,
          "lane": "~/dev/beta",
          "name": "beta-one",
          "type": "orchestrator",
          "session": "e6479329-1558-4693-9395-785138ed3bc2",
          "event": "UserPromptSubmit",
          "detail": "(ux harness)"
        }
      ],
      "banners": [],
      "bannerItems": [],
      "agentsReadAt": 1792064147408,
      "sources": {
        "agents": {
          "ok": true,
          "pending": false,
          "at": 1792064147408
        },
        "prs": {
          "ok": true,
          "pending": false,
          "at": 1792064094016
        },
        "tmux": {
          "ok": true,
          "pending": false,
          "at": 1792064147680
        },
        "worktrees": {
          "ok": true,
          "pending": false,
          "at": 1792064143958
        }
      },
      "hookEvents": 2,
      "statusPosts": 10,
      "dropped": 0,
      "terminals": [
        {
          "id": "beta-one",
          "sessionId": "e6479329-1558-4693-9395-785138ed3bc2",
          "path": "~/dev/beta",
          "type": "orchestrator",
          "worktree": "~/dev/beta",
          "branch": "main",
          "status": "idle",
          "ctxPct": 12,
          "created": 1792064095,
          "attached": 0,
          "running": true,
          "dead": false,
          "registered": true
        }
      ],
      "laneTypes": [
        {
          "name": "build",
          "prefix": "feature/"
        },
        {
          "name": "orchestrator",
          "prefix": ""
        }
      ],
      "tmuxSocket": "ux-p45-2",
      "laneBase": "main",
      "worktreeRoot": "~/dev/beta/.claude/worktrees",
      "trends": {
        "burnWindow": "five_hour",
        "costToday": 0,
        "autocompactPct": 95
      },
      "economy": {
        "active": true,
        "since": 1792064098000,
        "reason": "5-hour quota 42% ≥ 40%",
        "threshold": 40,
        "roles": [],
        "rolesNote": "no .claude/model-roles.json in this project, so no role is named"
      },
      "approvals": {},
      "remoteControl": "lanes",
      "done": [],
      "alerts": [],
      "templates": [],
      "suggestions": {},
      "queues": [],
      "queuesSource": {
        "ok": true,
        "pending": false
      },
      "thresholds": {
        "idleMinutes": 30,
        "contextPct": 85,
        "fiveHourPct": 90,
        "waitingSeconds": 120,
        "notify": false,
        "quotaGuardPct": 95,
        "approvalWaitHours": 24,
        "staleDays": 3
      },
      "restorable": [],
      "observe": {
        "overflowDrops": 0,
        "malformedDrops": 0,
        "agentsPolls": 18,
        "agentsPollMs": 91,
        "agentsPollAvgMs": 101,
        "agentsPollMaxMs": 155,
        "agentsIntervalMs": 2000,
        "agentsFilter": "--cwd ~/dev/beta (cross-checked every 5m0s)",
        "notifySent": 0,
        "notifyFailed": 0,
        "hookEvents": 2,
        "statusPosts": 10,
        "droppedForeign": 0,
        "droppedUnknownEvent": 0,
        "droppedEnded": 0,
        "unknownNotifications": 0,
        "claudeVersion": "2.1.284",
        "verifiedOn": "2.1.284",
        "versionSource": {
          "ok": true,
          "pending": false,
          "at": 1792064094048
        },
        "notifier": {
          "day": "2026-10-15",
          "interrupts": 0,
          "suppressed": 0,
          "deferred": 0
        }
      },
      "trust": {
        "trusted": true,
        "hash": "8d9dd59edb701b97860c27773b35ce2cd85a9bedbec0f160ddad975577383296",
        "prev": "8d9dd59edb701b97860c27773b35ce2cd85a9bedbec0f160ddad975577383296",
        "path": "~/dev/beta/.stridal/panel.json"
      },
      "warnings": [],
      "warningItems": [],
      "verification": {
        "confirmed": 0,
        "needed": 3
      }
    },
    "gamma": {
      "code": "no-project",
      "error": "no such project: gamma",
      "ok": false
    },
    "zeta": {
      "name": "Zeta",
      "projectId": "zeta",
      "root": "~/dev/zeta",
      "now": 1792064148721,
      "startedAt": 1792064093904,
      "lanes": [],
      "quietWorktrees": [
        {
          "id": "~/dev/zeta",
          "path": "~/dev/zeta",
          "branch": "main",
          "type": "orchestrator",
          "name": "main",
          "status": "none",
          "ctxPct": null,
          "subagents": [],
          "finishedSubagents": [],
          "metrics": {
            "subagentsRunning": 0,
            "subagentsFinished": 0
          },
          "head": "e7f40e0d3ef6a771706258138aeaa40e0c7462dc",
          "stale": false,
          "sessions": []
        }
      ],
      "quota": {
        "windows": [
          {
            "key": "five_hour",
            "label": "5-hour",
            "pct": 42,
            "resetsAt": 1792071348
          },
          {
            "key": "seven_day",
            "label": "7-day",
            "pct": 61,
            "resetsAt": 1792323348
          }
        ],
        "at": 1792064148643,
        "fromSession": "0a189007-0058-49f5-9351-dbbb2ccec9c7",
        "account": "eaa3c51c8f4c",
        "fiveHour": 42,
        "sevenDay": 61,
        "fiveHourResetsAt": 1792071348,
        "sevenDayResetsAt": 1792323348
      },
      "account": {
        "mode": "subscription",
        "plan": "Max",
        "method": "claude.ai",
        "quotaMode": "windows",
        "source": {
          "ok": true,
          "pending": false,
          "at": 1792064094045
        }
      },
      "estCostUsd": null,
      "needsYou": [],
      "cards": [
        {
          "id": "health",
          "title": "Health",
          "source": {
            "ok": false,
            "pending": false,
            "error": "not run: panel.json changed since you trusted it; run `stridal panel trust`",
            "at": 1792064093924
          }
        }
      ],
      "prs": [],
      "feed": [],
      "banners": [
        "panel.json is not trusted yet (sha256 d6f62641d9f6). Its commands (cards, queue RUN) and templates are off until you review it and run `stridal panel trust` (or restart with --trust-config)."
      ],
      "bannerItems": [
        {
          "kind": "untrusted",
          "text": "panel.json is not trusted yet (sha256 d6f62641d9f6). Its commands (cards, queue RUN) and templates are off until you review it and run `stridal panel trust` (or restart with --trust-config)."
        }
      ],
      "agentsReadAt": 1792064139435,
      "sources": {
        "agents": {
          "ok": true,
          "pending": false,
          "at": 1792064139435
        },
        "prs": {
          "ok": true,
          "pending": false,
          "at": 1792064094015
        },
        "tmux": {
          "ok": true,
          "pending": false,
          "at": 1792064143973
        },
        "worktrees": {
          "ok": true,
          "pending": false,
          "at": 1792064143958
        }
      },
      "hookEvents": 0,
      "statusPosts": 0,
      "dropped": 0,
      "terminals": [],
      "laneTypes": [
        {
          "name": "orchestrator",
          "prefix": ""
        }
      ],
      "tmuxSocket": "ux-p45-6",
      "laneBase": "main",
      "worktreeRoot": "~/dev/zeta/.claude/worktrees",
      "trends": {
        "burnWindow": "five_hour",
        "autocompactPct": 95
      },
      "economy": {
        "active": true,
        "since": 1792064098000,
        "reason": "5-hour quota 42% ≥ 40%",
        "threshold": 40,
        "roles": [],
        "rolesNote": "no .claude/model-roles.json in this project, so no role is named"
      },
      "approvals": {},
      "remoteControl": "lanes",
      "done": [],
      "alerts": [],
      "templates": [],
      "suggestions": {},
      "queues": [],
      "queuesSource": {
        "ok": true,
        "pending": false
      },
      "thresholds": {
        "idleMinutes": 30,
        "contextPct": 85,
        "fiveHourPct": 90,
        "waitingSeconds": 120,
        "notify": false,
        "quotaGuardPct": 95,
        "approvalWaitHours": 24,
        "staleDays": 3
      },
      "restorable": [],
      "observe": {
        "overflowDrops": 0,
        "malformedDrops": 0,
        "agentsPolls": 4,
        "agentsPollMs": 98,
        "agentsPollAvgMs": 79,
        "agentsPollMaxMs": 109,
        "agentsIntervalMs": 15000,
        "agentsFilter": "--cwd ~/dev/zeta (cross-checked every 5m0s)",
        "notifySent": 0,
        "notifyFailed": 0,
        "hookEvents": 0,
        "statusPosts": 0,
        "droppedForeign": 0,
        "droppedUnknownEvent": 0,
        "droppedEnded": 0,
        "unknownNotifications": 0,
        "claudeVersion": "2.1.284",
        "verifiedOn": "2.1.284",
        "versionSource": {
          "ok": true,
          "pending": false,
          "at": 1792064094048
        },
        "notifier": {
          "day": "2026-10-15",
          "interrupts": 0,
          "suppressed": 0,
          "deferred": 0
        }
      },
      "trust": {
        "trusted": false,
        "hash": "d6f62641d9f685271fb5043eb39d1b680b2dd566e7d90a66d32e0edcf6533ff1",
        "path": "~/dev/zeta/.stridal/panel.json"
      },
      "warnings": [],
      "warningItems": [],
      "verification": {
        "confirmed": 0,
        "needed": 3
      }
    }
  },
  "terminal": {
    "working": "╭──────────────────────────────────────────────────────╮\n│ ✻ Welcome to Claude Code (UX harness fake)              │\n│   lane working              role busy                 │\n╰──────────────────────────────────────────────────────╯\n cwd: ~/dev/alpha/.claude/worktrees/working\n\nSelectable words on this row\nPR: https://github.com/o/r/pull/12 is open.\nDocs here and https://example.com/same\n  ⎿ Read(src/app.ts) · 212 lines\n  ✓ Tests: 128 passed, 0 failed (4.2s)\n\n────────────────────────────────────────────────────────\n>\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n",
    "waiting": "╭──────────────────────────────────────────────────────╮\n│ ✻ Welcome to Claude Code (UX harness fake)              │\n│   lane waiting              role waiting              │\n╰──────────────────────────────────────────────────────╯\n cwd: ~/dev/alpha/.claude/worktrees/waiting\n\nSelectable words on this row\nPR: https://github.com/o/r/pull/12 is open.\nDocs here and https://example.com/same\n  ⎿ Read(src/app.ts) · 212 lines\n  ✓ Tests: 128 passed, 0 failed (4.2s)\n\n╭─ Permission ─────────────────────────────────╮\n│ Bash(npm run test:e2e)                        │\n│ ❯ 1. Yes   2. Yes, and don't ask again   3. No │\n╰───────────────────────────────────────────────╯\n────────────────────────────────────────────────────────\n>\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n",
    "stream": "07:35:28 [build] step 00085 compiling module-085.ts … ok (595 ms)\n07:35:29 [build] step 00086 compiling module-086.ts … ok (602 ms)\n07:35:29 [build] step 00087 compiling module-087.ts … ok (609 ms)\n07:35:29 [build] step 00088 compiling module-088.ts … ok (616 ms)\n07:35:30 [build] step 00089 compiling module-089.ts … ok (623 ms)\n07:35:30 [build] step 00090 compiling module-090.ts … ok (630 ms)\n07:35:31 [build] step 00091 compiling module-091.ts … ok (637 ms)\n07:35:31 [build] step 00092 compiling module-092.ts … ok (644 ms)\n07:35:31 [build] step 00093 compiling module-093.ts … ok (651 ms)\n07:35:32 [build] step 00094 compiling module-094.ts … ok (658 ms)\n07:35:32 [build] step 00095 compiling module-095.ts … ok (665 ms)\n07:35:33 [build] step 00096 compiling module-096.ts … ok (672 ms)\n07:35:33 [build] step 00097 compiling module-097.ts … ok (679 ms)\n07:35:33 [build] step 00098 compiling module-098.ts … ok (686 ms)\n07:35:34 [build] step 00099 compiling module-099.ts … ok (693 ms)\n07:35:34 [build] step 00100 compiling module-100.ts … ok (700 ms)\n07:35:35 [build] step 00101 compiling module-101.ts … ok (707 ms)\n07:35:35 [build] step 00102 compiling module-102.ts … ok (714 ms)\n07:35:36 [build] step 00103 compiling module-103.ts … ok (721 ms)\n07:35:36 [build] step 00104 compiling module-104.ts … ok (728 ms)\n07:35:36 [build] step 00105 compiling module-105.ts … ok (735 ms)\n07:35:37 [build] step 00106 compiling module-106.ts … ok (742 ms)\n07:35:37 [build] step 00107 compiling module-107.ts … ok (749 ms)\n07:35:38 [build] step 00108 compiling module-108.ts … ok (756 ms)\n07:35:38 [build] step 00109 compiling module-109.ts … ok (763 ms)\n07:35:38 [build] step 00110 compiling module-110.ts … ok (770 ms)\n07:35:39 [build] step 00111 compiling module-111.ts … ok (777 ms)\n07:35:39 [build] step 00112 compiling module-112.ts … ok (784 ms)\n07:35:40 [build] step 00113 compiling module-113.ts … ok (791 ms)\n07:35:40 [build] step 00114 compiling module-114.ts … ok (798 ms)\n07:35:40 [build] step 00115 compiling module-115.ts … ok (805 ms)\n07:35:41 [build] step 00116 compiling module-116.ts … ok (812 ms)\n07:35:41 [build] step 00117 compiling module-117.ts … ok (819 ms)\n07:35:42 [build] step 00118 compiling module-118.ts … ok (826 ms)\n07:35:42 [build] step 00119 compiling module-119.ts … ok (833 ms)\n07:35:42 [build] step 00120 compiling module-120.ts … ok (840 ms)\n07:35:43 [build] step 00121 compiling module-121.ts … ok (847 ms)\n07:35:43 [build] step 00122 compiling module-122.ts … ok (854 ms)\n07:35:44 [build] step 00123 compiling module-123.ts … ok (861 ms)\n07:35:44 [build] step 00124 compiling module-124.ts … ok (868 ms)\n07:35:44 [build] step 00125 compiling module-125.ts … ok (875 ms)\n07:35:45 [build] step 00126 compiling module-126.ts … ok (882 ms)\n07:35:45 [build] step 00127 compiling module-127.ts … ok (889 ms)\n07:35:46 [build] step 00128 compiling module-128.ts … ok (896 ms)\n07:35:46 [build] step 00129 compiling module-129.ts … ok (3 ms)\n07:35:47 [build] step 00130 compiling module-130.ts … ok (10 ms)\n07:35:47 [build] step 00131 compiling module-131.ts … ok (17 ms)\n07:35:47 [build] step 00132 compiling module-132.ts … ok (24 ms)\n07:35:48 [build] step 00133 compiling module-133.ts … ok (31 ms)\n07:35:48 [build] step 00134 compiling module-134.ts … ok (38 ms)\n"
  }
}
;
