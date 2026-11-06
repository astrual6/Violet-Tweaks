const fs=require('fs'),vm=require('vm'),assert=require('assert');
const data=JSON.parse(fs.readFileSync(__dirname+'/data/tweaks.json','utf8'));
const elems={};
const mk=()=>({innerHTML:'',textContent:'',value:'',style:{height:'',setProperty(){}},classList:{add(){},remove(){},toggle(){}},addEventListener(){},append(){},querySelectorAll(){return[]}});
const get=(k)=>elems[k]||(elems[k]=mk());
const document={
 querySelector:(k)=>k==='meta[name="vt-token"]'?{content:'TEST_TOKEN'}:get(k),
 querySelectorAll(){return[]},createElement(){return mk()},getElementById:(k)=>k==='vt-seed'?{textContent:JSON.stringify({catalog:data,applied:[],admin:false,windows:false,logs:[]})}:get('#'+k)
};
const window={__VIOLET_READY__:false,setTimeout,clearTimeout,scrollTo(){}};
const fetch=async p=>({ok:true,async json(){if(p==='/api/init')return {ok:true,catalog:data,applied:[],admin:false,windows:false,logs:[]};return {ok:true,cpu:23,ramUsed:5,ramTotal:16,gpu:null,gpuTemp:null}}});
const ctx={document,window,fetch,AbortController,console,setInterval(){},setTimeout,clearTimeout,Set,Math,Number,JSON,Promise,String,Date};
vm.runInNewContext(fs.readFileSync(__dirname+'/ui/app.js','utf8'),ctx);
setTimeout(()=>{
  assert(window.__VIOLET_READY__,'startup readiness flag never set');
  assert(get('#main').innerHTML.includes('Welcome back'),'dashboard not rendered');
  assert(get('#nav').innerHTML.includes('Potato Graphics'),'sidebar not rendered');
  console.log('PASS: startup renders even before status response.');
},150);
