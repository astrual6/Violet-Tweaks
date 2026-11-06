"""Real Chromium smoke test for startup and all major navigation routes.
Requires Python Playwright and /usr/bin/chromium; no actual Windows changes.
"""
import json
from pathlib import Path
from playwright.sync_api import sync_playwright

root=Path(__file__).resolve().parent
source=(root/'ui/index.html').read_text()
seed={'catalog':json.loads((root/'data/tweaks.json').read_text()),'applied':[],'admin':False,'windows':True,'logs':[]}
source=source.replace('__LOCAL_TOKEN__','TEST_TOKEN').replace('__INITIAL_DATA__',json.dumps(seed))
import re
source=re.sub(r'<script src="/ui/(?:boot|app)\.js[^\"]*" defer></script>','',source)
source=re.sub(r'<link rel="stylesheet" href="/ui/styles\.css">','',source)
with sync_playwright() as p:
 browser=p.chromium.launch(headless=True,executable_path='/usr/bin/chromium',args=['--no-sandbox'])
 page=browser.new_page(viewport={'width':1440,'height':900})
 page.set_default_timeout(2000)
 errors=[]
 page.on('pageerror',lambda e:errors.append(str(e)))
 page.set_content(source,wait_until='domcontentloaded')
 page.add_style_tag(content=(root/'ui/styles.css').read_text())
 # The API is simulated here to validate rendering/UI without editing a computer.
 page.add_script_tag(content='''window.fetch = async (path) => ({ok:true,async json(){
 const seed=JSON.parse(document.getElementById('vt-seed').textContent);
 if(path==='/api/status')return {ok:true,cpu:28,ramUsed:7.1,ramTotal:16,gpu:null,gpuTemp:null};
 if(path==='/api/init')return {...seed,ok:true};
 return {ok:true,processes:[],startups:[],jobs:[],items:[],message:'Test OK'};
 }});''')
 page.add_script_tag(content=(root/'ui/boot.js').read_text())
 page.add_script_tag(content=(root/'ui/app.js').read_text())
 page.wait_for_timeout(100)
 assert page.evaluate('window.__VIOLET_READY__'),f'failed startup: {errors}'
 assert 'Welcome back' in page.locator('#main').inner_text()
 print('PASS initial render (Chromium)')
 for id,name in [('general','General'),('debloating','Debloating'),('networking','Networking'),('advanced','Advanced'),('resources','Resources'),('fixes','Fixes'),('booster','Game Booster'),('potato','Potato Graphics'),('restore','System Restore'),('appRemoval','App Removal'),('games','Game Profiles')]:
  page.locator(f'#nav button[data-page="{id}"]').click(timeout=2000)
  page.wait_for_timeout(40)
  assert name in page.locator('#main').inner_text()[:250],f'{name} failed navigation'
  print('PASS',name)
 assert not errors, f'JS errors: {errors}'
 assert page.locator('html').get_attribute('translate')=='no'
 assert page.locator('body').get_attribute('translate')=='no'
 page.locator('#nav button[data-page="home"]').click()
 page.screenshot(path='/mnt/data/VioletTweaks-english-preview.png')
 print('PASS no page JavaScript errors, English translation safeguards present')
 browser.close()
