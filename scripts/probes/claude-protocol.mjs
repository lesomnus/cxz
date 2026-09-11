#!/usr/bin/env node
// A real CLI protocol probe, not a cxz supervisor. No SDK runtime dependency.
import assert from 'node:assert/strict';
import { spawn, execFileSync } from 'node:child_process';
import { mkdtempSync, existsSync, readFileSync, writeFileSync, mkdirSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { randomUUID } from 'node:crypto';
import { createInterface } from 'node:readline';
import { setTimeout as delay } from 'node:timers/promises';

const cli = process.env.CXZ_CLAUDE_BIN || 'claude';
const out = resolve(process.argv[2] || 'testdata/protocol/claude/latest');
for (const name of ['events.jsonl', 'summary.json']) {
  assert.equal(existsSync(join(out, name)), false, 'Choose a fresh output directory; existing evidence is never overwritten');
}
const scratch = mkdtempSync(join(tmpdir(), 'cxz-claude-protocol-'));
const started = Date.now();
const fixture = [];
const checks = [];
const children = new Set();
const ids = new Map();
const version = execFileSync(cli, ['--version'], { encoding: 'utf8' }).trim();
const withoutPromptTool = process.argv.includes('--without-prompt-tool');
const secretKeys = /^(account|email|orgId|orgName|account_id|access_token|refresh_token|id_token|authorization|apiKey|token|authorizationCode|state|code_verifier|code_challenge|signature)$/i;
function sanitize(value, key = '') {
  if (secretKeys.test(key)) return '<redacted>';
  if (Array.isArray(value)) return value.map(v => sanitize(v));
  if (value && typeof value === 'object') return Object.fromEntries(Object.entries(value).map(([k, v]) => [k, sanitize(v, k)]));
  if (typeof value !== 'string') return value;
  let text = value.replaceAll(scratch, '<scratch>');
  for (const path of [process.env.CLAUDE_CONFIG_DIR, process.env.HOME].filter(Boolean)) text = text.replaceAll(path, '<config-or-home>');
  text = text.replace(/\b[\w.+-]+@[\w.-]+\.[A-Za-z]{2,}\b/g, '<email>');
  text = text.replace(/(?:sk-ant-[\w-]+|eyJ[\w-]+\.[\w-]+\.[\w-]+)/g, '<redacted-token>');
  text = text.replace(/https:\/\/[^\s"<>]*(?:oauth|authorize)[^\s"<>]*/gi, '<redacted-oauth-url>');
  if (text && (/^(session_id|request_id|uuid|tool_use_id)$/.test(key)
      || (key === 'id' && /^(msg_|toolu_|[0-9a-f]{8}-)/.test(text)))) {
    if (!ids.has(text)) ids.set(text, `id-${ids.size + 1}`);
    text = ids.get(text);
  }
  return text;
}
function record(run, direction, message) {
  fixture.push({ elapsed_ms: Date.now() - started, run, direction, message: sanitize(message) });
}
function check(name, details = {}) {
  checks.push({ name, status: 'passed', ...details });
  console.log(JSON.stringify(checks.at(-1)));
}
class Client {
  constructor(run, resume, configDir) {
    this.run = run;
    this.messages = [];
    this.error = null;
    const args = ['-p', '--input-format', 'stream-json', '--output-format', 'stream-json', '--verbose',
      '--include-partial-messages', '--replay-user-messages', '--permission-mode', 'manual',
      '--permission-prompts', 'host', '--setting-sources=', '--strict-mcp-config', '--mcp-config', '{"mcpServers":{}}',
      '--disable-slash-commands', '--tools', 'Bash,AskUserQuestion', '--settings',
      JSON.stringify({ permissions: { defaultMode: 'manual', allow: [], deny: [], ask: ['Bash'] }, disableAllHooks: true }),
      '--system-prompt', 'Help the user with simple file operations. Use the requested commands exactly. When permission is denied, stop and report it.'];
    if (!withoutPromptTool) args.push('--permission-prompt-tool', 'stdio');
    if (resume) args.push(`--resume=${resume}`);
    const env = { ...process.env, CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: '1' };
    if (configDir) {
      env.CLAUDE_CONFIG_DIR = configDir;
      for (const key of ['ANTHROPIC_API_KEY', 'ANTHROPIC_AUTH_TOKEN', 'CLAUDE_CODE_OAUTH_TOKEN']) delete env[key];
    }
    // Keep the existing credential store in place; never copy refresh tokens.
    delete env.CLAUDECODE;
    this.child = spawn(cli, args, { cwd: scratch, env, stdio: ['pipe', 'pipe', 'pipe'], detached: true });
    children.add(this.child);
    this.exit = new Promise(resolveExit => this.child.once('exit', (code, signal) => {
      this.exited = { code, signal };
      children.delete(this.child);
      resolveExit(this.exited);
    }));
    this.child.on('error', error => { this.error = error; });
    this.child.stdin.on('error', error => { this.error = error; });
    createInterface({ input: this.child.stdout }).on('line', line => {
      try {
        const message = JSON.parse(line);
        record(run, 'out', message);
        this.messages.push(message);
      } catch { this.error = new Error('Non-JSON CLI stdout; see sanitized fixture'); record(run, 'diagnostic', line); }
    });
    createInterface({ input: this.child.stderr }).on('line', line => record(run, 'stderr', line));
    record(run, 'launch', { version, args });
  }
  send(message) {
    if (this.error) throw this.error;
    record(this.run, 'in', message);
    this.child.stdin.write(JSON.stringify(message) + '\n');
  }
  async wait(predicate, from = 0, timeout = 90000) {
    const deadline = Date.now() + timeout;
    while (Date.now() < deadline) {
      const found = this.messages.slice(from).find(predicate);
      if (found) return found;
      if (this.error) throw this.error;
      if (this.exited) throw new Error(`CLI exited before expected event: ${JSON.stringify(this.exited)}`);
      await delay(25);
    }
    throw new Error(`Timeout waiting for event in ${this.run}; inspect sanitized fixture`);
  }
  async control(request) {
    const request_id = randomUUID();
    this.send({ type: 'control_request', request_id, request });
    const result = await this.wait(m => m.type === 'control_response' && m.response.request_id === request_id);
    assert.equal(result.response.subtype, 'success', JSON.stringify(sanitize(result)));
    return result.response.response;
  }
  prompt(text) {
    const from = this.messages.length;
    this.send({ type: 'user', session_id: '', message: { role: 'user', content: text }, parent_tool_use_id: null });
    return from;
  }
  reply(request, allow) {
    this.send({ type: 'control_response', response: { subtype: 'success', request_id: request.request_id,
      response: allow ? { behavior: 'allow', updatedInput: request.request.input }
        : { behavior: 'deny', message: 'Denied by protocol test. Do not retry.' } } });
  }
  async permission(from, command) {
    const m = await this.wait(m => m.type === 'control_request' || m.type === 'result', from);
    assert.equal(m.type, 'control_request', `Turn ended without permission request (${m.terminal_reason || m.stop_reason})`);
    assert.equal(m.request.subtype, 'can_use_tool');
    assert.equal(m.request.tool_name, 'Bash');
    if (m.request.input.command.trim() !== command) {
      this.reply(m, false);
      throw new Error('Agent changed the exact probe command; denied');
    }
    return m;
  }
  async result(from, interrupted = false) {
    // Any unexpected extra tool request must fail closed, not hang or execute.
    const m = await this.wait(m => m.type === 'result' || m.type === 'control_request', from);
    if (m.type === 'control_request') { this.reply(m, false); throw new Error('Unexpected additional tool request'); }
    if (interrupted) assert.equal(m.terminal_reason, 'aborted_tools');
    else assert.equal(m.is_error, false, `Turn failed: ${m.terminal_reason || m.stop_reason}`);
    return m;
  }
  async close() {
    this.child.stdin.end();
    const graceful = await Promise.race([this.exit.then(() => true), delay(5000).then(() => false)]);
    if (!graceful) {
      try { process.kill(-this.child.pid, 'SIGTERM'); } catch {}
      await Promise.race([this.exit, delay(2000)]);
    }
    if (!this.exited) { try { process.kill(-this.child.pid, 'SIGKILL'); } catch {} await this.exit; }
  }
}

try {
  if (process.argv.includes('--login-only')) {
    const config = mkdtempSync(join(tmpdir(), 'cxz-claude-login-'));
    const c = new Client('login-isolated', undefined, config);
    await c.control({ subtype: 'initialize', hooks: {}, sdkMcpServers: [] });
    const response = await c.control({ subtype: 'claude_authenticate', loginWithClaudeAi: true });
    const encoded = JSON.stringify(response);
    assert(/https:\/\//.test(encoded), 'Authentication response did not include a URL');
    check('isolated_login_returns_url', { response_keys: Object.keys(response) });
    await c.close();
    check('login_probe_closed_without_completing_or_copying_credentials');
  } else if (withoutPromptTool) {
    const c = new Client('without-stdio-routing');
    await c.control({ subtype: 'initialize', hooks: {}, sdkMcpServers: [] });
    const marker = join(scratch, 'baseline.txt');
    const from = c.prompt(`Please create a greeting file with this exact Bash command: printf 'hello' > ${marker}`);
    const result = await c.wait(m => m.type === 'result', from);
    assert.equal(result.is_error, false, `Baseline turn failed: ${result.terminal_reason || result.stop_reason}`);
    assert(result.permission_denials?.some(d => d.tool_name === 'Bash'));
    assert(c.messages.slice(from).some(m => m.type === 'assistant' && m.message?.content?.some?.(b => b.type === 'tool_use' && b.name === 'Bash')));
    assert.equal(c.messages.slice(from).some(m => m.type === 'control_request'), false);
    assert.equal(existsSync(marker), false);
    check('without_stdio_no_host_request_and_no_execution');
    await c.close();
  } else {
  const c = new Client('initial');
  const init = await c.control({ subtype: 'initialize', hooks: {}, sdkMcpServers: [] });
  check('initialize', { response_keys: Object.keys(init || {}) });
  const allowed = join(scratch, 'allowed.txt');
  const command = `printf 'cxz-allowed' > ${allowed}`;
  let from = c.prompt(`Call Bash once with exactly this command: ${command}\nThen reply ALLOWED. Do not change the command.`);
  const request = await c.permission(from, command);
  const holdStart = Date.now();
  assert.equal(existsSync(allowed), false);
  await delay(3500);
  assert.equal(existsSync(allowed), false);
  assert.equal(c.messages.slice(from).some(m => m.type === 'result'), false);
  check('approval_blocks_execution', { held_ms: Date.now() - holdStart });
  const afterApproval = c.messages.length;
  c.reply(request, true);
  const first = await c.result(afterApproval);
  assert.equal(readFileSync(allowed, 'utf8'), 'cxz-allowed');
  check('approval_allows_execution');
  const session = first.session_id;

  const denied = join(scratch, 'denied.txt');
  const denyCommand = `printf 'hello' > ${denied}`;
  from = c.prompt(`Please create another greeting file by running this exact Bash command: ${denyCommand}`);
  const denial = await c.permission(from, denyCommand);
  const afterDenial = c.messages.length;
  c.reply(denial, false);
  const second = await c.result(afterDenial);
  assert.equal(existsSync(denied), false);
  assert.equal(second.session_id, session);
  assert(c.messages.slice(from).some(m => m.type === 'user' && m.message?.content?.some?.(b => b.type === 'tool_result' && b.is_error)));
  check('denial_prevents_execution');
  check('multiple_turns_same_session');

  from = c.prompt('Use AskUserQuestion to ask me which greeting language I prefer, English or Korean, then tell me my selection. Do not use Bash.');
  const question = await c.wait(m => m.type === 'control_request' || m.type === 'result', from);
  assert.equal(question.type, 'control_request');
  assert.equal(question.request.tool_name, 'AskUserQuestion');
  const questionInput = question.request.input;
  const answers = Object.fromEntries(questionInput.questions.map(q => [q.question, 'Korean']));
  const afterQuestion = c.messages.length;
  c.send({ type: 'control_response', response: { subtype: 'success', request_id: question.request_id,
    response: { behavior: 'allow', updatedInput: { ...questionInput, answers } } } });
  await c.result(afterQuestion);
  assert(c.messages.slice(afterQuestion).some(m => m.type === 'user' && JSON.stringify(m.message?.content).includes('Korean')));
  check('question_answer_round_trip');

  const remembered = `CXZ_MEMORY_${randomUUID().replaceAll('-', '')}`;
  from = c.prompt(`Remember this test token for a later turn: ${remembered}. Reply only MEMORIZED. Do not use tools.`);
  await c.result(from);

  const interrupted = join(scratch, 'interrupted.txt');
  const interruptCommand = `sleep 20 && printf 'hello' > ${interrupted}`;
  from = c.prompt(`Call Bash exactly once with command: ${interruptCommand}\nDo not run in background. If interrupted, do not retry.`);
  const pending = await c.permission(from, interruptCommand);
  const afterAllow = c.messages.length;
  c.reply(pending, true);
  await delay(1200);
  const interruptAt = Date.now();
  await c.control({ subtype: 'interrupt' });
  const interruptedResult = await c.result(afterAllow, true);
  assert.equal(existsSync(interrupted), false);
  check('interrupt_ack_and_result', { elapsed_ms: Date.now() - interruptAt, result_subtype: interruptedResult.subtype,
    stop_reason: interruptedResult.stop_reason });
  // Wait beyond the original command's duration to catch surviving shell children.
  await delay(Math.max(0, 22000 - (Date.now() - interruptAt)));
  assert.equal(existsSync(interrupted), false);
  check('interrupt_prevents_delayed_side_effect');
  await c.close();

  const resumed = new Client('resume', session);
  await resumed.control({ subtype: 'initialize', hooks: {}, sdkMcpServers: [] });
  from = resumed.prompt('What exact test token did I ask you to remember earlier? Reply only that token. Do not use tools.');
  const result = await resumed.result(from);
  assert.equal(result.session_id, session);
  const answer = resumed.messages.slice(from).filter(m => m.type === 'assistant').flatMap(m => m.message?.content || [])
    .filter(b => b.type === 'text').map(b => b.text).join('');
  assert(answer.includes(remembered), 'Resumed conversation did not remember its prior token');
  check('resume_same_session_and_context_after_process_exit');
  await resumed.close();
  }
} catch (error) {
  checks.push({ name: 'suite', status: 'failed', error: sanitize(error.message) });
  console.error(JSON.stringify(checks.at(-1)));
  process.exitCode = 1;
} finally {
  for (const child of children) { try { process.kill(-child.pid, 'SIGKILL'); } catch {} }
  mkdirSync(out, { recursive: true });
  writeFileSync(join(out, 'events.jsonl'), fixture.map(v => JSON.stringify(v)).join('\n') + '\n', { mode: 0o600 });
  writeFileSync(join(out, 'summary.json'), JSON.stringify({ version, sdk_source_version: '0.3.268',
    tested_at: new Date().toISOString(), without_prompt_tool: withoutPromptTool, checks }, null, 2) + '\n', { mode: 0o600 });
  console.log(`Sanitized results: ${out}`);
}
