"""Real Chromium P2.5 checks against an isolated server, with real SQLite data.
Requires Playwright, Chromium, --binary and --seed-helper (built from the Go
fixture beside this file). No QQ connection and no production data modifications.
"""
import argparse
import asyncio
import json
import os
from pathlib import Path
import secrets
import shutil
import socket
import subprocess
import tempfile
import time
from playwright.async_api import async_playwright

async def check(base, password, root, artifacts, chromium, restart):
    async with async_playwright() as p:
        proxy = {'server':os.environ['HTTPS_PROXY'],'bypass':'127.0.0.1,localhost'} if os.environ.get('HTTPS_PROXY') else None
        browser = await p.chromium.launch(executable_path=chromium, args=['--no-sandbox','--disable-dev-shm-usage'], proxy=proxy)
        context = await browser.new_context(viewport={'width':1440,'height':900},locale='zh-CN')
        async def api(path, method='GET', value=None):
            response=await context.request.fetch(base+path,method=method,data=value,headers={'Content-Type':'application/json'})
            body=await response.json();assert response.ok and body['code']==0, body
            return body.get('data')
        response=await context.request.post(base+'/login',data={'username':'admin','password':password},headers={'Content-Type':'application/json'})
        assert (await response.json())['code']==0
        page=await context.new_page();errors=[];page.on('pageerror',lambda e:errors.append(page.url+' '+str(e)))
        await page.goto(base+'/#/stats',wait_until='domcontentloaded')
        await page.wait_for_function('historyChart?.data.labels.length===30 && document.querySelector("#stat-sent")?.textContent==="5"')
        assert (await api('/api/stats'))['user_count']==2
        await page.evaluate('window.__initialChartColor=historyChart.data.datasets[0].borderColor')
        await page.emulate_media(color_scheme='dark')
        await page.wait_for_function('historyChart.data.datasets[0].borderColor!==window.__initialChartColor')
        await page.emulate_media(color_scheme='light')
        await page.wait_for_function('historyChart.data.datasets[0].borderColor===window.__initialChartColor')
        await page.wait_for_function('document.querySelector(".page-background").naturalWidth===1920')
        await page.screenshot(path=str(artifacts/'stats-desktop.png'),full_page=True,animations='disabled')
        async def navigate(route):
            assert await page.evaluate('(r)=>navigateTo("/"+r)',route)
            selectors={'config':'#theme-mode','stats':'#stat-sent','about':'#about-runtime dd','community':'h1.page-title','bots':'#bots-manage-list [data-action]','plugins':'#plugin-save','rules':'#rules-new','sandbox':'#sandbox-send','logs':'#log-search','api-test':'#api-method','users':'#users-list'}
            await page.wait_for_selector(selectors[route])
            if route=='stats':await page.wait_for_function('historyChart?.data.labels.length>0 && document.querySelector("#stat-sent").textContent!=="-"')
        async def field(selector,value):await page.locator(selector).locator('input,textarea').first.fill(value)
        async def select(selector,value):await page.locator(selector).evaluate('(n,v)=>{n.value=v;n.dispatchEvent(new Event("change",{bubbles:true}))}',value)
        async def save():
            revision=(await api('/api/config'))['revision']
            await page.locator('#config-save').click()
            await page.wait_for_function('(r)=>configData.revision!==r && !pageScope.busy',arg=revision)
        await navigate('config')
        await page.wait_for_function('document.querySelector("#background-preview").complete && document.querySelector("#background-preview").naturalWidth>0')
        assert await page.evaluate('typeof mdui.setColorScheme==="function" && typeof mdui.removeColorScheme==="function" && typeof mdui.setTheme==="function"')
        initial_color=await page.evaluate('getComputedStyle(document.documentElement).getPropertyValue("--mdui-color-primary")')
        await select('#theme-preset','custom');await field('#theme-primary','#123456');await select('#theme-mode','dark')
        assert await page.evaluate('document.documentElement.classList.contains("mdui-theme-dark")')
        assert await page.evaluate('getComputedStyle(document.documentElement).getPropertyValue("--mdui-color-primary")')!=initial_color
        assert (await api('/api/appearance'))['mode']=='auto'
        await page.locator('#appearance-cancel').click()
        assert await page.evaluate('document.documentElement.classList.contains("mdui-theme-auto")')
        assert await page.evaluate('getComputedStyle(document.documentElement).getPropertyValue("--mdui-color-primary")')==initial_color
        await select('#background-type','url');await field('#background-url','https://example.invalid/missing.png')
        await page.wait_for_function('document.querySelector("#appearance-feedback").textContent.includes("背景加载失败")')
        await page.locator('#config-save').click()
        await page.wait_for_function('document.querySelector("#config-feedback").textContent.includes("背景尚未加载")')
        assert (await api('/api/appearance'))['background_url']=='/static/img/default-background.png'
        await page.locator('#appearance-cancel').click()
        # The original URL also works as a custom URL without a server-side fetch.
        await select('#background-type','url');await field('#background-url',await page.evaluate('AppearanceUI.fallback'))
        await page.wait_for_function('document.querySelector("#background-preview").naturalWidth===1920')
        await save();assert (await api('/api/config'))['webui']['appearance']['background']['type']=='url'
        # Actual upload, preview and saved URL are identical.
        await select('#background-type','upload');await page.locator('#background-file').set_input_files(root/'upload-test.png')
        await page.locator('#background-upload').click()
        await page.wait_for_function('document.querySelector("#background-url").value.startsWith("/appearance/images/") && document.querySelector("#background-preview").naturalWidth===256')
        uploaded=await page.locator('#background-url').evaluate('(n)=>n.value')
        assert (await api('/api/appearance'))['background_url']!=uploaded
        await select('#theme-preset','#0061A4');await select('#theme-mode','dark');await save()
        public=await api('/api/appearance');assert public=={'background_url':uploaded,'primary_color':'#0061A4','mode':'dark'}
        await page.screenshot(path=str(artifacts/'appearance-desktop.png'),full_page=True,animations='disabled')
        await page.reload(wait_until='domcontentloaded')
        await page.wait_for_function('document.documentElement.classList.contains("mdui-theme-dark") && document.querySelector(".page-background").naturalWidth===256')
        await page.wait_for_selector('#theme-mode')
        assert await page.locator('#theme-mode').evaluate('(n)=>n.value')=='dark'
        new_context=await browser.new_context(viewport={'width':390,'height':844})
        login=await new_context.new_page();login.on('pageerror',lambda e:errors.append('login '+str(e)))
        await login.goto(base+'/login',wait_until='domcontentloaded')
        await login.wait_for_function('document.documentElement.classList.contains("mdui-theme-dark") && document.querySelector(".page-background").naturalWidth===256')
        assert await login.evaluate('ShizukuAppearance.primary_color')=='#0061A4'
        await login.screenshot(path=str(artifacts/'login-mobile-dark.png'),animations='disabled')
        public_response=await new_context.request.get(base+'/api/appearance');public_body=await public_response.json()
        assert len(public_body['data'])==3 and not any(s in json.dumps(public_body) for s in ['password','app_secret','access_token'])
        await new_context.close()
        # A process restart keeps both SQLite data and the global appearance.
        before=await api('/api/stats');await asyncio.to_thread(restart)
        after=await api('/api/stats')
        for key in ['user_count','group_count','today_dau','sent_messages','received_messages']:assert before[key]==after[key],(key,before,after)
        assert await api('/api/appearance')==public
        await page.reload(wait_until='domcontentloaded');await page.wait_for_selector('#stats-retention')
        await select('#stats-retention','7');await field('#stats-cleanup-time','04:15');await save()
        assert (await api('/api/stats/storage'))['settings']['cleanup_time']=='04:15'
        history=await api('/api/stats/history?days=90');assert len(history)==7 and sum(d['sent'] for d in history)==3
        await navigate('stats')
        await page.wait_for_function('document.querySelector("#history-range").value==="7" && historyChart.data.labels.length===7')
        assert await page.locator('#history-range mdui-menu-item[value="90"]').evaluate('(n)=>n.disabled')
        await navigate('config')
        await page.locator('#stats-cleanup').click()
        await page.wait_for_function('!pageScope.busy && [...document.querySelectorAll("mdui-snackbar[open]")].some(n=>n.textContent.includes("过期数据已清理"))')
        # Raw global settings include the new fields and use the same revision guard.
        raw=json.loads(await page.locator('#config-textarea').input_value());raw['statistics']['retention_days']=30
        await page.locator('#config-textarea').fill(json.dumps(raw,ensure_ascii=False,indent=2))
        await page.locator('#config-save-raw').click()
        await page.wait_for_function('configData.statistics.retention_days===30 && !pageScope.busy')
        assert (await api('/api/stats/storage'))['settings']['retention_days']==30
        await page.locator('#stats-retention').scroll_into_view_if_needed()
        await page.screenshot(path=str(artifacts/'statistics-desktop.png'),animations='disabled')
        await navigate('about');assert 'botgo-plus' in await page.locator('#about-runtime').inner_text()
        assert 'go1.' in await page.locator('#about-runtime').inner_text()
        await page.screenshot(path=str(artifacts/'about-desktop.png'),full_page=True,animations='disabled')
        # Every route uses the shared gutter and fits all required widths.
        layout=[]
        for width,height,padding in [(1440,900,'32px'),(1024,768,'24px'),(390,844,'16px'),(360,800,'16px')]:
            await page.set_viewport_size({'width':width,'height':height})
            for route in ['stats','bots','logs','api-test','plugins','rules','community','sandbox','config','users','about']:
                await navigate(route)
                await page.wait_for_function('document.querySelectorAll(".page-view").length===1')
                metrics=await page.evaluate('({width:innerWidth,scroll:document.scrollingElement.scrollWidth,padding:getComputedStyle(document.querySelector(".page-view:not([hidden])")).paddingLeft,contentScroll:document.querySelector("#app-content").scrollWidth,contentWidth:document.querySelector("#app-content").clientWidth,viewScroll:document.querySelector(".page-view:not([hidden])").scrollWidth,viewWidth:document.querySelector(".page-view:not([hidden])").clientWidth})')
                assert metrics['padding']==padding,(route,metrics)
                if metrics['contentScroll']>metrics['contentWidth'] or metrics['viewScroll']>metrics['viewWidth']:
                    metrics['overflowElements']=await page.evaluate('[...document.querySelector(".page-view:not([hidden])").querySelectorAll("*")].filter(n=>n.getBoundingClientRect().right>innerWidth+1).slice(0,8).map(n=>({tag:n.tagName,id:n.id,class:n.className,width:n.getBoundingClientRect().width}))')
                layout.append({'route':route,**metrics})
            await navigate('config')
            await page.screenshot(path=str(artifacts/f'config-{width}.png'),full_page=True,animations='disabled')
        overflow=[item for item in layout if item['scroll']>item['width'] or item['contentScroll']>item['contentWidth'] or item['viewScroll']>item['viewWidth']]
        assert not overflow,overflow
        await page.set_viewport_size({'width':390,'height':844})
        await page.locator('#menu-toggle').click();await page.wait_for_function('document.querySelector(".mobile-nav").open')
        await page.screenshot(path=str(artifacts/'drawer-mobile.png'),animations='disabled');await page.keyboard.press('Escape')
        # Discarding a preview by navigation restores saved global settings.
        await select('#theme-mode','light');await page.evaluate('void navigateTo("/about")')
        await page.get_by_text('放弃修改',exact=True).click();await page.wait_for_selector('#about-runtime dd')
        assert await page.evaluate('document.documentElement.classList.contains("mdui-theme-dark")')
        await navigate('config');await page.locator('#background-default').click();await save()
        assert (await api('/api/appearance'))['background_url']=='/static/img/default-background.png'
        assert (await context.request.get(base+uploaded)).status==404
        await page.emulate_media(reduced_motion='reduce');await navigate('about')
        assert await page.evaluate('document.querySelector("#app-content").getAnimations().length')==0
        assert not errors,errors
        (artifacts/'verification.json').write_text(json.dumps({'systemChartColors':True,'restart':True,'previewRollback':True,'sharedLogin':True,'historyPoints':len(history),'layout':layout,'browserErrors':errors},ensure_ascii=False,indent=2))
        await browser.close()
        print('PASS P2.5 SQLite restart/history, live retention, URL/upload/default, dynamic color/modes, raw save, public privacy, preview rollback, 4 viewport gutters/overflow and reduced motion')

def main():
    parser=argparse.ArgumentParser();parser.add_argument('--binary',required=True);parser.add_argument('--seed-helper',required=True);parser.add_argument('--chromium',default=shutil.which('chromium'));parser.add_argument('--artifacts',required=True);args=parser.parse_args()
    root=Path(tempfile.mkdtemp(prefix='shizuku-p25-ui-'));artifacts=Path(args.artifacts).resolve();artifacts.mkdir(parents=True,exist_ok=True)
    (root/'config').mkdir();(root/'webui').symlink_to(Path(__file__).resolve().parents[1]/'webui',target_is_directory=True)
    with socket.socket() as listener:listener.bind(('127.0.0.1',0));port=listener.getsockname()[1]
    settings={'webui':{'port':port,'enabled':True,'password':secrets.token_urlsafe(32)},'bots':[{'id':bid,'name':bid,'app_id':'mock','app_secret':'mock','enabled':False,'mode':'webhook'} for bid in ['alpha','beta']]}
    (root/'config/config.json').write_text(json.dumps(settings));(root/'config/config.json').chmod(0o600)
    subprocess.run([str(Path(args.seed_helper).resolve()),str(root)],check=True)
    state={}
    def start():
        state['log']=(root/'server.log').open('a');state['process']=subprocess.Popen([str(Path(args.binary).resolve())],cwd=root,stdout=state['log'],stderr=state['log'])
        for _ in range(200):
            if state['process'].poll() is not None:raise RuntimeError('server exited')
            try:
                with socket.create_connection(('127.0.0.1',port),timeout=.2):
                    if (root/'data/.admin-initial-password').exists():return
            except OSError:pass
            time.sleep(.05)
        raise RuntimeError('server readiness timeout')
    def stop():state['process'].terminate();state['process'].wait(timeout=15);state['log'].close()
    def restart():stop();start()
    start()
    try:asyncio.run(check('http://127.0.0.1:'+str(port),(root/'data/.admin-initial-password').read_text().strip(),root,artifacts,args.chromium,restart))
    finally:stop()
    print('Actual screenshots:',artifacts);print('Private isolated fixture:',root)
if __name__=='__main__':main()
