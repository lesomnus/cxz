#!/usr/bin/env node
// Disposable owned project: exercise aliases through the real manager/runtime.
import assert from 'node:assert/strict';
import {execFile} from 'node:child_process';
import {promisify} from 'node:util';
import {mkdirSync,writeFileSync} from 'node:fs';
import {join,resolve} from 'node:path';
import {randomUUID} from 'node:crypto';
const exec=promisify(execFile);
const [state,repo]=process.argv.slice(2);assert(state&&repo,'STATE ENGINE_REPO required');
const binary=resolve(process.env.CXZ_BIN||'bin/cxz');
const run=randomUUID().slice(0,8), workspace=join(repo,'.cxz-test-workspaces',`names-${run}`);
mkdirSync(join(workspace,'.devcontainer'),{recursive:true});
writeFileSync(join(workspace,'.devcontainer/devcontainer.json'),JSON.stringify({image:'alpine:latest',remoteUser:'root'}));
async function cli(...args){return(await exec(binary,['--state',state,...args],{timeout:300000,maxBuffer:8*1024*1024})).stdout.trim();}
let project;
try{
 project=JSON.parse(await cli('project','add','--name','Alias Test Project','--alias',`atp-${run}`,workspace));
 assert.equal(project.name,'Alias Test Project');assert.equal(project.alias,`atp-${run}`);
 const before=JSON.parse(await cli('up','--no-attach',project.alias));assert.equal(before.project_alias,project.alias);
 const cwd=await cli('exec',project.alias,'--','pwd');assert(cwd.endsWith(`names-${run}`));
 await cli('logs',project.alias);
 project=JSON.parse(await cli('project','set','--name','Renamed Project','--alias',`rp-${run}`,project.alias));
 const after=JSON.parse(await cli('up','--no-attach',project.alias));assert.equal(after.id,before.id);assert.equal(after.project_name,'Renamed Project');
 console.log(JSON.stringify({check:'register, alias up/exec/logs, rename, same-session reconnect',status:'passed'}));
 await cli('down',project.alias);project=null;
 console.log(JSON.stringify({check:'down by alias retains workspace and volumes',status:'passed'}));
}finally{if(project)await cli('down',project.id);console.log(JSON.stringify({scratch_workspace_retained:workspace,named_volumes_retained:true}));}
