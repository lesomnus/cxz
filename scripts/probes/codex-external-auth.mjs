#!/usr/bin/env node
// Native protocol check using synthetic JWTs only. Never starts a model turn.
import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {mkdtempSync,existsSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join,resolve} from 'node:path';
import {createInterface} from 'node:readline';
const binary=resolve(process.argv[2]);
const home=mkdtempSync(join(tmpdir(),'cxz-native-auth-'));
const env={PATH:process.env.PATH,HOME:home,CODEX_HOME:home,XDG_CONFIG_HOME:home};
const child=spawn(binary,['-c','cli_auth_credentials_store="ephemeral"','app-server','--listen','stdio://'],{env,stdio:['pipe','pipe','pipe']});
child.stderr.resume();
const pending=new Map();
createInterface({input:child.stdout}).on('line',line=>{const v=JSON.parse(line);const p=pending.get(v.id);if(p){pending.delete(v.id);v.error?p.reject(Error('native authentication protocol rejected: '+JSON.stringify(v.error))):p.resolve(v.result);}});
const send=v=>child.stdin.write(JSON.stringify(v)+'\n');
let id=0;
async function call(method,params){const key=++id;return new Promise((resolve,reject)=>{pending.set(key,{resolve,reject});send({id:key,method,params});});}
const timeout=setTimeout(()=>{child.kill('SIGKILL');for(const p of pending.values())p.reject(Error('native protocol timeout'));},15000);
try{
 await call('initialize',{clientInfo:{name:'cxz-auth-probe',version:'1'},capabilities:{experimentalApi:true}});send({method:'initialized'});
 const encode=v=>Buffer.from(JSON.stringify(v)).toString('base64url');
 const token=[encode({alg:'none'}),encode({sub:'synthetic-user',email:'synthetic@example.invalid',exp:Math.floor(Date.now()/1000)+3600,'https://api.openai.com/auth':{chatgpt_account_id:'synthetic-account',chatgpt_user_id:'synthetic-user',chatgpt_plan_type:'plus'}}),'synthetic'].join('.');
 const login=await call('account/login/start',{type:'chatgptAuthTokens',accessToken:token,chatgptAccountId:'synthetic-account',chatgptPlanType:'plus'});
 assert.equal(login.type,'chatgptAuthTokens');
 const status=await call('account/read',{refreshToken:false});assert.equal(status.account.type,'chatgpt');
 assert.equal(existsSync(join(home,'auth.json')),false);
 console.log(JSON.stringify({status:'passed',checks:['native external token login accepted','account/read reports ChatGPT','ephemeral auth does not create auth.json'],model_turns:0,real_credentials:false,scratch:home}));
}finally{clearTimeout(timeout);child.stdin.end();child.kill('SIGKILL');}
