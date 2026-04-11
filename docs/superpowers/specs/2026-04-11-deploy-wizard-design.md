# Deploy Wizard - Design Spec

## Overview

Replace the current single-form deploy slide panel with a 3-step wizard that validates before deploying, surfaces routing config, and streams deploy logs in real time.

## Current State

- SlidePanel with app name input + plain textarea for compose YAML
- Paste/upload toggle for compose input
- No validation before deploy (API exists but unused)
- Panel closes on success, no deploy progress visibility
- No registry selection, no label configuration

## Step 1: Name & Compose

### App Name
- Text input with real-time inline validation
- Pattern: `^[a-zA-Z0-9][a-zA-Z0-9._-]{0,62}$`
- Error message shown below input when invalid

### Compose Input
- Replace plain textarea with existing `YamlEditor` component (line numbers, tab support, scroll sync)
- Keep Paste/Upload toggle; upload populates the YamlEditor
- Auto-validate via `validateCompose` API on blur, debounced ~800ms
- Validation result banner above editor: green checkmark or red error list
- "Next" button disabled until name valid + compose valid

### Step Indicator
- Three-step indicator at top of panel: "Compose", "Review", "Deploy"
- Shows current step highlighted

## Step 2: Review & Configure

### Service Summary
- Client-side parse of YAML to extract service names, images, and exposed ports
- One card per service showing this info (read-only)
- If parsing fails, skip summary silently (server already validated)

### Registry Selector
- Shown only if any image references a non-Docker Hub registry (image contains a dot before first `/`)
- Dropdown populated from `listRegistries` API
- Optional, can skip if images are public

### Quick Labels (collapsed accordion, optional)
- "Configure Routing" section:
  - Domain input (generates `simpledeploy.domain`)
  - Port input (generates `simpledeploy.port`)
  - TLS toggle: letsencrypt/off (generates `simpledeploy.tls`)
- Labels injected into YAML before deploy by finding/adding `labels:` block on first service

### Navigation
- "Back" returns to step 1 (state preserved)
- "Deploy" proceeds to step 3

## Step 3: Deploy & Monitor

### Deploy Trigger
- Entering step 3 immediately calls `api.deploy()` and opens `deployLogsWs` WebSocket
- No manual start button

### Progress Display
- Status badge at top: "Deploying..." (amber pulse), "Success" (green), "Failed" (red)
- Current action label from WebSocket `OutputLine.action` field (e.g. "Pulling images...")
- Mini terminal log viewer (~300px) with dark background, auto-follow, monospace text
- Simplified vs LogViewer: no service tabs, no download, no timestamps toggle

### Completion
- **Success**: green banner + "View App" button (navigates to `/apps/{slug}`) + "Deploy Another" (resets wizard)
- **Failure**: red banner + "Back to Edit" button (returns to step 1, compose preserved for retry)

### Panel Behavior
- Panel stays open throughout deploy
- Clicking backdrop/X while deploying shows confirmation ("Deploy in progress, close anyway?")
- After completion, closes normally

## Architecture

### New Component
- `ui/src/components/DeployWizard.svelte`
- Contains all wizard state and step rendering
- Dashboard passes `open`, `onclose`, `onComplete` props

### State Management
- Single `step` reactive var (1/2/3)
- Each step rendered via `{#if step === N}` blocks
- All form state at component level so Back preserves everything
- Validation state: `nameValid`, `composeValid`, `validating`, `validationErrors`
- Deploy state: `deploying`, `deployStatus`, `deployLines`, `currentAction`

### Label Injection
- When user fills routing config in step 2, inject `simpledeploy.*` labels into YAML string
- Find first service's `labels:` key or append one
- Simple string manipulation, not full YAML AST

### Client-Side YAML Parsing
- Minimal: extract `services:` top-level keys, image values, ports arrays
- Regex/line-based, not a full parser
- Graceful failure: skip summary if parsing fails

### Deploy Log Mini-Viewer
- Inline in wizard step 3
- Reactive `lines` array fed by WebSocket
- Same dark terminal aesthetic as `LogViewer` component
- Auto-scroll to bottom on new lines

## Files Changed

| File | Change |
|------|--------|
| `ui/src/components/DeployWizard.svelte` | New: full wizard component |
| `ui/src/routes/Dashboard.svelte` | Remove inline deploy form, render `<DeployWizard>` instead |

## No Backend Changes

All required APIs exist:
- `POST /apps/deploy` - deploy app
- `POST /apps/validate-compose` - validate compose file
- `GET /registries` - list configured registries
- `WS /apps/{slug}/deploy-logs` - stream deploy output
