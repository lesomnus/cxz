#!/usr/bin/env node
// Abrupt owned-project loss + rebuilding the manager's derived SQLite database.
// No credentials or model usage required; database files are moved to backups.
import assert from 'node:assert/strict';
import {execFile} from 'node:child_process';
import {promisify} from 'node:util';
import {readFileSync} from 'node:fs';
import {join,resolve} from 'node:path';
import {randomUUID} from 'node:crypto';
import {setTimeout as delay} from 'node:timers/promises';
const exec=promisify(execFile);
const [state,project]=process.argv.slice(2);assert(state&&project,'STATE PROJECT required');
const install=JSON.parse(readFileSync(join(state,'installation.json'),'utf8'));
const binary=resolve(process.env.CXZ_BIN||'bin/cxz');
async function docker(...args){return(await exec('docker',args,{timeout:60000,maxBuffer:1024*1024})).stdout.trim();}
async function cli(...args){return JSON.parse((await exec(binary,['--state',state,...args],{timeout:300000,maxBuffer:8*1024*1024})).stdout);}
function pass(check){console.log(JSON.stringify({check,status:'passed'}));}
let list=await cli('projects');
let p=list.projects.find(p=>p.id===project||p.name===project);assert(p);
let before=await cli('up',p.id,'--agent','codex','--no-attach');assert(before.vendor_id,'use a Codex project with a persisted conversation');
p=(await cli('projects')).projects.find(v=>v.id===p.id);
const owned=JSON.parse(await docker('inspect',p.container_id))[0];
assert.equal(owned.Config.Labels['cxz.owner'],install.owner);assert.equal(owned.Config.Labels['cxz.project'],p.id);
await docker('rm','-f',p.container_id);
const offline=await cli('get',before.id);assert.equal(offline.state,'interrupted');assert(!offline.pending?.length);
const after=await cli('up',p.id,'--agent','codex','--no-attach');
assert.equal(after.id,before.id);assert.equal(after.vendor_id,before.vendor_id);assert.notEqual(after.run_id,before.run_id);assert(after.last_seq>before.last_seq);
pass('abrupt project removal: same session/thread, new fenced run, journal retained');
list=await cli('projects');const ids=list.projects.filter(p=>p.id).map(p=>p.id).sort();
const manager=JSON.parse(await docker('inspect',install.container))[0];assert.equal(manager.Config.Labels['cxz.owner'],install.owner);
const volume=JSON.parse(await docker('volume','inspect',install.state_volume))[0];assert.equal(volume.Labels['cxz.owner'],install.owner);
const backup=`cxz.db.backup-${randomUUID()}`;
await docker('stop',install.container);
try{
  await docker('run','--rm','--label',`cxz.owner=${install.owner}`,'--mount',`type=volume,source=${install.state_volume},target=/state`,'--entrypoint','sh',install.image,'-c',`set -e; test ! -e /state/${backup}; mv /state/cxz.db /state/${backup}; for suffix in -wal -shm; do if test -f /state/cxz.db$suffix; then mv /state/cxz.db$suffix /state/${backup}$suffix; fi; done`);
}finally{await docker('start',install.container);}
let restored;for(let i=0;i<60;i++){try{restored=await cli('projects');break;}catch{await delay(250);}}
assert(restored,'manager did not restart');assert.deepEqual(restored.projects.filter(p=>p.id).map(p=>p.id).sort(),ids);
const current=await cli('get',after.id);assert.equal(current.run_id,after.run_id);assert.equal(current.vendor_id,after.vendor_id);
pass('manager SQLite rebuilt from manifests without restarting project supervisor');
console.log(JSON.stringify({database_backup:backup,location:'manager state volume',recoverable:true}));
