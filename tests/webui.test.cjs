'use strict';
const fs=require('node:fs'),vm=require('node:vm'),assert=require('node:assert/strict');
const source=fs.readFileSync(__dirname+'/../module/webroot/app.js','utf8');
function boot(bridge=true){
 const elements=new Map(),events={},winEvents={},calls=[],timers=[],intervals=[];
 class E{constructor(){this.textContent='';this.dataset={};this.attrs={};this.children=[];this.listeners={};this.open=false;this.hidden=false;this.value='';}
 set innerHTML(_v){throw Error('unsafe innerHTML');} setAttribute(k,v){this.attrs[k]=v;} append(...a){this.children.push(...a);} replaceChildren(){this.children=[];}
 addEventListener(k,cb){this.listeners[k]=cb;} showModal(){this.open=true;} fire(k,e={}){this.listeners[k]?.(e);}}
 const document={hidden:false,getElementById(id){if(!elements.has(id))elements.set(id,new E());return elements.get(id);},createElement(){return new E();},addEventListener(k,cb){events[k]=cb;}};
 const window={addEventListener(k,cb){winEvents[k]=cb;}};
 if(bridge)window.ksu={exec(cmd,options,callback){calls.push({cmd,options,callback});}};
 const ctx={document,window,Date,JSON,Object,Array,String,Number,Error,RegExp,setTimeout(fn){timers.push(fn);return timers.length;},clearTimeout(){},setInterval(fn){intervals.push(fn);return intervals.length;},clearInterval(){}};
 vm.runInNewContext(source,ctx);
 const $=id=>document.getElementById(id);
 const reply=(data,code=0,stderr='')=>window[calls.at(-1).callback](code,JSON.stringify(data),stderr);
 return {$,calls,reply,document,events,window,timers,intervals};
}
function snapshot(mode='observe'){return {ok:true,version:'0.1.0',config:{mode,interval:60,keep_web:true,keep_mac:true},status:{checked_at:new Date().toISOString(),daemon:{pid:1234,alive:true,responsive:true,build:26101400},web:{enabled:true,connected:false,port:13012},mac:{active:true,connected:false,device_connected:false},worker_running:true,supported:true,health:'healthy',reason:'ready'},events:Array.from({length:60},(_,i)=>({at:new Date().toISOString(),kind:'event_'+i,detail:'<img src=x onerror=alert(1)>'}))};}
let checks=0;function check(name,fn){fn();console.log('PASS',name);checks++;}
check('No bridge means offline and no shell calls',()=>{const b=boot(false);assert.equal(b.$('preview').hidden,false);assert.equal(b.$('tick').disabled,true);assert.equal(b.calls.length,0);});
check('Initial command is the literal cached view',()=>{const b=boot();assert.equal(b.calls.length,1);assert.equal(b.calls[0].cmd,'/data/adb/modules/andromeld_guardian/bin/guardian view');assert.equal(b.calls[0].options,'{}');});
check('Zero-time status is unknown, not a fake stopped daemon',()=>{const b=boot();const d=snapshot();d.status.checked_at='0001-01-01T00:00:00Z';d.status.daemon.alive=false;b.reply(d);assert.equal(b.$('daemon-state').textContent,'未知');assert.match(b.$('updated').textContent,/未知/);});
check('Only newest 30 events rendered via safe text',()=>{const b=boot();b.reply(snapshot());assert.equal(b.$('events').children.length,30);assert.equal(b.$('events').children[0].children[1].textContent,'event_59');assert.match(b.$('events').children[0].children[2].textContent,/<img/);});
check('Protection requires modal acceptance',()=>{const b=boot();b.reply(snapshot());b.$('policy').fire('click');assert.equal(b.$('confirm').open,true);assert.equal(b.calls.length,1);b.$('confirm').open=false;b.$('confirm').returnValue='cancel';b.$('confirm').fire('close');assert.equal(b.calls.length,1);b.$('policy').fire('click');b.$('confirm').open=false;b.$('confirm').returnValue='protect';b.$('confirm').fire('close');assert.match(b.calls.at(-1).cmd,/policy protect$/);});
check('Observation switch is fixed and does not stop sessions',()=>{const b=boot();b.reply(snapshot('protect'));b.$('policy').fire('click');assert.match(b.calls.at(-1).cmd,/policy observe$/);});
check('Concurrent clicks and timeouts cannot duplicate a mutation',()=>{const b=boot();b.reply(snapshot());b.$('tick').fire('click');b.$('tick').fire('click');assert.equal(b.calls.length,2);b.timers.at(-1)();b.$('tick').fire('click');assert.equal(b.calls.length,2);assert.equal(b.$('tick').disabled,true);b.reply(snapshot());assert.equal(b.$('tick').disabled,false);});
check('Untrusted interval cannot become shell input',()=>{const b=boot();b.reply(snapshot());b.$('interval').fire('change',{target:{value:'60; rm -rf /'}});assert.equal(b.calls.length,1);b.$('interval').fire('change',{target:{value:'120'}});assert.match(b.calls.at(-1).cmd,/interval 120$/);});
check('Backend errors do not reveal stderr or retain stale success',()=>{const b=boot();b.reply({ok:false,error:'check_in_progress'},2,'PRIVATE_SECRET');assert.match(b.$('feedback').textContent,/check_in_progress/);assert.doesNotMatch(b.$('feedback').textContent,/SECRET/);assert.equal(b.$('daemon-state').textContent,'未知');});
check('Hidden page does not dispatch polling',()=>{const b=boot();b.reply(snapshot());b.document.hidden=true;b.events.visibilitychange();b.intervals.at(-1)();assert.equal(b.calls.length,1);});
check('Stale and future evidence is visibly marked',()=>{for(const delta of [-3600000,3600000]){const b=boot();const d=snapshot();d.status.checked_at=new Date(Date.now()+delta).toISOString();b.reply(d);assert.equal(b.$('health').textContent,'状态未确认');}});
check('Static assets do not enable network or HTML evaluation',()=>{const html=fs.readFileSync(__dirname+'/../module/webroot/index.html','utf8');assert.match(html,/connect-src 'none'/);assert.doesNotMatch(source,/innerHTML|eval\(|new Function|fetch\(|XMLHttpRequest/);});
console.log(`${checks} WebUI checks passed`);
