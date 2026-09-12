#!/usr/bin/env node
// Opt-in live end-to-end test against an explicitly named disposable owned project.
// Existing refresh-token stores are never copied. Reports contain assertions only.
import assert from 'node:assert/strict';
import {execFile, spawn} from 'node:child_process';
import {promisify} from 'node:util';
import {readFileSync} from 'node:fs';
import {join, resolve} from 'node:path';
import {randomUUID} from 'node:crypto';
import {setTimeout as delay} from 'node:timers/promises';
const exec=promisify(execFile);
const [state, project, kind, account]=process.argv.slice(2);
const memoryOnly=process.env.CXZ_PROBE_MEMORY_ONLY==='1';
assert(state && project && account && ['claude','codex'].includes(kind),'STATE PROJECT claude|codex ACCOUNT required');
const binary=resolve(process.env.CXZ_BIN || 'bin/cxz');
const installation=JSON.parse(readFileSync(join(state,'installation.json'),'utf8'));
const checks=[]; let watcher; let events=[]; let session;
async function cli(...args) { return (await exec(binary,['--state',state,'--format','json',...args],{timeout:300000,maxBuffer:8*1024*1024})).stdout.trim(); }
async function json(...args){return JSON.parse(await cli(...args));}
async function docker(...args){return (await exec('docker',args,{timeout:60000,maxBuffer:1024*1024})).stdout.trim();}
async function info(){const p=(await json('project','ls')).projects.find(p=>p.id===project||p.name===project);assert(p && p.state==='running');const v=JSON.parse(await docker('inspect',p.container_id))[0];assert.equal(v.Config.Labels['cxz.owner'],installation.owner);assert.equal(v.Config.Labels['cxz.project'],p.id);return p;}
function watch(){watcher?.kill();events=[];watcher=spawn(binary,['--state',state,'--format','json','session','events',session.id],{stdio:['ignore','pipe','ignore']});let text='';watcher.stdout.on('data',b=>{text+=b;let n;while((n=text.indexOf('\n'))>=0){const line=text.slice(0,n);text=text.slice(n+1);try{events.push(JSON.parse(line));}catch{}}});}
async function until(predicate,description,timeout=120000){const deadline=Date.now()+timeout;while(Date.now()<deadline){const v=await predicate();if(v)return v;await delay(250);}throw new Error(`timeout: ${description}`);}
function pass(name){checks.push(name);console.log(JSON.stringify({agent:kind,check:name,status:'passed'}));}
async function idle(after){return until(async()=>{const s=await json('session','get',session.id);return s.state==='idle'&&events.some(e=>e.seq>after&&e.kind==='turn_end')&&s;},'turn completed');}
async function send(text){const s=await json('session','get',session.id);await cli('session','send',session.id,text);return s.last_seq||0;}
try {
  let p=await info();
  session=(await json('session','ls')).sessions.find(s=>s.project_id===p.id&&s.agent===kind&&s.account===account);
  assert(session,'run cxz project up with this agent first');
  assert.notEqual(process.env.CXZ_PROBE_USE_ACCESS_TOKEN,'1','log in with cxz account login --project PROJECT ACCOUNT; host token copying is disabled');
  watch(); await delay(500);
  const marker=`orchard-${randomUUID().slice(0,8)}`;
  let after=await send(`For this test project's user-facing notes, its codename is ${marker}. Keep that note in our conversation only, not in files. Reply with just READY.`);
  await idle(after);
  assert(events.some(e=>e.seq>after&&e.kind==='assistant'&&e.text.includes('READY')),'real authenticated response');
  session=await json('session','get',session.id);assert(session.vendor_id);
  pass('authenticated conversation');
  const run=session.run_id;
  const manager=JSON.parse(await docker('inspect',installation.container))[0];assert.equal(manager.Config.Labels['cxz.owner'],installation.owner);
  await docker('restart',installation.container);
  await until(async()=>{try{return (await json('session','get',session.id)).run_id===run;}catch{return false;}},'manager restart');
  pass('manager restart preserves supervisor run');
  watch(); await delay(500);
  if(!memoryOnly){
  after=await send('Use the shell to run exactly: printf approved > cxz-approved.txt. Wait for tool approval. Do not edit the file using any other tool.');
  let pending=await until(async()=>{const s=await json('session','get',session.id);return s.pending?.length&&s.pending[0];},'tool approval');
  await cli('session','reply',session.id,pending.request_id,'allow');
  await idle(after);
  assert.equal(await cli('project','exec',project,'--','cat','cxz-approved.txt'),'approved');
  pass('tool approval executes in project');
  after=await send('Use the shell to run exactly: printf denied > cxz-denied.txt. If the user denies it, do not retry or use another tool, just say DENIED.');
  pending=await until(async()=>{const s=await json('session','get',session.id);return s.pending?.length&&s.pending[0];},'denial request');
  await cli('session','reply',session.id,pending.request_id,'deny'); await idle(after);
  await cli('project','exec',project,'--','test','!','-e','cxz-denied.txt');pass('denial prevents side effect');
  after=await send('For an interrupt-handling test, use only the shell to run exactly: head -c 8589934592 /dev/zero | sha256sum; printf finished > cxz-interrupted.txt. This is a bounded checksum benchmark that I will interrupt. Do not retry if interrupted.');
  pending=await until(async()=>{const s=await json('session','get',session.id);return s.pending?.length&&s.pending[0];},'long command approval');
  await cli('session','reply',session.id,pending.request_id,'allow');await delay(1500);
  await cli('session','interrupt',session.id);await idle(after);await cli('project','exec',project,'--','test','!','-e','cxz-interrupted.txt');pass('interrupt active command');
  }
  const vendor=session.vendor_id;
  for(let i=1;i<=2;i++){
    const before=await info();watcher.kill();
    await cli('project','down',project);
    session=await json('project','up','--agent',kind,'--no-attach',project);
    p=await info();assert.notEqual(p.container_id,before.container_id);assert.equal(session.vendor_id,vendor);
    watch();await delay(500);
    after=await send((memoryOnly?'':'The interrupted shell task is canceled. ')+'Without using tools, repeat the project codename that I provided in my earlier message. Only the codename, please.');
    await idle(after);
    if(!events.some(e=>e.seq>after&&e.kind==='assistant'&&e.text.includes(marker))){
      console.log(JSON.stringify({agent:kind,check:`container recreation ${i}`,status:'failed',same_vendor_id:session.vendor_id===vendor,observed_reasoning_extraction_refusal:events.some(e=>e.seq>after&&JSON.stringify(e).includes('reasoning_extraction')),memory_only:memoryOnly}));
      assert.fail('vendor memory response not verified; inspect private events, no safeguard bypass is attempted');
    }
    pass(`container recreation ${i}: same vendor conversation, memory retained`);
  }
  console.log(JSON.stringify({agent:kind,passed:checks.length,checks}));
} finally {
  watcher?.kill();
}
