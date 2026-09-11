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
const [state, project, kind]=process.argv.slice(2);
assert(state && project && ['claude','codex'].includes(kind),'STATE PROJECT claude|codex required');
const binary=resolve(process.env.CXZ_BIN || 'bin/cxz');
const installation=JSON.parse(readFileSync(join(state,'installation.json'),'utf8'));
const checks=[]; let watcher; let events=[]; let temporaryAuth=false; let authPath; let session;
async function cli(...args) { return (await exec(binary,['--state',state,...args],{timeout:300000,maxBuffer:8*1024*1024})).stdout.trim(); }
async function json(...args){return JSON.parse(await cli(...args));}
async function docker(...args){return (await exec('docker',args,{timeout:60000,maxBuffer:1024*1024})).stdout.trim();}
async function info(){const p=(await json('projects')).projects.find(p=>p.id===project||p.name===project);assert(p && p.state==='running');const v=JSON.parse(await docker('inspect',p.container_id))[0];assert.equal(v.Config.Labels['cxz.owner'],installation.owner);assert.equal(v.Config.Labels['cxz.project'],p.id);return p;}
async function input(args,data){await new Promise((resolve,reject)=>{const p=spawn('docker',args,{stdio:['pipe','ignore','pipe']});let error='';p.stderr.on('data',b=>error+=b);p.on('error',reject);p.on('close',code=>code===0?resolve():reject(new Error(`credential provisioning failed (${code}): ${error.replaceAll(data.trim(),'<redacted>').replace(/eyJ[\w-]+\.[\w-]+\.[\w-]+|sk-[\w-]+/g,'<redacted>').slice(0,1000)}`)));p.stdin.end(data);});}
function watch(){watcher?.kill();events=[];watcher=spawn(binary,['--state',state,'events',session.id],{stdio:['ignore','pipe','ignore']});let text='';watcher.stdout.on('data',b=>{text+=b;let n;while((n=text.indexOf('\n'))>=0){const line=text.slice(0,n);text=text.slice(n+1);try{events.push(JSON.parse(line));}catch{}}});}
async function until(predicate,description,timeout=120000){const deadline=Date.now()+timeout;while(Date.now()<deadline){const v=await predicate();if(v)return v;await delay(250);}throw new Error(`timeout: ${description}`);}
function pass(name){checks.push(name);console.log(JSON.stringify({agent:kind,check:name,status:'passed'}));}
async function idle(after){return until(async()=>{const s=await json('get',session.id);return s.state==='idle'&&events.some(e=>e.seq>after&&e.kind==='turn_end')&&s;},'turn completed');}
async function send(text){const s=await json('get',session.id);await cli('send',session.id,text);return s.last_seq||0;}
try {
  let p=await info();
  session=(await json('ls')).sessions.find(s=>s.project_id===p.id&&s.agent===kind);
  assert(session,'run cxz up with this agent first');
  if(process.env.CXZ_PROBE_USE_ACCESS_TOKEN==='1'){
    authPath=`/cxz/state/data/agents/${kind}/${kind==='claude'?'.credentials.json':'auth.json'}`;
    await docker('exec',p.container_id,'test','!','-e',authPath);
    if(['idle','working','waiting_input','starting'].includes(session.state)) await cli('stop',session.id);
    if(kind==='claude'){
      const source=JSON.parse(readFileSync(join(process.env.CLAUDE_CONFIG_DIR||join(process.env.HOME,'.claude'),'.credentials.json'),'utf8')).claudeAiOauth;
      assert(source.accessToken&&source.expiresAt>Date.now()+600000,'valid access token with 10 minutes remaining required');
      const {accessToken,expiresAt,scopes,subscriptionType,rateLimitTier}=source;
      await input(['exec','-i','--user',p.remote_user,p.container_id,'sh','-c',`umask 077; cat > ${authPath}`],JSON.stringify({claudeAiOauth:{accessToken,expiresAt,scopes,subscriptionType,rateLimitTier}}));
    }else{
      const source=JSON.parse(readFileSync(join(process.env.CODEX_HOME||join(process.env.HOME,'.codex'),'auth.json'),'utf8'));
      assert(source.tokens?.access_token,'existing access token required');
      const {id_token,access_token,account_id}=source.tokens;
      // --with-access-token accepts agent-identity tokens, not ChatGPT OAuth.
      // Project-local short-lived projection deliberately has NO refresh token.
      await input(['exec','-i','--user',p.remote_user,p.container_id,'sh','-c',`umask 077; cat > ${authPath}`],JSON.stringify({auth_mode:'chatgpt',tokens:{id_token,access_token,account_id,refresh_token:''},last_refresh:new Date().toISOString()}));
    }
    temporaryAuth=true;
    session=await json('resume',session.id);
  }
  watch(); await delay(500);
  const marker=`CXZ_MEMORY_${randomUUID().replaceAll('-','')}`;
  let after=await send(`Remember this exact token for this conversation: ${marker}. Do not use tools or write files. Reply with just READY.`);
  await idle(after);
  assert(events.some(e=>e.seq>after&&e.kind==='assistant'&&e.text.includes('READY')),'real authenticated response');
  session=await json('get',session.id);assert(session.vendor_id);
  pass('authenticated conversation');
  const run=session.run_id;
  const manager=JSON.parse(await docker('inspect',installation.container))[0];assert.equal(manager.Config.Labels['cxz.owner'],installation.owner);
  await docker('restart',installation.container);
  await until(async()=>{try{return (await json('get',session.id)).run_id===run;}catch{return false;}},'manager restart');
  pass('manager restart preserves supervisor run');
  watch(); await delay(500);
  after=await send('Use the shell to run exactly: printf approved > cxz-approved.txt. Wait for tool approval. Do not edit the file using any other tool.');
  let pending=await until(async()=>{const s=await json('get',session.id);return s.pending?.length&&s.pending[0];},'tool approval');
  await cli('reply',session.id,pending.request_id,'allow');
  await idle(after);
  assert.equal(await cli('exec',project,'--','cat','cxz-approved.txt'),'approved');
  pass('tool approval executes in project');
  after=await send('Use the shell to run exactly: printf denied > cxz-denied.txt. If the user denies it, do not retry or use another tool, just say DENIED.');
  pending=await until(async()=>{const s=await json('get',session.id);return s.pending?.length&&s.pending[0];},'denial request');
  await cli('reply',session.id,pending.request_id,'deny'); await idle(after);
  await cli('exec',project,'--','test','!','-e','cxz-denied.txt');pass('denial prevents side effect');
  after=await send('Use only the shell to run exactly: sleep 60; printf finished > cxz-interrupted.txt. Do not retry if interrupted.');
  pending=await until(async()=>{const s=await json('get',session.id);return s.pending?.length&&s.pending[0];},'long command approval');
  await cli('reply',session.id,pending.request_id,'allow');await delay(1500);
  await cli('interrupt',session.id);await idle(after);await cli('exec',project,'--','test','!','-e','cxz-interrupted.txt');pass('interrupt active command');
  const vendor=session.vendor_id;
  for(let i=1;i<=2;i++){
    const before=await info();watcher.kill();
    await cli('down',project);
    session=await json('up',project,'--agent',kind,'--no-attach');
    p=await info();assert.notEqual(p.container_id,before.container_id);assert.equal(session.vendor_id,vendor);
    watch();await delay(500);
    after=await send('Cancel any previously interrupted task. Do not use tools. What exact CXZ_MEMORY_ token did I ask you to remember? Reply with only that token.');
    await idle(after);assert(events.some(e=>e.seq>after&&e.kind==='assistant'&&e.text.includes(marker)),'vendor memory survived');
    pass(`container recreation ${i}: same vendor conversation, memory retained`);
  }
  console.log(JSON.stringify({agent:kind,passed:checks.length,checks}));
} finally {
  watcher?.kill();
  if(temporaryAuth){const p=await info();await docker('exec','--user',p.remote_user,p.container_id,'rm','-f',authPath);console.log(JSON.stringify({agent:kind,temporary_access_token_removed:true}));}
}
