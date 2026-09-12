#!/usr/bin/env node
// ONLY a fresh, disposable installation: seeds its private tools volume with a
// deterministic Claude fixture. Never use this manager for real agent work.
import assert from 'node:assert/strict';
import {execFile} from 'node:child_process';
import {promisify} from 'node:util';
import {mkdirSync,writeFileSync,readFileSync} from 'node:fs';
import {join,resolve} from 'node:path';
import {randomUUID} from 'node:crypto';
import {setTimeout as delay} from 'node:timers/promises';
const exec=promisify(execFile);
const [state,repo]=process.argv.slice(2);assert(state&&repo,'fresh STATE ENGINE_REPO required');
const binary=resolve('bin/cxz'),fake=resolve('bin/fake-claude');
const installation=JSON.parse(readFileSync(join(state,'installation.json'),'utf8'));
const run=randomUUID().slice(0,8),work=join(repo,'.cxz-test-workspaces',`accounts-${run}`);
async function cli(...args){return(await exec(binary,['--state',state,...args],{timeout:300000,maxBuffer:8*1024*1024})).stdout.trim();}
async function json(...args){return JSON.parse(await cli(...args));}
async function docker(...args){return(await exec('docker',args,{timeout:60000,maxBuffer:1024*1024})).stdout.trim();}
const owner=JSON.parse(await docker('inspect',installation.container))[0].Config.Labels['cxz.owner'];assert.equal(owner,installation.owner);
assert.equal((await json('projects')).projects?.length||0,0,'must be a fresh manager');
const version=readFileSync('internal/distribution/release.go','utf8').match(/const ClaudeVersion = "([^"]+)"/)[1];
const arch=await docker('exec',installation.container,'uname','-m');
const cache=`/cxz/tools/claude/${version}/linux-${arch==='aarch64'?'arm64':'x64'}-musl`;
await docker('exec',installation.container,'test','!','-e',cache);
await docker('exec',installation.container,'mkdir','-p',cache);
await docker('cp',fake,`${installation.container}:${cache}/claude`);
mkdirSync(join(work,'.devcontainer'),{recursive:true});
writeFileSync(join(work,'.devcontainer/devcontainer.json'),JSON.stringify({image:'alpine:latest',remoteUser:'root',containerEnv:{OPENAI_API_KEY:'synthetic-wrong-account',ANTHROPIC_API_KEY:'synthetic-wrong-account'}}));
let project,second;
async function stopped(s){await cli('stop',s.id);for(let i=0;i<50;i++){const v=await json('get',s.id);if(v.state==='stopped')return v;await delay(100);}throw new Error('stop timeout');}
async function contextCheck(s,alias){
 await cli('send',s.id,'account-context');
 for(let i=0;i<80;i++){
  const v=await json('get',s.id);
  if(v.state==='idle'&&(v.last_seq||0)>(s.last_seq||0)+2){
   const p=(await json('projects')).projects.find(p=>p.id===s.project_id);
   const journal=await docker('exec',p.container_id,'cat',`/cxz/state/data/sessions/${s.id}/events.jsonl`);
   assert(journal.includes(`profile=${alias} home=${alias} inherited-key=false`));return;
  }await delay(100);
 }throw new Error('account context timeout');
}
try{
 for(const alias of ['personal','work']){await cli('account','add','--agent','claude',alias);}
 await cli('account','add','--agent','claude','missing');
 project=await json('project','add','--name','Account Test Project','--alias',`account-${run}`,work);
 await assert.rejects(()=>cli('new','--account','missing','--no-attach',project.alias));
 await cli('account','login','--project',project.alias,'personal');
 await cli('account','status','--project',project.alias,'personal');
 let personal=await json('new','--account','personal','--no-attach',project.alias);assert.equal(personal.account,'personal');
 assert.equal(personal.project_name,'Account Test Project');assert.equal(personal.project_alias,project.alias);
 await cli('exec',project.alias,'--','pwd');await cli('logs',project.alias);
 await contextCheck(personal,'personal');
 const p=(await json('projects')).projects.find(p=>p.id===personal.project_id);
 await docker('exec',p.container_id,'test','!','-e','/cxz/state/data/accounts/work');
 await assert.rejects(()=>cli('up','--account','work','--no-attach',project.alias));
 await assert.rejects(()=>cli('account','login','--project',project.alias,'personal'));
 personal=await stopped(personal);
 await cli('account','login','--project',project.alias,'work');
 let workSession=await json('new','--account','work','--no-attach',project.alias);assert.notEqual(workSession.id,personal.id);assert.equal(workSession.account,'work');await contextCheck(workSession,'work');
 workSession=await stopped(workSession);
 project=await json('project','set','--name','Renamed Account Project','--alias',`renamed-${run}`,project.alias);
 personal=await json('resume',personal.id);assert.equal(personal.account,'personal');await contextCheck(personal,'personal');personal=await stopped(personal);
 await docker('restart',installation.container);
 for(let i=0;i<60;i++){try{await cli('account','get','work');break;}catch{await delay(100);}}
 const oldContainer=(await json('projects')).projects.find(p=>p.id===project.id).container_id;
 workSession=await json('recreate','--yes','--account','work','--no-attach',project.alias);assert.equal(workSession.account,'work');await contextCheck(workSession,'work');
 assert.equal(workSession.project_name,'Renamed Account Project');assert.equal(workSession.project_alias,project.alias);
 assert.notEqual((await json('projects')).projects.find(p=>p.id===project.id).container_id,oldContainer);
 await docker('exec',installation.container,'test','!','-e','/var/lib/cxz/accounts');
 const other=work+'-other';mkdirSync(join(other,'.devcontainer'),{recursive:true});
 writeFileSync(join(other,'.devcontainer/devcontainer.json'),JSON.stringify({image:'alpine:latest',remoteUser:'root'}));
 second=await json('project','add','--alias',`other-${run}`,other);
 await assert.rejects(()=>cli('new','--account','work','--no-attach',second.alias));
 const otherProject=(await json('projects')).projects.find(p=>p.id===second.id);
 await docker('exec',otherProject.container_id,'test','!','-e','/cxz/state/data/accounts/work/config/.credentials.json');
 console.log(JSON.stringify({status:'passed',checks:['missing login rejected','project-local staged login/status','no manager credential store','isolated HOME/config and inherited keys removed','different-account live attach and active login rejected','two accounts distinct threads','resume retains original account','manager restart and project recreate retain account','another project requires independent login','project name/alias regression']}));
}finally{if(second)await cli('down',second.id);if(project)await cli('down',project.id);console.log(JSON.stringify({scratch_workspace_retained:work,test_only_tools_volume:installation.tools_volume}));}
