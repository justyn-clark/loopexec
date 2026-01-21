# Integrations

This document describes how to integrate Spindle with external systems.

---

## JSON Output

Spindle provides structured JSON output via the `emit` command.

### Basic Usage

```bash
spindle emit --json
```

Output structure:
```json
{
  "status": "running",
  "current_task": {
    "id": "task-3",
    "title": "Implement feature X",
    "status": "in_progress"
  },
  "plan_summary": {
    "total": 5,
    "completed": 2,
    "in_progress": 1,
    "pending": 2,
    "blocked": 0
  },
  "last_checkpoint": {
    "task_id": "task-2",
    "status": "completed",
    "timestamp": "2026-01-21T00:05:30Z"
  },
  "replayId": "86f57e500b5ec132..."
}
```

### Filtering Output

Emit specific sections:
```bash
spindle emit --json --section plan
spindle emit --json --section progress
spindle emit --json --section status
```

---

## Bash Integration

### Simple Wrapper Loop

Run Spindle in a bash loop with manual intervention:

```bash
#!/bin/bash
set -e

while true; do
    # Check state
    small check --strict || { echo "Strict check failed"; exit 1; }

    # Get status
    status=$(spindle emit --json | jq -r '.status')

    if [ "$status" = "complete" ]; then
        echo "All tasks complete"
        small handoff --summary "Loop completed successfully"
        break
    fi

    if [ "$status" = "blocked" ]; then
        echo "Blocked - manual intervention required"
        small handoff --summary "Loop blocked, awaiting intervention"
        break
    fi

    # Run one iteration
    spindle run

    # Optional: add delay between iterations
    sleep 1
done
```

### Status Polling

Poll Spindle status from another process:

```bash
#!/bin/bash

# Check if work is complete
is_complete() {
    local status=$(spindle emit --json | jq -r '.status')
    [ "$status" = "complete" ]
}

# Get current task
current_task() {
    spindle emit --json | jq -r '.current_task.title // "none"'
}

# Get progress percentage
progress_percent() {
    local json=$(spindle emit --json)
    local completed=$(echo "$json" | jq '.plan_summary.completed')
    local total=$(echo "$json" | jq '.plan_summary.total')
    echo "scale=0; $completed * 100 / $total" | bc
}

echo "Current task: $(current_task)"
echo "Progress: $(progress_percent)%"
```

### Exit Code Handling

Spindle exit codes:

| Code | Meaning |
|------|---------|
| 0 | Success (iteration complete or loop finished) |
| 1 | Execution failure (command failed) |
| 2 | Gate failure (strict check failed) |
| 3 | Configuration error |

Handle in bash:
```bash
spindle run
case $? in
    0) echo "Success" ;;
    1) echo "Execution failed - check evidence" ;;
    2) echo "Gate failed - fix SMALL state" ;;
    3) echo "Configuration error" ;;
esac
```

---

## CI Integration

### GitHub Actions

Run Spindle as a CI check:

```yaml
name: Spindle Check
on: [push, pull_request]

jobs:
  spindle:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Install SMALL CLI
        run: |
          # Install small CLI
          curl -fsSL https://small.dev/install.sh | sh

      - name: Install Spindle
        run: |
          # Install spindle
          go install github.com/your-org/spindle/cmd/spindle@latest

      - name: Validate SMALL State
        run: small check --strict

      - name: Check Spindle Status
        run: |
          spindle emit --json > spindle-status.json
          cat spindle-status.json

      - name: Fail on Blocked Tasks
        run: |
          blocked=$(jq '.plan_summary.blocked' spindle-status.json)
          if [ "$blocked" -gt 0 ]; then
            echo "Blocked tasks detected"
            exit 1
          fi
```

### Pre-commit Hook

Validate state before commit:

```bash
#!/bin/bash
# .git/hooks/pre-commit

# Skip if not a SMALL workspace
[ -d ".small" ] || exit 0

# Validate state
if ! small check --strict; then
    echo "SMALL strict check failed. Fix before committing."
    exit 1
fi

# Check for in-progress tasks
in_progress=$(small status --json | jq '.plan.tasks_by_status.in_progress // 0')
if [ "$in_progress" -gt 0 ]; then
    echo "Warning: Tasks are in progress. Consider checkpointing first."
fi
```

### Post-merge Hook

Update handoff after merge:

```bash
#!/bin/bash
# .git/hooks/post-merge

[ -d ".small" ] || exit 0

# Regenerate handoff with new state
small check --strict && small handoff --summary "Post-merge state sync"
```

---

## Local Developer Usage

### Interactive Session

Start a development session:

```bash
# Begin session
small status --json
small check --strict

# See what to work on
spindle emit --json | jq '.current_task'

# Run one step
spindle run

# Check results
small status --json

# End session
small check --strict
small handoff --summary "Completed feature X implementation"
```

### Watch Mode

Monitor Spindle in a terminal:

```bash
watch -n 5 'spindle emit --json | jq "."'
```

### Debug Mode

Run with verbose output:

```bash
spindle run --verbose
```

Output includes:
- Task selection reasoning
- Substrate configuration
- Command execution details
- Checkpoint contents

---

## Deferred Integrations

The following integrations are explicitly out of scope for this document:

### Direct Model Invocation

Spindle does not invoke AI models directly. Model invocation is the responsibility of the agent or automation layer above Spindle.

### Prompt Management

Spindle does not manage prompts, context windows, or model configurations. These concerns belong to the agent layer.

### Agent Orchestration

Spindle does not orchestrate multiple agents or manage agent lifecycles. It executes work; it does not decide what work to do.

These separations are intentional. Spindle provides execution discipline. Higher layers provide intelligence.

---

## Integration Checklist

When integrating Spindle:

1. Ensure SMALL CLI is installed and accessible
2. Verify `.small/` workspace is initialized
3. Confirm strict check passes before starting
4. Use `spindle emit --json` for programmatic access
5. Handle all exit codes appropriately
6. Always run strict check after failures
7. Generate handoff at session boundaries
