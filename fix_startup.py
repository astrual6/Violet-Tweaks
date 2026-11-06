from pathlib import Path
root=Path('/mnt/data/VioletTweaks')
app=root/'ui/app.js'
s=app.read_text()
old="async function api(path,options={}){const cfg={headers:{'X-Violet-Token':TOKEN},...options};if(options.body){cfg.headers={'X-Violet-Token':TOKEN,'Content-Type':'application/json'};cfg.body=JSON.stringify(options.body);cfg.method='POST'}const res=await fetch(path,cfg);let data;try{data=await res.json()}catch{throw new Error('The local service did not return JSON')}if(!res.ok||data.ok===false)throw new Error(data.error||'Request failed');return data}"
new="""async function api(path,options={}){
 const controller=new AbortController();
 const timeout=window.setTimeout(()=>controller.abort(), options.timeoutMs||8500);
 try {
  const cfg={headers:{'X-Violet-Token':TOKEN},signal:controller.signal,...options};
  delete cfg.timeoutMs;
  if(options.body){cfg.headers={'X-Violet-Token':TOKEN,'Content-Type':'application/json'};cfg.body=JSON.stringify(options.body);cfg.method='POST'}
  const res=await fetch(path,cfg);
  let data;
  try{data=await res.json()}catch{throw new Error('The local service did not return JSON')}
  if(!res.ok||data.ok===false)throw new Error(data.error||'Request failed');
  return data;
 } catch(e) {
  if(e.name==='AbortError')throw new Error('Local Windows service timed out ('+path+'). Try Restart app.');
  throw e;
 } finally{window.clearTimeout(timeout)}
}"""
if old not in s: raise Exception('api anchor missing')
s=s.replace(old,new)
old="async function syncInit(){const d=await api('/api/init');S.catalog=d.catalog||[];S.applied=new Set(d.applied||[]);S.admin=d.admin;S.windows=d.windows;S.logs=d.logs||[];await loadStats();render()}"
new="""async function syncInit(){
 // Bootstrap from the server-rendered catalog so hardware sensors can never block the first render.
 const seed=document.getElementById('vt-seed');
 if(seed){try{const d=JSON.parse(seed.textContent);S.catalog=d.catalog||[];S.applied=new Set(d.applied||[]);S.admin=d.admin;S.windows=d.windows;S.logs=d.logs||[]}catch(e){console.warn('Embedded startup data:',e)}}
 render();
 window.__VIOLET_READY__=true;
 // Read runtime state separately; render again only when updated. It has a firm timeout.
 try{
  const d=await api('/api/init',{timeoutMs:6500});
  S.catalog=d.catalog||S.catalog;S.applied=new Set(d.applied||[]);S.admin=d.admin;S.windows=d.windows;S.logs=d.logs||[];
  render();
 }catch(e){console.error('Initialization:',e);toast('Local service unavailable: '+e.message,'error')}
 // Live hardware data is optional and must never be awaited before showing the dashboard.
 loadStats();
}"""
if old not in s: raise Exception('syncInit anchor missing')
s=s.replace(old,new)
old="(async()=>{try{await syncInit();setInterval(loadStats,7000);setInterval(()=>{if(S.page==='fixes')loadJobs()},4000);setInterval(()=>{if(S.page==='ping')loadPing()},8500)}catch(e){$('#main').innerHTML=`<div class=\"settings-panel\"><h2>Cannot start VioletTweaks</h2><p>${esc(e.message)}</p><p>Run the original VioletTweaks.exe and allow it to launch the application window.</p></div>`}})();"
new="""(async()=>{try{
 await syncInit();
 setInterval(loadStats,7000);
 setInterval(()=>{if(S.page==='fixes')loadJobs()},4000);
 setInterval(()=>{if(S.page==='ping')loadPing()},8500);
 }catch(e){console.error('Startup failed',e);const main=$('#main');if(main)main.innerHTML=`<div class=\"settings-panel\"><h2>VioletTweaks could not load</h2><p>${esc(e.message)}</p><button class=\"btn btn-primary\" onclick=\"location.reload()\">Retry</button><p>Please check the startup.log in AppData\\Local\\VioletTweaks.</p></div>`;
 }} )();"""
# above uses inline onclick blocked by CSP; replace with DOM event in boot.js; avoid inline onclick
new=new.replace(' onclick=\\\"location.reload()\\\"',' data-startup-retry')
if old not in s: raise Exception('end anchor missing')
s=s.replace(old,new)
app.write_text(s)

index=root/'ui/index.html'
t=index.read_text()
t=t.replace('<script src="/ui/app.js" defer></script>', '<script src="/ui/boot.js?v=1.0.2" defer></script><script src="/ui/app.js?v=1.0.2" defer></script>')
t=t.replace('<body>','<body><script type="application/json" id="vt-seed">__INITIAL_DATA__</script>')
index.write_text(t)

(root/'ui/boot.js').write_text('''/* Independent boot watchdog: detects missing JavaScript, API stalls and startup errors. */
(function(){
 'use strict';
 window.__VIOLET_READY__=false;
 var problem='';
 window.addEventListener('error',function(event){
  if(event.message)problem=String(event.message);
 });
 window.addEventListener('unhandledrejection',function(event){
  if(event.reason)problem=String(event.reason.message||event.reason);
 });
 document.addEventListener('click',function(event){
  if(event.target.closest('[data-startup-retry]')){location.reload();}
 });
 setTimeout(function(){
  if(window.__VIOLET_READY__)return;
  var main=document.getElementById('main');
  if(!main)return;
  while(main.firstChild)main.removeChild(main.firstChild);
  var pane=document.createElement('div');pane.className='settings-panel';
  var header=document.createElement('h2');header.textContent='Startup took too long';pane.appendChild(header);
  var p=document.createElement('p');p.textContent='VioletTweaks could not start its interface. '+(problem?'Browser error: '+problem:'Try restarting the app. A local service or browser script may have been blocked.');pane.appendChild(p);
  var button=document.createElement('button');button.className='btn btn-primary';button.textContent='Retry loading';button.addEventListener('click',function(){location.reload()});pane.appendChild(button);
  var hint=document.createElement('p');hint.textContent='Diagnostics: %LOCALAPPDATA%\\\\VioletTweaks\\\\startup.log';pane.appendChild(hint);
  main.appendChild(pane);
 },9000);
})();
''')

main=root/'main.go'
s=main.read_text()
# write startup diagnostic log as early as possible
s=s.replace('func main() {\n\ta := newApp()', '''func main() {
 // Keep diagnostics even when the executable runs without a console window.
 if dir := os.Getenv("LOCALAPPDATA"); dir != "" {
  logDir := filepath.Join(dir, "VioletTweaks")
  if err := os.MkdirAll(logDir, 0700); err == nil {
   if f, err := os.OpenFile(filepath.Join(logDir, "startup.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600); err == nil {
    defer f.Close()
    log.SetOutput(f)
   }
  }
 }
 log.Println("Launching VioletTweaks 1.0.2")
 a := newApp()''')
old='''html := strings.ReplaceAll(string(b), "__LOCAL_TOKEN__", a.token)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")'''
new='''html := strings.ReplaceAll(string(b), "__LOCAL_TOKEN__", a.token)
 // Embed only server-owned data; first UI render never waits on expensive hardware probes.
 a.mu.Lock()
 applied := make([]string, 0, len(a.state.Applied))
 for id := range a.state.Applied { applied = append(applied, id) }
 history := append([]Activity{}, a.state.Activities...)
 a.mu.Unlock()
 seed, _ := json.Marshal(map[string]any{"catalog": a.catalog, "applied": applied, "admin": a.admin, "windows": runtime.GOOS == "windows", "logs": history})
 html = strings.ReplaceAll(html, "__INITIAL_DATA__", string(seed))
 w.Header().Set("Content-Type", "text/html; charset=utf-8")'''
if old not in s: raise Exception('Go page anchor missing')
s=s.replace(old,new)
s=s.replace('w.Header().Set("Cache-Control", "max-age=3600")','w.Header().Set("Cache-Control", "no-store")')
s=s.replace('"version": "1.0.0"','"version": "1.0.2"')
main.write_text(s)
print('Patched startup')
