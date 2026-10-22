#!/usr/bin/env node
// Piper hook script — reads Claude Code hook event from stdin,
// POSTs session status to the Piper API.

const sessionName = process.env.PIPER_SESSION;
const piperHost = process.env.PIPER_HOST;
if (!sessionName || !piperHost) process.exit(0);

let raw = '';
process.stdin.setEncoding('utf-8');
process.stdin.on('data', chunk => { raw += chunk; });
process.stdin.on('end', () => {
  let input;
  try {
    input = JSON.parse(raw);
  } catch {
    process.exit(0);
  }

  const hookEvent = input.hook_event_name || '';
  let status = '';

  switch (hookEvent) {
    case 'UserPromptSubmit':
    case 'PreToolUse':
    case 'SubagentStart':
      status = 'working';
      break;
    case 'Stop':
      status = 'idle';
      break;
    case 'StopFailure':
      status = 'needs_input';
      break;
    case 'Notification':
      if (input.notification_type === 'permission_prompt') {
        status = 'needs_input';
      } else if (input.notification_type === 'idle_prompt') {
        status = 'idle';
      }
      break;
    default:
      process.exit(0);
  }

  if (!status) process.exit(0);

  fetch(`http://${piperHost}/api/internal/hook-status`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      sessionName,
      status,
      timestamp: Date.now(),
      hookEvent,
    }),
    signal: AbortSignal.timeout(3000),
  }).catch(() => {});
});
