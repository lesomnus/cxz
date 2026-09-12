#!/usr/bin/env node
// Fresh disposable manager ONLY. Installs a synthetic Codex in its private cache.
import assert from 'node:assert/strict';
import {execFile} from 'node:child_process';
import {promisify} from 'node:util';
import {mkdirSync,writeFileSync,readFileSync} from 'node:fs';
import {join,resolve} from 'node:path';
import {randomUUID,createHash} from 'node:crypto';
import {setTimeout as delay} from 'node:timers/promises';
const exec=promisify(execFile);
const [state,repo,retryRun]=process.argv.slice(2);assert(state&&repo);
const binary=resolve('bin/cxz'),installation=JSON.parse(readFileSync(join(state,'installation.json')));
const run=retryRun||randomUUID().slice(0,8),projects=[];
assert.match(run,/^[a-f0-9]{8}$/);
async function cli(...args){return(await exec(binary,['--state',state,'--format','json',...args],{timeout:300000,maxBuffer:8*1024*1024})).stdout.trim();}
async function json(...args){return JSON.parse(await cli(...args));}
async function docker(...args){return(await exec('docker',args,{timeout:60000,maxBuffer:8*1024*1024})).stdout.trim();}
assert.equal(JSON.parse(await docker('inspect',installation.container))[0].Config.Labels['cxz.owner'],installation.owner);
const existing=(await json('project','ls')).projects||[];
if(retryRun){assert(existing.every(p=>p.workspace.startsWith(join(repo,'.cxz-test-workspaces',`broker-${run}-`))&&p.state==='absent'));}
else{assert.equal(existing.length,0);}
const version=readFileSync('internal/distribution/release.go','utf8').match(/const CodexVersion = "([^"]+)"/)[1];
const arch=(await docker('exec',installation.container,'uname','-m'))==='aarch64'?'aarch64':'x86_64';
const cache=`/cxz/tools/codex/${version}/${arch}-unknown-linux-musl/bin`;
let cached=false;try{await docker('exec',installation.container,'test','-e',cache);cached=true;}catch{}
if(cached){assert.equal((await docker('exec',installation.container,'sha256sum',`${cache}/codex`)).split(' ')[0],createHash('sha256').update(readFileSync('bin/fake-codex')).digest('hex'),'existing tool must be exact test fixture');}
else {await docker('exec',installation.container,'mkdir','-p',cache);await docker('cp',resolve('bin/fake-codex'),`${installation.container}:${cache}/codex`);}
async function until(id,state){for(let i=0;i<100;i++){const s=await json('session','get',id);if(s.state===state)return s;await delay(100);}throw Error(`timeout ${state}`);}
async function stop(s){await cli('session','stop',s.id);return until(s.id,'stopped');}
async function chat(s,prompt='hello'){await cli('session','send',s.id,prompt);const v=await until(s.id,'idle');assert.equal(v.account,s.account);return v;}
try{
 for(const alias of ['work','personal']){
  let account;try{account=await json('account','get',alias);}catch{account=await json('account','add','codex',alias);}assert.equal(account.authBackend,'brokered-access-token');
  await cli('account','login',alias);await cli('account','status',alias);
 }
 for(let i=0;i<2;i++){
  const work=join(repo,'.cxz-test-workspaces',`broker-${run}-${i}`);mkdirSync(join(work,'.devcontainer'),{recursive:true});
  writeFileSync(join(work,'.devcontainer/devcontainer.json'),JSON.stringify({image:i===0?'alpine:latest':'node:24-bookworm-slim',remoteUser:i===0?'root':'node',containerEnv:{OPENAI_API_KEY:'synthetic-wrong'}}));
  projects.push(await json('project','add','--alias',`broker-${run}-${i}`,work));
 }
 let a=await json('session','new','--account','work','--no-attach',projects[0].alias);
 let b=await json('session','new','--account','work','--no-attach',projects[1].alias);
 assert.notEqual(a.auth_binding,b.auth_binding);assert.equal(a.auth_backend,'brokered-access-token');
 console.log('one central login supplies two projects');
 await Promise.all([chat(a,'refresh'),chat(b,'refresh')]);
 console.log('concurrent project refresh requests succeed');
 await cli('session','send',a.id,'approval');let pending=await until(a.id,'waiting_input');await cli('session','reply',a.id,pending.pending[0].request_id,'allow');await until(a.id,'idle');
 await cli('session','send',a.id,'wait');await until(a.id,'working');await cli('session','interrupt',a.id);await until(a.id,'idle');
 a=await stop(a);
 let personal=await json('session','new','--account','personal','--no-attach',projects[0].alias);await chat(personal,'refresh');personal=await stop(personal);
 a=await json('session','resume',a.id);assert.equal(a.account,'work');await chat(a);
 const before=a.auth_binding;a=await stop(a);
 await docker('restart',installation.container);
 for(let i=0;i<80;i++){try{await cli('account','get','work');break;}catch{await delay(100);}}
 await chat(b,'refresh');
 a=await json('project','recreate','--yes','--account','work','--no-attach',projects[0].alias);assert.equal(a.auth_binding,before);await chat(a,'refresh');
 console.log('manager restart, project recreation, approvals, interrupt and account-preserving resume');
 const inventory=(await json('project','ls')).projects;
 for(const p of projects){
  const container=inventory.find(v=>v.id===p.id).container_id;
  await docker('exec',container,'test','!','-e','/cxz/state/data/accounts/work/config/auth.json');
  const files=await docker('exec',container,'sh','-c','find /cxz/state/data -type f -name "*.json*" -exec grep -l "synthetic-refresh-\\|synthetic-access-" {} + || true');assert.equal(files,'','tokens leaked to project state');
 }
 assert.equal((await json('binding','ls','work')).items.length,2);
 console.log(JSON.stringify({status:'passed',checks:['central login shared across two projects','separate account profiles','concurrent refresh serialized','approval and interrupt','resume and recreation retain binding','running project refresh after manager restart','no vendor token persisted in project JSON/journals']}));
}finally{for(const p of projects){await cli('project','down',p.id);}console.log('scratch workspaces retained; uninstall disposable manager and remove its owned volumes');}
