#!/usr/bin/env node
// Piper statusline script — reads Claude Code statusline JSON from stdin,
// renders a colored status bar to stdout AND POSTs metadata to the Piper API.

import { execSync } from 'child_process';

// ANSI color helpers
const c = {
  reset: '\x1b[0m',
  bold: '\x1b[1m',
  dim: '\x1b[2m',
  red: '\x1b[31m',
  green: '\x1b[32m',
  yellow: '\x1b[33m',
  magenta: '\x1b[35m',
  cyan: '\x1b[36m',
  gray: '\x1b[90m',
};

let raw = '';
process.stdin.setEncoding('utf-8');
process.stdin.on('data', chunk => { raw += chunk; });
process.stdin.on('end', () => {
  let data;
  try {
    data = JSON.parse(raw);
  } catch {
    process.exit(0);
  }

  const model = (data.model && data.model.display_name) || '';
  const costUsd = (data.cost && data.cost.total_cost_usd) || 0;
  const contextPct = (data.context_window && data.context_window.used_percentage) || 0;
  const linesAdded = (data.cost && data.cost.total_lines_added) || 0;
  const linesRemoved = (data.cost && data.cost.total_lines_removed) || 0;
  const sep = ` ${c.dim}\u2502${c.reset} `;

  // — Git branch —
  let branch = '';
  try {
    branch = execSync('git branch --show-current 2>/dev/null', { encoding: 'utf8' }).trim();
  } catch { /* ignore */ }

  // — Build parts —
  const parts = [];

  // 1. Model (magenta, bold)
  if (model) {
    parts.push(`${c.bold}${c.magenta}${model}${c.reset}`);
  }

  // 2. Branch (cyan + powerline icon)
  if (branch) {
    parts.push(`${c.cyan}\ue0a0 ${branch}${c.reset}`);
  }

  // 3. Context bar — green < 50%, yellow 50-90%, red >= 90%
  const barLen = 10;
  const filled = Math.round(contextPct / 100 * barLen);
  const ctxColor = contextPct < 50 ? c.green : contextPct < 90 ? c.yellow : c.red;
  const bar = `${ctxColor}${'\u2588'.repeat(filled)}${c.gray}${'\u2591'.repeat(barLen - filled)}${c.reset}`;
  parts.push(`${bar} ${ctxColor}${Math.round(contextPct)}%${c.reset}`);

  // 4. Cost — green < $1, yellow $1-5, red >= $5
  const costColor = costUsd < 1 ? c.green : costUsd < 5 ? c.yellow : c.red;
  parts.push(`${costColor}$${costUsd.toFixed(2)}${c.reset}`);

  // 5. Lines changed (dim, only if non-zero)
  if (linesAdded || linesRemoved) {
    const lineParts = [];
    if (linesAdded) lineParts.push(`${c.green}+${linesAdded}${c.reset}`);
    if (linesRemoved) lineParts.push(`${c.red}-${linesRemoved}${c.reset}`);
    parts.push(lineParts.join(' '));
  }

  process.stdout.write(parts.join(sep));

  // — POST to Piper API if configured —
  const sessionName = process.env.PIPER_SESSION;
  const piperHost = process.env.PIPER_HOST;
  if (!sessionName || !piperHost) return;

  fetch(`http://${piperHost}/api/internal/session-meta`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      sessionName,
      timestamp: Date.now(),
      sessionId: data.session_id || '',
      model,
      costUsd,
      contextUsedPct: contextPct,
      durationMs: (data.cost && data.cost.total_duration_ms) || 0,
      rateLimits: data.rate_limits || null,
    }),
    signal: AbortSignal.timeout(3000),
  }).catch(() => {});
});
