#!/usr/bin/env node
// A fresh isolated installation. Never removes project or shared engine volumes.
import assert from 'node:assert/strict';
import {execFile} from 'node:child_process';
import {promisify} from 'node:util';
import {existsSync,readFileSync} from 'node:fs';
import {join,resolve} from 'node:path';
const exec=promisify(execFile);
const [state,root,image,previous]=process.argv.slice(2);assert(state&&root&&image&&previous);
assert(!existsSync(join(state,'installation.json')),'requires a fresh client state');
const binary=resolve(process.env.CXZ_BIN||'bin/cxz');
const load=()=>JSON.parse(readFileSync(join(state,'installation.json'),'utf8'));
async function cli(...args){return (await exec(binary,['--state',state,...args],{timeout:120000})).stdout;}
async function inspect(){const v=load();const c=JSON.parse((await exec('docker',['inspect',v.container])).stdout)[0];assert.equal(c.Config.Labels['cxz.owner'],v.owner);return c;}
function pass(check){console.log(JSON.stringify({check,status:'passed'}));}
try {
 await cli('install','--workspace-root',root,'--image',image);const first=await inspect();const identity=load();
 await cli('install','--image',image);assert.equal((await inspect()).Id,first.Id);await cli('doctor');pass('fresh installation and repeated install keep manager identity');
 await exec('docker',['stop',first.Id]);await cli('install','--image',image);assert.equal((await inspect()).Id,first.Id);pass('install resumes stopped owned manager');
 try{await cli('update','--image','invalid.invalid/cxz:missing');assert.fail('invalid image accepted');}catch(e){assert(!String(e).includes('invalid image accepted'));}
 assert.equal((await inspect()).Id,first.Id);assert.equal(load().image,image);pass('unavailable update preserves running manager and locator');
 await cli('update','--image',previous);const updated=await inspect();assert.notEqual(updated.Id,first.Id);assert.equal(load().previous_image,image);
 await cli('rollback');const rolled=await inspect();assert.notEqual(rolled.Id,updated.Id);assert.equal(load().image,image);assert.equal(load().owner,identity.owner);assert.equal(load().state_volume,identity.state_volume);await cli('doctor');pass('update and rollback retain installation identity and volumes');
} finally {
 if(existsSync(join(state,'installation.json'))){await inspect();await cli('uninstall');console.log(JSON.stringify({manager_removed:true,named_volumes_retained:true,client_state:state}));}
}
