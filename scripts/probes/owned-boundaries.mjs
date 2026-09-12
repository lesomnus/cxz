#!/usr/bin/env node
// Uses an installed test manager. Creates/removes only this run's scratch project.
import assert from 'node:assert/strict';
import {execFile} from 'node:child_process';
import {promisify} from 'node:util';
import {mkdirSync,writeFileSync,readFileSync} from 'node:fs';
import {join,resolve} from 'node:path';
import {randomUUID} from 'node:crypto';
const exec=promisify(execFile);
const [state,engineRepo]=process.argv.slice(2);assert(state&&engineRepo,'STATE ENGINE_REPO required');
const binary=resolve(process.env.CXZ_BIN||'bin/cxz');
const install=JSON.parse(readFileSync(join(state,'installation.json'),'utf8'));
const run=`boundary-${randomUUID().slice(0,12)}`;
const workspace=join(engineRepo,'.cxz-test-workspaces',run);
mkdirSync(join(workspace,'.devcontainer'),{recursive:true});
writeFileSync(join(workspace,'.devcontainer/devcontainer.json'),JSON.stringify({image:'alpine:latest',remoteUser:'root'}));
async function docker(...args){return (await exec('docker',args,{timeout:60000})).stdout.trim();}
async function cli(...args){return (await exec(binary,['--state',state,'--format','json',...args],{timeout:300000,maxBuffer:8*1024*1024})).stdout.trim();}
async function rejected(fn,pattern){try{await fn();assert.fail('unexpected success');}catch(e){assert.match(e.stderr||e.message,pattern);}}
function pass(check){console.log(JSON.stringify({check,status:'passed'}));}
let foreign;let owned;
try{
  foreign=await docker('run','-d','--name',`cxz-${run}`,'--label',`cxz.probe.run=${run}`,'--label',`devcontainer.local_folder=${workspace}`,'alpine:latest','sleep','infinity');
  await rejected(()=>cli('project','up','--no-attach',workspace),/foreign container/);
  assert(JSON.parse(await docker('inspect',foreign))[0].State.Running);
  pass('foreign up rejected without mutation');
  await rejected(()=>cli('project','recreate','--no-attach',workspace),/pass --yes/);
  pass('noninteractive recreate requires explicit confirmation');
  const session=JSON.parse(await cli('project','recreate','--yes','--no-attach',workspace));
  owned=JSON.parse(await cli('project','ls')).projects.find(p=>p.id===session.project_id);
  const v=JSON.parse(await docker('inspect',owned.container_id))[0];
  assert.equal(v.Config.Labels['cxz.owner'],install.owner);assert.equal(v.Config.Labels['cxz.project'],owned.id);
  await rejected(()=>docker('inspect',foreign),/no such/i);foreign=null;
  assert(v.Mounts.find(m=>m.Destination==='/cxz/tools'&&m.RW===false));
  assert(!v.Mounts.some(m=>m.Destination.includes('docker.sock')));
  pass('confirmed recreate creates owned container with RO tools and no Docker socket');
  await rejected(()=>cli('project','exec',owned.id,'--','cxz','project','ls'),/PermissionDenied/);
  await rejected(()=>cli('project','exec',owned.id,'--','cxz','get','ffffffffffffffffffffffff'),/NotFound/);
  pass('in-container client rejects fleet access and foreign session ids');
  await rejected(()=>cli('session','new','--agent','codex','--no-attach',workspace),/active claude session/);
  pass('second active agent rejected');
  await cli('project','down',workspace);
  await rejected(()=>docker('inspect',owned.container_id),/no such/i);
  const volume=v.Mounts.find(m=>m.Destination==='/cxz/state').Name;
  assert.equal(JSON.parse(await docker('volume','inspect',volume))[0].Labels['cxz.project'],owned.id);
  pass('down removes project container and retains its state volume');
  owned=null;
} finally {
  if(owned)await cli('project','down',owned.id);
  if(foreign){const v=JSON.parse(await docker('inspect',foreign))[0];assert.equal(v.Config.Labels['cxz.probe.run'],run);await docker('rm','-f',foreign);}
  console.log(JSON.stringify({scratch_workspace_retained:workspace,named_volumes_retained:true}));
}
