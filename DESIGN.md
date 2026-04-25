# vibedrive create

## Product Definition

`vibedrive create` helps a user turn a rough product idea into an implementation-ready `DESIGN.md` for coding agents. It brings the useful parts of Gary Tan's gstack `/office-hours`, `/plan-design-review`, and `/plan-eng-review` workflows into vibedrive as a guided command.

The command is for agent-driven development. The output should be good enough that a coding agent can continue into planning with minimal extra clarification.

The flow has three authoring stages:

1. Product Definition
2. UX Review
3. Technical Review

After those stages, the user can hand off to the existing planning flow that currently lives behind `vibedrive init`.

### Product Definition

Product Definition replaces the gstack-style office-hours phase. It is mostly about features, requirements, product goals, and scope.

The author agent should:

- inspect the workspace before interviewing the user
- interact directly with the user in the agent TUI
- ask questions until it believes the product requirements are ready for the next stage
- push back when the idea has product problems, contradictions, over-broad scope, unclear users, missing workflows, or weak success criteria
- write or update the root `DESIGN.md`

This phase should keep going until the idea is ready for agent planning. It should not stop after a fixed checklist if important questions remain.

### UX Review

UX Review replaces the gstack-style design review phase. It should update `DESIGN.md` based on the project type and what matters for the user experience.

Depending on the project, it may cover:

- user journeys and workflows
- interaction design
- visual style
- layout and responsive behavior
- accessibility
- empty, loading, and error states
- content and terminology
- scope tradeoffs from a product/design perspective

The UX Review author should inspect the workspace and current `DESIGN.md` before editing.

### Technical Review

Technical Review replaces the gstack-style engineering review phase. It should explain how the project can be implemented, but it should not produce a detailed task-by-task plan yet. The detailed planning stage happens afterward through `vibedrive init`.

The Technical Review may cover:

- likely architecture
- data model and API contracts
- integration points
- implementation risks
- edge cases
- test strategy
- rollout or migration notes
- known unknowns
- a rough implementation approach

The Technical Review author should inspect the workspace and current `DESIGN.md` before editing.

## DESIGN.md Behavior

`DESIGN.md` lives at the root of the target workspace.

All create-stage output goes into the same `DESIGN.md`. Later stages may edit, rewrite, or reorganize earlier content when that produces a better document. Existing `DESIGN.md` content is part of the input to every stage.

The stage agents are trusted to decide how best to update the document. The command does not need to enforce section-level preservation or append-only behavior.

`vibedrive create` should not fail just because `DESIGN.md` already exists. When the command starts, it should allow the user to choose any stage, using the existing document as context.

Planning should only be offered from the create menu when `DESIGN.md` exists, because planning needs a document to work from. Running `vibedrive init` directly should continue to work even when there is no `DESIGN.md`.

## Stage Flow

When `vibedrive create` starts, it should show a simple numbered menu. A richer selector can be added later, but the first version can avoid a new dependency.

The menu should let the user start at any stage:

- Product Definition
- UX Review
- Technical Review
- Planning, only when `DESIGN.md` exists
- Stop

After each author run completes successfully, vibedrive updates create state and returns to the menu. That menu is also the pause point where the user can manually edit `DESIGN.md` before choosing what happens next.

After Product Definition, useful next choices include:

- continue to UX Review
- skip to Technical Review
- skip directly to Planning
- stop

After UX Review, useful next choices include:

- continue to Technical Review
- skip directly to Planning
- stop

After Technical Review, useful next choices include:

- continue to Planning
- stop

The command should support Ctrl-C cleanly during menus and agent runs. It should preserve whatever `DESIGN.md` and state already exist.

## Critic Flow

`vibedrive create` has two roles:

- `--author claude|codex`
- `--critic claude|codex`

Defaults:

- author: `codex`
- critic: `claude`

The author owns all create stages. The critic gives a separate opinion when the user asks for one.

After each author stage, vibedrive asks whether the user wants a second opinion. If yes:

1. Run the critic in a fresh instance with no author conversation context.
2. The critic reads the workspace, `DESIGN.md`, and the critic prompt for that stage.
3. The critic output is visible in the terminal.
4. Pass the critic feedback to a fresh author instance.
5. Ask the author to decide what to do with the feedback and update `DESIGN.md`.
6. Return to the stage menu.

The critic does not edit `DESIGN.md` directly. Critic feedback does not need to be saved as a separate artifact; it only needs to be passed to the author during that run.

Every create-stage author run should use a fresh instance, including reruns of the same stage and author follow-up after critic feedback.

## Prompt Strategy

Create-stage prompts should be maintained as separate prompt definitions in the source code for clarity, but compiled into the binary for this version. Runtime prompt override files are not needed yet.

Prompt definitions should exist for:

- Product Definition author
- Product Definition critic
- UX Review author
- UX Review critic
- Technical Review author
- Technical Review critic
- Author follow-up from critic feedback

The prompts should tell each agent what to do directly. They do not need to explain the whole implementation architecture to the agent.

## State

Create state is stored in a hidden file inside the target workspace:

```json
{
  "last_stage": "product_definition"
}
```

The exact JSON should include only what is required for functionality. It does not need to store conversation answers, timestamps, or agent names.

State is updated when an author finishes a stage successfully, including an author run that incorporates critic feedback.

There is no special unfinished concept. Whenever the user runs `vibedrive create`, the command can read existing state if present and show the user a stage menu. The user may go forward, go backward, rerun completed stages, or go to planning when `DESIGN.md` exists.

## Planning Handoff

Planning is the existing init flow, using `DESIGN.md` as the only source.

When the user chooses Planning from `vibedrive create`, vibedrive should behave like:

```bash
vibedrive init --source DESIGN.md --author <same-author> --critic <same-critic>
```

The create command should give the user an option to stop instead of continuing into planning.

## Init Changes

The existing `vibedrive init --planner` terminology should be renamed to `--author`.

There is no need for backward compatibility. Remove `--planner`; do not keep it as a deprecated alias.

`vibedrive init` should also support:

```bash
--critic claude|codex
```

Defaults:

- author: `codex`
- critic: `claude`

The existing init flow currently has a second critical review prompt using the same selected planner. That should change:

1. The author creates `vibedrive-plan.yaml`.
2. The critic reviews the plan in a fresh instance with a fresh context.
3. The author runs again in a fresh instance, receives the critic feedback, and revises the plan.

If author and critic are the same agent type, init should still run both steps. The critic must be a new instance with new context.

README, command help, tests, and internal code names should be renamed from planner to author where practical. User-facing docs should not mention `--planner`.

## CLI Shape

Top-level commands should include:

```bash
vibedrive create [-workspace DIR] [--author claude|codex] [--critic claude|codex]
vibedrive init [-config vibedrive.yaml] [-workspace DIR] [--source PATH ...] [--author claude|codex] [--critic claude|codex] [--print-sources] [-force] [SOURCE]
```

`vibedrive create` does not need:

- `--dry-run`
- a special positional idea argument
- `--resume`

The user explains the idea interactively to the Product Definition author agent.

## Testing

Tests should cover orchestration with fake agent clients and fake menu input. They do not need to run real Claude or Codex CLIs.

Important cases:

- create writes or updates root `DESIGN.md` through author prompts
- create starts from any selected stage
- planning is hidden when `DESIGN.md` does not exist
- planning uses `DESIGN.md` as the only init source
- state is updated after successful author stage completion
- critic feedback is routed to a fresh author run and critic does not edit directly
- author and critic defaults are codex and claude
- init uses author, critic, and author-revision flow
- `--planner` is removed from help and parsing
- Ctrl-C/context cancellation preserves existing files and state
