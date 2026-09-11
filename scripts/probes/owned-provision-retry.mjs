#!/usr/bin/env node
// Fault injection only into this probe's newly created owned workspace.
import assert from 'node:assert/strict';
import {execFile} from 'node:child_process';
import {promisify} from 'node:util';
import {mkdirSync,writeFileSync,readFileSync} from 'node:fs';
import {join,resolve} from 'node:path';
import {randomUUID} from 'node:crypto';
import {setTimeout as delay} from 'node:timers/promises';
const exec=promisify(execFile);
const [state,repo]=process.argv.slice(2);assert(state&&repo,'STATE ENGINE_REPO required');
const binary=resolve(process.env.CXZ_BIN||'bin/cxz');
const installation=JSON.parse(readFileSync(join(state,'installation.json'),'utf8'));
const path=join(repo,'.cxz-test-workspaces',`retry-${randomUUID().slice(0,8)}`);
mkdirSync(join(path,'.devcontainer'),{recursive:true});
writeFileSync(join(path,'.devcontainer/devcontainer.json'),JSON.stringify({image:'alpine:latest',remoteUser:'root',postStartCommand:'if [ ! -e /tmp/cxz-retry-hook ]; then touch /tmp/cxz-retry-hook; sleep 12; fi'}));
async function cli(...args){return (await exec(binary,['--state',state,...args],{timeout:180000,maxBuffer:8*1024*1024})).stdout.trim();}
async function docker(...args){return (await exec('docker',args,{timeout:60000})).stdout.trim();}
async function until(fn){for(let i=0;i<180;i++){try{const v=await fn();if(v)return v;}catch{}await delay(250);}throw Error('timeout');}
function pass(check){console.log(JSON.stringify({check,status:'passed'}));}
let project;
try {
 const pending=cli('up','--no-attach',path).catch(()=>null);
 project=await until(async()=>{const p=JSON.parse(await cli('projects')).projects?.find(p=>p.workspace===path);return p?.provision_step==='devcontainer-up'&&p;});
 // Wait for the hook, not just the stage preceding container creation.
 const container=await until(async()=>{const ids=await docker('ps','--no-trunc','-q','--filter',`label=cxz.owner=${installation.owner}`,'--filter',`label=cxz.project=${project.id}`,'--filter',`label=devcontainer.local_folder=${path}`);if(!ids)return null;await docker('exec',ids,'test','-e','/tmp/cxz-retry-hook');return ids;});
 const manager=JSON.parse(await docker('inspect',installation.container))[0];assert.equal(manager.Config.Labels['cxz.owner'],installation.owner);
 await docker('kill',installation.container);await pending;
 await docker('start',installation.container);
 const interrupted=await until(async()=>{const p=JSON.parse(await cli('projects')).projects.find(p=>p.id===project.id);return p?.provision_state==='interrupted'&&p;});
 assert.equal(interrupted.provision_step,'devcontainer-up');assert.equal(interrupted.container_id,container);
 pass('manager kill preserves checkpoint and recovers uncommitted container identity');
 // Concurrent retries converge to one project and one session.
 const sessions=await Promise.all([cli('up','--no-attach',path),cli('up','--no-attach',path)]);
 const [a,b]=sessions.map(JSON.parse);assert.equal(a.id,b.id);
 const after=JSON.parse(await cli('projects')).projects.find(p=>p.id===project.id);
 assert.equal(after.container_id,container);assert.equal(after.provision_state,'complete');assert.equal(after.provision_attempt,3);
 const ids=await docker('ps','--no-trunc','-aq','--filter',`label=cxz.owner=${installation.owner}`,'--filter',`label=cxz.project=${project.id}`,'--filter',`label=devcontainer.local_folder=${path}`);assert.equal(ids,container);
 pass('concurrent retries reuse container, volume and session');
 await cli('logs',project.id);pass('provisioning log accessible by project ID');
 await cli('down',project.id);project=null;
} finally {
 if(project)await cli('down',project.id);
 console.log(JSON.stringify({scratch_workspace_retained:path,named_volumes_retained:true}));
}
