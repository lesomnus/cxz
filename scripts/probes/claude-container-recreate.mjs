#!/usr/bin/env node
// Live Docker integration probe. Deletes only resources created by this run.
import assert from 'node:assert/strict';
import { execFile, spawn } from 'node:child_process';
import { promisify } from 'node:util';
import { readFileSync, writeFileSync, mkdirSync, existsSync, realpathSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { randomUUID, createHash } from 'node:crypto';
import { createInterface } from 'node:readline';
import { setTimeout as delay } from 'node:timers/promises';

const exec = promisify(execFile);
const run = `cxz-claude-recreate-${randomUUID().slice(0, 12)}`;
const output = resolve(process.argv[2] || `testdata/protocol/claude/${run}`);
assert(!existsSync(output), 'Output directory must be new');
const binary = realpathSync(process.env.CXZ_CLAUDE_BIN || '/usr/local/bin/claude');
const configuredImage = process.env.CXZ_PROBE_IMAGE || 'golang:1.26-trixie';
const oauth = JSON.parse(readFileSync(join(process.env.CLAUDE_CONFIG_DIR || join(process.env.HOME, '.claude'), '.credentials.json'), 'utf8')).claudeAiOauth;
assert(oauth?.accessToken && oauth.expiresAt > Date.now() + 10 * 60000, 'Need an already-valid access token with at least 10 minutes remaining');
// Never copy the credential file, refresh token, or shared config to Docker.
const accessToken = oauth.accessToken;
const label = 'cxz.probe=claude-recreate';
const ownerLabel = `cxz.probe.run=${run}`;
const volumes = [];
const containers = new Set();
const events = [];
const checks = [];
const cleanup = [];
const started = Date.now();
let generation = 0;
let image;
const metadata = { run, configured_image: configuredImage, cli_sha256: createHash('sha256').update(readFileSync(binary)).digest('hex') };
const aliases = new Map();
function scrub(v, key = '') {
  if (/^(account|email|signature|access_token|refresh_token|authorization|apiKey|authorizationCode|code_verifier)$/i.test(key)) return '<redacted>';
  if (Array.isArray(v)) return v.map(x => scrub(x));
  if (v && typeof v === 'object') return Object.fromEntries(Object.entries(v).map(([k, x]) => [k, scrub(x, k)]));
  if (typeof v !== 'string') return v;
  let s = v.replaceAll(accessToken, '<redacted-token>').replace(/sk-ant-[\w-]+|eyJ[\w-]+\.[\w-]+\.[\w-]+/g, '<redacted-token>');
  s = s.replace(/\b[\w.+-]+@[\w.-]+\.[A-Za-z]{2,}\b/g, '<email>');
  if (s && (/^(session_id|request_id|uuid|tool_use_id)$/.test(key) || (key === 'id' && /^(toolu_|msg_)/.test(s)))) {
    if (!aliases.has(s)) aliases.set(s, `id-${aliases.size + 1}`);
    s = aliases.get(s);
  }
  return s;
}
function record(container, direction, message) {
  events.push({ elapsed_ms: Date.now() - started, container, direction, message: scrub(message) });
}
function pass(name, detail = {}) { checks.push({ name, status: 'passed', ...detail }); console.log(JSON.stringify(checks.at(-1))); }
async function docker(args, timeout = 60000) {
  try { return (await exec('docker', args, { timeout, maxBuffer: 4 * 1024 * 1024 })).stdout.trimEnd(); }
  catch (error) { throw new Error(scrub(`docker ${args[0]} failed: ${error.stderr || error.message}`)); }
}
async function inspectContainer(id) { return JSON.parse(await docker(['inspect', id]))[0]; }
async function removeContainer(id) {
  const info = await inspectContainer(id);
  assert.equal(info.Config.Labels['cxz.probe.run'], run);
  await docker(['rm', '-f', id]);
  containers.delete(id);
  cleanup.push({ resource: info.Name.slice(1), removed: true });
}
async function createContainer(provision = false) {
  const name = `${run}-${provision ? 'provision' : ++generation}`;
  const args = ['create', '--name', name, '--label', label, '--label', ownerLabel,
    '--cap-drop', 'ALL', '--security-opt', 'no-new-privileges', '--pids-limit', '256', '--memory', '2g', '--cpus', '2',
    '--mount', `type=volume,src=${run}-state,dst=/cxz/state,volume-nocopy`,
    '--mount', `type=volume,src=${run}-workspace,dst=/workspace,volume-nocopy`,
    '--mount', `type=volume,src=${run}-tools,dst=/opt/cxz-probe,volume-nocopy${provision ? '' : ',readonly'}`,
    '--env', 'CLAUDE_CONFIG_DIR=/cxz/state', '--env', 'HOME=/tmp/cxz-home',
    '--env', 'CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1', '--workdir', '/workspace'];
  if (!provision) args.push('--user', '1000:1000');
  else args.push('--cap-add', 'CHOWN');
  args.push(image, 'sleep', 'infinity');
  const id = await docker(args);
  containers.add(id);
  await docker(['start', id]);
  record(id, 'lifecycle', { action: 'created', generation, name });
  if (!provision) {
    const ownership = await docker(['exec', id, 'stat', '-c', '%u:%g', '/workspace', '/cxz/state']);
    record(id, 'preflight', { ownership });
    assert.equal(ownership, '1000:1000\n1000:1000', 'Volume ownership changed during container creation');
    await docker(['exec', id, 'sh', '-c', 'test -w /workspace && test -w /cxz/state']);
  }
  return id;
}
async function fileText(id, path) {
  return docker(['exec', id, 'cat', path]);
}
async function fileExists(id, path) {
  return (await docker(['exec', id, 'sh', '-c', 'if test -f "$1"; then printf yes; else printf no; fi', 'probe', path])) === 'yes';
}
async function waitFile(id, path, timeout = 12000) {
  const end = Date.now() + timeout;
  while (Date.now() < end) { if (await fileExists(id, path)) return; await delay(100); }
  throw new Error(`Expected file not created: ${path}`);
}
class Client {
  constructor(id, resume) {
    this.id = id;
    this.messages = [];
    const args = ['-p', '--input-format', 'stream-json', '--output-format', 'stream-json', '--verbose',
      '--permission-mode', 'manual', '--permission-prompts', 'host', '--permission-prompt-tool', 'stdio',
      '--setting-sources=', '--strict-mcp-config', '--mcp-config', '{"mcpServers":{}}',
      '--disable-slash-commands', '--tools', 'Bash', '--settings',
      JSON.stringify({ permissions: { defaultMode: 'manual', allow: [], deny: [], ask: ['Bash'] }, disableAllHooks: true }),
      '--system-prompt', 'Help with the requested file operations. Use Bash only when explicitly requested. Run commands exactly. If permission is denied, stop.'];
    if (resume) args.push(`--resume=${resume}`);
    // Token travels over stdin, not Docker's container environment or argv.
    this.child = spawn('docker', ['exec', '-i', id, 'sh', '-c',
      'IFS= read -r CLAUDE_CODE_OAUTH_TOKEN; export CLAUDE_CODE_OAUTH_TOKEN; exec /opt/cxz-probe/claude "$@"', 'probe', ...args],
    { stdio: ['pipe', 'pipe', 'pipe'] });
    this.child.stdin.write(accessToken + '\n');
    this.child.on('error', e => { this.error = e; });
    this.child.stdin.on('error', e => { this.error = e; });
    this.exit = new Promise(resolveExit => this.child.once('exit', (code, signal) => { this.exited = { code, signal }; resolveExit(); }));
    createInterface({ input: this.child.stdout }).on('line', line => {
      try { const m = JSON.parse(line); this.messages.push(m); record(id, 'out', m); }
      catch { this.error = new Error('Invalid JSON from CLI'); record(id, 'diagnostic', line); }
    });
    createInterface({ input: this.child.stderr }).on('line', line => record(id, 'stderr', line));
    record(id, 'launch', { args, credential_transport: 'access-token-only over stdin' });
  }
  send(message) { record(this.id, 'in', message); this.child.stdin.write(JSON.stringify(message) + '\n'); }
  async wait(predicate, from = 0, timeout = 90000) {
    const end = Date.now() + timeout;
    while (Date.now() < end) {
      const m = this.messages.slice(from).find(predicate);
      if (m) return m;
      if (this.error) throw this.error;
      if (this.exited) throw new Error(`CLI exited: ${JSON.stringify(this.exited)}`);
      await delay(30);
    }
    throw new Error('Timed out waiting for CLI protocol event');
  }
  async initialize() {
    const request_id = randomUUID();
    this.send({ type: 'control_request', request_id, request: { subtype: 'initialize', hooks: {}, sdkMcpServers: [] } });
    const m = await this.wait(m => m.type === 'control_response' && m.response.request_id === request_id);
    assert.equal(m.response.subtype, 'success');
  }
  prompt(content) {
    const from = this.messages.length;
    this.send({ type: 'user', session_id: '', parent_tool_use_id: null, message: { role: 'user', content } });
    return from;
  }
  async result(from) {
    const m = await this.wait(m => ['result', 'control_request'].includes(m.type), from);
    if (m.type === 'control_request') { this.reply(m, false); throw new Error('Unexpected tool request while expecting text-only result'); }
    assert.equal(m.is_error, false, `Turn failed: ${m.terminal_reason || m.stop_reason}`);
    return m;
  }
  async permission(from, command) {
    const m = await this.wait(m => ['result', 'control_request'].includes(m.type), from);
    assert.equal(m.type, 'control_request', `Expected approval, got terminal reason ${m.terminal_reason}`);
    if (m.request.subtype !== 'can_use_tool' || m.request.tool_name !== 'Bash' || m.request.input.command.trim() !== command) {
      this.reply(m, false); throw new Error('Unexpected tool command; denied');
    }
    return m;
  }
  reply(m, allow) {
    this.send({ type: 'control_response', response: { subtype: 'success', request_id: m.request_id,
      response: allow ? { behavior: 'allow', updatedInput: m.request.input }
        : { behavior: 'deny', message: 'Cancelled by the user. Do not retry.' } } });
  }
  async remember(word) {
    const result = await this.result(this.prompt(`Our project codename is ${word}. Remember it for this conversation. Reply OK without using tools.`));
    return result.session_id;
  }
  async recall(session, word) {
    const from = this.prompt('The previous file operation is cancelled. Do not execute or retry it. Without using tools, what project codename did I tell you earlier?');
    const result = await this.result(from);
    assert.equal(result.session_id, session);
    assert(this.messages.slice(from).filter(m => m.type === 'assistant').flatMap(m => m.message?.content || [])
      .some(b => b.type === 'text' && b.text.includes(word)), 'Prior conversation context was not recalled');
  }
  async close() {
    this.child.stdin.end();
    await Promise.race([this.exit, delay(8000)]);
    assert(this.exited, 'CLI did not exit after stdin EOF');
  }
}
async function recreate(id, client) {
  const old = await inspectContainer(id);
  await removeContainer(id);
  await Promise.race([client.exit, delay(8000)]);
  assert(client.exited, 'Old docker exec connection survived container removal');
  record(id, 'lifecycle', { action: 'removed_without_cli_shutdown' });
  const next = await createContainer();
  assert.notEqual(next, id);
  const fresh = await inspectContainer(next);
  for (const path of ['/workspace', '/cxz/state']) {
    assert.equal(old.Mounts.find(m => m.Destination === path).Name, fresh.Mounts.find(m => m.Destination === path).Name);
  }
  assert.equal(fresh.Config.User, '1000:1000');
  return next;
}

try {
  image = await docker(['image', 'inspect', configuredImage, '--format', '{{.Id}}']);
  metadata.image_id = image;
  metadata.docker_version = await docker(['version', '--format', '{{.Server.Version}}']);
  for (const kind of ['state', 'workspace', 'tools']) {
    const name = `${run}-${kind}`;
    // Refuse collisions rather than adopting another resource.
    const names = (await docker(['volume', 'ls', '--format', '{{.Name}}'])).split('\n');
    assert(!names.includes(name));
    await docker(['volume', 'create', '--label', label, '--label', ownerLabel, name]);
    volumes.push(name);
  }
  const provision = await createContainer(true);
  await docker(['exec', provision, 'chown', '1000:1000', '/cxz/state', '/workspace']);
  await docker(['cp', binary, `${provision}:/opt/cxz-probe/claude`], 120000);
  await docker(['exec', provision, 'chmod', '755', '/opt/cxz-probe/claude']);
  metadata.cli_version = await docker(['exec', provision, '/opt/cxz-probe/claude', '--version']);
  await removeContainer(provision);
  pass('isolated_resources_provisioned', { cli_version: metadata.cli_version });

  // Case 1: a completed conversation and file survive container replacement.
  let id = await createContainer();
  let c = new Client(id);
  await c.initialize();
  const word = `Garden-${randomUUID().slice(0, 8)}`;
  const session = await c.remember(word);
  const command = "printf 'persistent-greeting' > /workspace/greeting.txt";
  let from = c.prompt(`Run this exact Bash command once: ${command}`);
  let request = await c.permission(from, command);
  let nextFrom = c.messages.length;
  c.reply(request, true);
  await c.result(nextFrom);
  assert.equal(await fileText(id, '/workspace/greeting.txt'), 'persistent-greeting');
  await docker(['exec', id, 'touch', '/tmp/cxz-container-only']);
  await docker(['exec', id, 'sync']);
  id = await recreate(id, c);
  assert.equal(await fileExists(id, '/tmp/cxz-container-only'), false);
  assert.equal(await fileText(id, '/workspace/greeting.txt'), 'persistent-greeting');
  c = new Client(id, session);
  await c.initialize();
  await c.recall(session, word);
  pass('completed_turn_same_session_context_and_file_after_recreate');
  await c.close();

  // Case 2: removal while an approval is pending. A stale response must not
  // authorize a new request on the resumed process.
  c = new Client(id);
  await c.initialize();
  const pendingWord = `Orchard-${randomUUID().slice(0, 8)}`;
  const pendingSession = await c.remember(pendingWord);
  const pendingCommand = "printf 'old' > /workspace/pending-old.txt";
  from = c.prompt(`Run this exact Bash command once: ${pendingCommand}`);
  const oldRequest = await c.permission(from, pendingCommand);
  assert.equal(await fileExists(id, '/workspace/pending-old.txt'), false);
  await docker(['exec', id, 'sync']);
  id = await recreate(id, c);
  c = new Client(id, pendingSession);
  await c.initialize();
  await c.recall(pendingSession, pendingWord);
  assert.equal(await fileExists(id, '/workspace/pending-old.txt'), false);
  pass('pending_approval_resume_preserves_context_without_executing_old_command');
  const freshCommand = "printf 'new' > /workspace/pending-new.txt";
  from = c.prompt(`Run this exact Bash command once: ${freshCommand}`);
  request = await c.permission(from, freshCommand);
  assert.notEqual(request.request_id, oldRequest.request_id);
  c.reply(oldRequest, true);
  await delay(3500);
  assert.equal(await fileExists(id, '/workspace/pending-old.txt'), false);
  assert.equal(await fileExists(id, '/workspace/pending-new.txt'), false);
  assert.equal(c.messages.slice(from).some(m => m.type === 'result'), false);
  nextFrom = c.messages.length;
  c.reply(request, false);
  await c.result(nextFrom);
  pass('stale_approval_does_not_authorize_fresh_request', { held_ms: 3500 });
  await c.close();

  // Case 3: kill while a shell has already written its first side effect.
  c = new Client(id);
  await c.initialize();
  const runningWord = `Meadow-${randomUUID().slice(0, 8)}`;
  const runningSession = await c.remember(runningWord);
  const runningCommand = "printf 'started' >> /workspace/partial.txt; sleep 20; printf 'finished' > /workspace/late.txt";
  from = c.prompt(`Run this exact Bash command in the foreground, not in the background: ${runningCommand}`);
  request = await c.permission(from, runningCommand);
  c.reply(request, true);
  await waitFile(id, '/workspace/partial.txt');
  assert.equal(await fileText(id, '/workspace/partial.txt'), 'started');
  assert.equal(await fileExists(id, '/workspace/late.txt'), false);
  const killedAt = Date.now();
  id = await recreate(id, c);
  assert.equal(await fileText(id, '/workspace/partial.txt'), 'started');
  c = new Client(id, runningSession);
  await c.initialize();
  await c.recall(runningSession, runningWord);
  await delay(Math.max(0, 22000 - (Date.now() - killedAt)));
  assert.equal(await fileText(id, '/workspace/partial.txt'), 'started');
  assert.equal(await fileExists(id, '/workspace/late.txt'), false);
  pass('running_tool_partial_write_survives_without_replay_or_delayed_completion', { observed_after_kill_ms: Date.now() - killedAt });
  await c.close();
  const transcriptNames = await docker(['exec', id, 'find', '/cxz/state/projects', '-type', 'f', '-name', '*.jsonl']);
  assert(transcriptNames.includes(session) && transcriptNames.includes(pendingSession) && transcriptNames.includes(runningSession));
  assert.equal(await fileExists(id, '/cxz/state/.credentials.json'), false);
  pass('three_vendor_transcripts_on_state_volume_without_copied_credentials');
} catch (error) {
  checks.push({ name: 'suite', status: 'failed', error: scrub(error.message) });
  console.error(JSON.stringify(checks.at(-1)));
  process.exitCode = 1;
} finally {
  for (const id of [...containers]) {
    try { await removeContainer(id); }
    catch (error) { cleanup.push({ resource: id, removed: false, error: scrub(error.message) }); process.exitCode = 1; }
  }
  for (const name of volumes) {
    try {
      const info = JSON.parse(await docker(['volume', 'inspect', name]))[0];
      assert.equal(info.Labels['cxz.probe.run'], run);
      await docker(['volume', 'rm', name]);
      cleanup.push({ resource: name, removed: true });
    } catch (error) { cleanup.push({ resource: name, removed: false, error: scrub(error.message) }); process.exitCode = 1; }
  }
  mkdirSync(output, { recursive: true });
  writeFileSync(join(output, 'events.jsonl'), events.map(e => JSON.stringify(e)).join('\n') + '\n', { mode: 0o600 });
  writeFileSync(join(output, 'summary.json'), JSON.stringify({ ...metadata, tested_at: new Date().toISOString(), checks, cleanup,
    limits: ['Direct Docker CLI, not devcontainer up or a cxz supervisor', 'Same workspace path and UID',
      'Existing short-lived access token injected for each CLI process; OAuth persistence not tested',
      'Named-volume retention, not host-loss backup restore'] }, null, 2) + '\n', { mode: 0o600 });
  console.log(`Sanitized evidence: ${output}`);
}
