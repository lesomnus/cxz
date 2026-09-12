#!/usr/bin/env node
// App-level recovery test; deterministic agent, no credentials or API usage.
// Build bin/cxz and bin/fake-claude first. All Docker resources are uniquely owned.
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { existsSync, mkdirSync, writeFileSync } from 'node:fs';
import { resolve, join } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';

const repo = process.env.CXZ_ENGINE_REPO || '/workspaces/github.com/lesomnus/cxz';
assert.ok(repo.startsWith('/workspaces/'), 'Engine bind path must use /workspaces');
const image = process.env.CXZ_PROBE_IMAGE || 'golang:1.26-trixie';
const out = resolve(process.argv[2] || 'testdata/recovery/cxz-container');
assert.ok(!existsSync(join(out, 'summary.json')), 'Choose fresh evidence directory');
const token = randomUUID();
const prefix = `cxz-app-probe-${token}`;
const label = `cxz.probe=${token}`;
const state = `${prefix}-state`, workspace = `${prefix}-workspace`;
const volumes = new Set(), containers = new Set(), checks = [];
const docker = args => execFileSync('docker', args, { encoding: 'utf8', timeout: 60000, stdio: ['ignore', 'pipe', 'pipe'] }).trim();
const mounts = ['--mount', `type=volume,source=${state},target=/cxz-state,volume-nocopy`, '--mount', `type=volume,source=${workspace},target=/cxz-work,volume-nocopy`];
const check = name => { checks.push({ name, status: 'passed' }); console.log(name); };
function removeContainer(name) {
  if (!containers.has(name)) return;
  const owner = docker(['inspect', '--format', '{{index .Config.Labels "cxz.probe"}}', name]);
  assert.equal(owner, token);
  docker(['rm', '-f', name]); containers.delete(name);
}
function start(n) {
  const name = `${prefix}-${n}`; containers.add(name);
  docker(['run', '-d', '--name', name, '--label', label, '--user', '1000:1000', '--cap-drop', 'ALL', '--security-opt', 'no-new-privileges', '--network', 'none',
    '--mount', `type=bind,source=${repo},target=/app,readonly`, ...mounts,
    image, '/app/bin/cxz', '--state', '/cxz-state', 'manager', 'serve', '--agent', '/app/bin/fake-claude']);
  return name;
}
function cli(name, ...args) {
  return JSON.parse(docker(['exec', name, '/app/bin/cxz', '--state', '/cxz-state', '--format', 'json', ...args]));
}
async function ready(name) {
  for (let i=0;i<80;i++) { try { cli(name, 'session','ls');return; } catch { await delay(100); } }
  throw new Error('daemon did not become ready');
}
async function untilState(name,id,state) {
  for(let i=0;i<80;i++){const s=cli(name,'session','get',id);if(s.state===state)return s;await delay(100);}
  throw new Error(`did not reach ${state}`);
}
try {
  for (const v of [state,workspace]) { docker(['volume','create','--label',label,v]);volumes.add(v); }
  const provision=`${prefix}-provision`;containers.add(provision);
  docker(['run','--name',provision,'--label',label,'--user','0','--cap-drop','ALL','--cap-add','CHOWN','--network','none',...mounts,image,'chown','1000:1000','/cxz-state','/cxz-work']);removeContainer(provision);
  let name=start(1);await ready(name);
  cli(name,'account','add','claude','recovery-test');
  execFileSync('docker',['exec','-i',name,'/app/bin/cxz','--state','/cxz-state','_account-import','recovery-test','claude'],{input:JSON.stringify({claudeAiOauth:{accessToken:'synthetic-recovery-only'}}),stdio:['pipe','pipe','pipe']});
  let s=cli(name,'_new-local','recovery-test','/cxz-work');const id=s.id;assert.equal(s.account,'recovery-test');
  cli(name,'session','send',id,'hello');s=await untilState(name,id,'idle');const vendor=s.vendor_id;
  check('nonroot_app_session_and_conversation');
  cli(name,'session','send',id,'approval recovery');s=await untilState(name,id,'waiting_input');const oldRun=s.run_id,request=s.pending[0].request_id,last=s.last_seq;
  removeContainer(name);name=start(2);await ready(name);s=cli(name,'session','get',id);
  assert.equal(s.state,'interrupted');assert.equal(s.account,'recovery-test');assert.equal(s.vendor_id,vendor);assert.ok(s.last_seq>=last);assert.ok(!s.pending?.length);
  check('container_recreation_preserves_journal_registry_and_marks_interrupted');
  s=cli(name,'session','resume',id);assert.notEqual(s.run_id,oldRun);assert.equal(s.vendor_id,vendor);assert.equal(s.state,'idle');
  assert.throws(()=>cli(name,'session','reply',id,request,'allow'));await delay(300);assert.equal(cli(name,'session','get',id).state,'idle');
  check('explicit_resume_new_run_stale_approval_rejected_no_prompt_replay');
  cli(name,'session','send',id,'after recreation');await untilState(name,id,'idle');
  cli(name,'session','send',id,'wait');s=await untilState(name,id,'working');removeContainer(name);name=start(3);await ready(name);
  assert.equal(cli(name,'session','get',id).state,'interrupted');s=cli(name,'session','resume',id);assert.equal(s.vendor_id,vendor);
  cli(name,'session','send',id,'after active turn loss');await untilState(name,id,'idle');cli(name,'session','stop',id);await untilState(name,id,'stopped');
  check('active_turn_container_loss_and_post_recovery_conversation');
} finally {
  for(const name of [...containers]) { try { removeContainer(name); } catch(e) { console.error(`Cleanup failed for owned container ${name}: ${e.message}`); } }
  for(const v of volumes){assert.equal(docker(['volume','inspect','--format','{{index .Labels "cxz.probe"}}',v]),token);docker(['volume','rm',v]);}
}
mkdirSync(out,{recursive:true,mode:0o700});writeFileSync(join(out,'summary.json'),JSON.stringify({timestamp:new Date().toISOString(),agent:'deterministic stream-json fixture',checks,cleanup:'owned containers and volumes removed'},null,2)+'\n',{mode:0o600,flag:'wx'});
