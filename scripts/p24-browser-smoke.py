"""P2.4 real-browser comparison and lifecycle/motion checks.

Reuses the isolated P2 fixture. Stored local test rules and sandbox replies are
real API data; presentation fixtures are run separately after screenshots.
P24_PHASE=before captures the unmodified baseline, after also runs assertions.
"""
import asyncio
import importlib.util
import json
import os
from pathlib import Path
from playwright.async_api import async_playwright

PHASE = os.environ.get('P24_PHASE', 'after')

async def check(base, password, artifacts, chromium):
    async with async_playwright() as engine:
        proxy = {'server': os.environ['HTTPS_PROXY'], 'bypass': '127.0.0.1,localhost'} if os.environ.get('HTTPS_PROXY') else None
        browser = await engine.chromium.launch(executable_path=chromium, args=['--no-sandbox', '--disable-dev-shm-usage'], proxy=proxy)
        context = await browser.new_context(viewport={'width': 1440, 'height': 900}, reduced_motion='no-preference')
        response = await context.request.post(base+'/login', data={'username':'admin','password':password}, headers={'Content-Type':'application/json'})
        assert (await response.json())['code'] == 0
        rules = [{'id':'hello-rule','name':'问候回复','enabled':True,'priority':100,'trigger':{'type':'contains','value':'你好'},'conditions':{'sources':['group','c2c'],'group_ids':[]},'action':{'type':'reply_text','text':'你好！'},'stop_on_match':True},
                 {'id':'ping-rule','name':'测试回复','enabled':False,'priority':50,'trigger':{'type':'exact','value':'测试'},'conditions':{'sources':['c2c'],'group_ids':[]},'action':{'type':'reply_text','text':'收到'},'stop_on_match':True}]
        saved = await context.request.put(base+'/api/bots/alpha/rules', data={'revision':0,'rules':rules}, headers={'Content-Type':'application/json'})
        assert (await saved.json())['code'] == 0
        page = await context.new_page()
        errors = []
        page.on('pageerror', lambda e: errors.append(page.url + '\n' + (e.stack or str(e))))
        await page.goto(base+'/#/stats', wait_until='domcontentloaded', timeout=45000)
        await page.wait_for_function('Boolean(customElements.get("mdui-select"))')
        async def background_ready(target):
            result = await target.evaluate("""async () => {
                const image=document.querySelector('.page-background'); await image.decode();
                return {url:image.currentSrc,width:image.naturalWidth,height:image.naturalHeight,referrerPolicy:image.referrerPolicy};
            }""")
            assert result['width']==1920 and result['height']==1080
            return result
        if PHASE=='after':
            (artifacts/'background.json').write_text(json.dumps(await background_ready(page),indent=2))
        async def ready(route):
            selector={'stats':'#stat-sent','bots':'#bots-manage-list [data-action]','plugins':'#plugin-save','rules':'#rule-save','sandbox':'#sandbox-send','config':'#config-textarea','about':'.about-title'}[route]
            await page.wait_for_selector(selector)
            if route=='stats': await page.wait_for_function('document.querySelector("#stat-sent")?.textContent!=="-" && Boolean(window.Chart?.getChart(document.querySelector("#hourly-chart")))')
            if route=='sandbox': await page.wait_for_function('document.querySelector("#sandbox-send")?.disabled===false')
            if route=='config': await page.wait_for_function('document.querySelector("#config-textarea")?.value.includes("alpha")')
            if PHASE=='before': await page.wait_for_function('document.querySelector("#app-content").classList.contains("fade-in")')
            await page.evaluate('document.fonts.ready')
        async def navigate(route):
            assert await page.evaluate('(r)=>navigateTo("/"+r)',route)
            await ready(route)
        for route in ['stats','bots','plugins','rules','sandbox','config','about']:
            if route!='stats': await navigate(route)
            else: await ready(route)
            if route=='sandbox':
                await page.locator('#sandbox-input').locator('textarea,input').first.fill('你好')
                await page.locator('#sandbox-send').click()
                await page.wait_for_selector('[data-reply-kind="text"]')
            await page.screenshot(path=str(artifacts/(route+'-desktop.png')), full_page=True, animations='disabled')
        await navigate('bots')
        await page.locator('.qq-diagnostics summary').first.click()
        await page.screenshot(path=str(artifacts/'qq-diagnostics-desktop.png'), full_page=True, animations='disabled')
        await page.set_viewport_size({'width':390,'height':844})
        await page.locator('#menu-toggle').click()
        await page.wait_for_function('document.querySelector(".mobile-nav").open')
        await page.screenshot(path=str(artifacts/'navigation-mobile.png'), animations='disabled')
        await page.keyboard.press('Escape')
        await page.set_viewport_size({'width':360,'height':800})
        await navigate('rules')
        await page.screenshot(path=str(artifacts/'rules-mobile-360.png'), full_page=True, animations='disabled')
        token_names=['--mdui-motion-easing-standard','--mdui-motion-easing-emphasized','--mdui-motion-easing-emphasized-decelerate','--mdui-motion-duration-short3','--mdui-motion-duration-medium1','--mdui-motion-duration-medium2']
        tokens=await page.evaluate('(names)=>Object.fromEntries(names.map(n=>[n,getComputedStyle(document.documentElement).getPropertyValue(n).trim()]))',token_names)
        (artifacts/'tokens.json').write_text(json.dumps(tokens,ensure_ascii=False,indent=2))
        if PHASE=='after': await verify(page, context, base, navigate, errors, artifacts)
        assert not errors, errors
        login_context=await browser.new_context(viewport={'width':390,'height':844})
        login_page=await login_context.new_page();await login_page.goto(base+'/login',wait_until='domcontentloaded')
        await login_page.wait_for_function('Boolean(customElements.get("mdui-button"))')
        if PHASE=='after': await background_ready(login_page)
        await login_page.screenshot(path=str(artifacts/'login-mobile.png'),full_page=True,animations='disabled')
        if PHASE=='after': await verify_login(login_page)
        await browser.close()
        print('PASS P2.4 '+PHASE+' actual UI screenshots (local stored data; no QQ connection)')

async def verify(page, context, base, navigate, errors, artifacts):
    metrics = {}
    await page.set_viewport_size({'width':1440,'height':900})
    # Synchronous entry, finite live animation, and cancellation are observed in
    # the browser, not inferred from screenshots or fixed-duration sleeps.
    metrics['entry'] = await page.evaluate('''async () => {
        await navigateTo('/community');
        const root = document.querySelector('#app-content');
        const first = root.getAnimations()[0];
        const result = {immediate:root.textContent.includes('社区插件'), count:root.getAnimations().length,
            duration:first?.effect.getTiming().duration, opacity:Number(getComputedStyle(root).opacity)};
        await navigateTo('/about'); result.oldCanceled = !first || first.playState === 'idle';
        return result;
    }''')
    assert metrics['entry']['immediate'] and metrics['entry']['count']==1
    assert 180 <= metrics['entry']['duration'] <= 280 and metrics['entry']['opacity'] >= .7
    assert metrics['entry']['oldCanceled']
    fallback = await page.evaluate('''async () => {
        const root=document.querySelector('#app-content'); root.animate=undefined;
        await navigateTo('/community');
        const valid=root.textContent.includes('旧版插件暂不支持') && root.getAnimations().length===0;
        delete root.animate; return valid;
    }''')
    assert fallback, 'Static fallback hid or delayed content'
    # Navigate away from a real delayed stats response; no old chart/timer is installed.
    started, release, complete = asyncio.Event(), asyncio.Event(), asyncio.Event()
    async def delayed_stats(route):
        response = await route.fetch(); started.set(); await release.wait()
        try: await route.fulfill(response=response)
        except Exception: pass  # A canceled browser request is the expected path.
        finally: complete.set()
    await page.route('**/api/stats', delayed_stats)
    await page.evaluate('navigateTo("/stats")')
    await asyncio.wait_for(started.wait(),5)
    await navigate('bots'); release.set(); await asyncio.wait_for(complete.wait(),5)
    assert await page.evaluate('currentRoute==="/bots" && statsRefreshTimer===null && historyChart===null && hourlyChart===null')
    await page.unroute('**/api/stats', delayed_stats)
    await page.evaluate('''async () => {for(let cycle=0;cycle<3;cycle++) for(const route of ['/stats','/plugins','/config','/logs','/rules','/about','/community']) await navigateTo(route);}''')
    await page.wait_for_function('currentRoute==="/community" && document.querySelector("#app-content").textContent.includes("社区插件")')
    assert await page.evaluate('statsRefreshTimer===null && logWebSocket===null')
    await page.wait_for_function('document.querySelectorAll(".page-view").length===1')
    metrics['rapidRoutes']=21
    metrics['retainedViews']=await page.locator('.page-view').count()
    await navigate('about'); await page.evaluate('history.back()')
    await page.wait_for_function('currentRoute==="/community" && location.hash==="#/community"')
    assert await page.locator('.desktop-nav').evaluate('(n)=>n.value') == 'community'
    # Selection updates the existing item rather than replacing the entire list.
    await navigate('plugins')
    item=await page.locator('[data-plugin-id="autoreply"]').element_handle()
    await page.locator('[data-plugin-id="hello"]').click()
    assert await item.evaluate('(n)=>n.isConnected')
    assert await page.locator('[data-plugin-id="hello"]').evaluate('(n)=>n.active')
    await page.locator('[data-plugin-id="autoreply"]').click()
    async def field(selector, value): await page.locator(selector).locator('input,textarea').first.fill(value)
    await field('mdui-text-field[data-schema-path=\'$["keyword"]\']', 'p24-key')
    writes=[]
    page.on('request',lambda req:writes.append(req.url) if req.method=='PUT' and req.url.endswith('/api/plugins/autoreply') else None)
    write_started, write_release = asyncio.Event(), asyncio.Event()
    async def pending_write(route):
        if route.request.method != 'PUT': await route.continue_(); return
        response = await route.fetch(); write_started.set(); await write_release.wait()
        await route.fulfill(response=response)
    await page.route('**/api/plugins/autoreply', pending_write)
    await page.locator('#plugin-save').click()
    await asyncio.wait_for(write_started.wait(),5)
    assert await page.locator('#plugin-save').evaluate('(n)=>n.loading && n.disabled')
    assert await page.locator('#plugin-feedback').inner_text()==''
    await page.evaluate('document.querySelector("#plugin-save")?.click()')
    write_release.set()
    await page.wait_for_function('[...document.querySelectorAll("mdui-snackbar[open]")].some(n=>n.textContent.includes("已保存"))')
    assert len(writes)==1, 'An in-flight click caused another save'
    await page.unroute('**/api/plugins/autoreply', pending_write)
    assert await page.locator('#plugin-feedback').inner_text()==''
    assert await page.locator('mdui-snackbar[open]').count()==1
    saved=await (await context.request.get(base+'/api/plugins/autoreply?bot_id=alpha')).json()
    assert saved['data']['config']['keyword']=='p24-key'
    # High-frequency rule selection, draft guard and native dialog focus/escape.
    await navigate('rules')
    first=await page.locator('[data-rule-id="hello-rule"]').element_handle()
    await page.locator('[data-rule-id="ping-rule"]').click()
    assert await first.evaluate('(n)=>n.isConnected')
    await field('#rule-name','未保存修改')
    await page.evaluate('void navigateTo("/community")')
    await page.get_by_text('继续编辑',exact=True).click()
    assert await page.locator('#rule-name').evaluate('(n)=>n.value')=='未保存修改'
    assert await page.evaluate('currentRoute')=='/rules'
    await page.locator('#rule-reset').click(); await page.get_by_text('放弃修改',exact=True).click()
    await page.locator('#rule-delete').click()
    await page.wait_for_selector('mdui-dialog[open]')
    await page.wait_for_function('document.activeElement?.tagName==="MDUI-DIALOG" || Boolean(document.activeElement?.closest("mdui-dialog"))')
    await page.evaluate('window.__dialogClosed = new Promise(resolve => document.querySelector("mdui-dialog[open]").addEventListener("closed",()=>resolve(true),{once:true})); void 0')
    await page.keyboard.press('Escape')
    await page.evaluate('window.__dialogClosed')
    assert await page.evaluate('document.activeElement?.id==="rule-delete"'), 'Dialog did not restore trigger focus'
    assert len((await (await context.request.get(base+'/api/bots/alpha/rules')).json())['data']['rules'])==2
    # Rule preview produces actual local traces and a finite local entry effect.
    await field('#rule-test-message','测试')
    await page.locator('#rule-test-source').evaluate('(n)=>{n.value="c2c";n.dispatchEvent(new Event("change",{bubbles:true}))}')
    await page.locator('#rule-test-draft').click()
    await page.wait_for_selector('#rules-test-result [data-trace-rule]')
    assert '收到' in await page.locator('#rules-test-result').inner_text()
    # New sandbox turns preserve prior DOM and actual captured replies.
    await navigate('sandbox')
    await page.locator('#sandbox-plugin-select').evaluate('(n)=>{n.value="hello";n.dispatchEvent(new Event("change",{bubbles:true}))}')
    await field('#sandbox-input','/ping'); await page.locator('#sandbox-send').click()
    await page.wait_for_selector('[data-reply-kind="text"]')
    old_turn=await page.locator('.sandbox-turn').first.element_handle()
    await field('#sandbox-input','/ping'); await page.locator('#sandbox-send').click()
    await page.wait_for_function('document.querySelectorAll("[data-reply-kind=text]").length===2')
    assert await old_turn.evaluate('(n)=>n.isConnected')
    # Config validation and discard guard; credentials are write-only.
    await navigate('config')
    assert await page.locator('#webui-password').evaluate('(n)=>n.value')==''
    assert await page.locator('.bot-config-appsecret').first.evaluate('(n)=>n.value')==''
    old_port=await page.locator('#webui-port').evaluate('(n)=>n.value')
    await field('#webui-port','70000'); await page.locator('#config-save').click()
    assert '1–65535' in await page.locator('#config-port-error').inner_text()
    await field('#webui-port',str(old_port))
    await field('#market-api','https://example.invalid/local-draft')
    await page.evaluate('void navigateTo("/about")'); await page.get_by_text('继续编辑',exact=True).click()
    assert await page.locator('#market-api').evaluate('(n)=>n.value')=='https://example.invalid/local-draft'
    await page.locator('#config-reload').click(); await page.get_by_text('放弃修改',exact=True).click()
    await page.wait_for_function('document.querySelector("#market-api").value!=="https://example.invalid/local-draft"')
    # Failed Bot start is a separate HTTP presentation fixture, never real QQ.
    await navigate('bots')
    async def rejected_start(route): await route.fulfill(json={'code':403,'message':'测试：凭证无效'})
    await page.route('**/api/bots/alpha/start', rejected_start)
    await page.locator('[data-action="start"][data-id="alpha"]').click()
    await page.wait_for_function('[...document.querySelectorAll("mdui-snackbar[open]")].some(n=>n.textContent.includes("凭证无效"))')
    assert '已停止' in await page.locator('[data-bot-id="alpha"]').inner_text()
    await page.unroute('**/api/bots/alpha/start', rejected_start)
    # Reduced motion applies to page/local motion, buttons, list state and cards.
    await page.emulate_media(reduced_motion='reduce')
    await navigate('plugins')
    await page.wait_for_function('matchMedia("(prefers-reduced-motion: reduce)").matches')
    metrics['reduced']=await page.evaluate('''() => ({page:document.querySelector('#app-content').getAnimations().length,
        detail:document.querySelector('#plugin-details').getAnimations().length,
        button:getComputedStyle(document.querySelector('#plugin-save')).transitionDuration,
        list:getComputedStyle(document.querySelector('#plugins-list mdui-list-item')).transitionDuration,
        card:getComputedStyle(document.querySelector('#plugin-details')).transitionDuration,
        foreground:getComputedStyle(document.querySelector('#plugin-details')).opacity})''')
    assert metrics['reduced']=={'page':0,'detail':0,'button':'0s','list':'0s','card':'0s','foreground':'1'}, metrics
    # Every page remains usable without horizontal scrolling at both phone widths.
    for width,height in [(390,844),(360,800)]:
        await page.set_viewport_size({'width':width,'height':height})
        for route in ['stats','bots','plugins','rules','sandbox','config','about']:
            await navigate(route)
            assert await page.evaluate('document.scrollingElement.scrollWidth <= innerWidth'), (width,route)
    # Separate hostile-text fixture; screenshots above contain only real API data.
    assert await page.evaluate('''async () => {
        window.__p24xss=0;showSnackbar('<img src=x onerror="window.__p24xss=1">');
        const message=document.querySelector('mdui-snackbar'); await message.updateComplete;
        return message.textContent.includes('<img')&&!message.querySelector('img,script')&&!window.__p24xss;
    }''')
    (artifacts/'browser-errors.json').write_text(json.dumps(errors,ensure_ascii=False,indent=2))
    assert not errors, errors[:3]
    (artifacts/'verification.json').write_text(json.dumps(metrics,ensure_ascii=False,indent=2))
    print('PASS immediate/canceled entry motion, static fallback, late-request cleanup, history, native dialogs, single saves/toasts, real rules/sandbox, 360/390 overflow, reduced motion and XSS')

async def verify_login(page):
    errors=[];page.on('pageerror',lambda e:errors.append(str(e)))
    await page.locator('#login-submit').click()
    assert '请输入用户名和密码' in await page.locator('#error-msg').inner_text()
    count=[]
    async def login_error(route):
        count.append(1);await route.fulfill(json={'code':401,'message':'用户名或密码错误'})
    await page.route('**/login',login_error)
    await page.locator('#password').locator('input').fill('fake-local-password')
    await page.locator('#login-submit').click()
    await page.wait_for_function('!document.querySelector("#login-submit").loading')
    assert '用户名或密码错误' in await page.locator('#error-msg').inner_text()
    assert len(count)==1 and not errors
    assert await page.evaluate('document.scrollingElement.scrollWidth <= innerWidth')

if __name__=='__main__':
    spec=importlib.util.spec_from_file_location('p21_fixture',Path(__file__).with_name('p2-browser-smoke.py'))
    fixture=importlib.util.module_from_spec(spec);spec.loader.exec_module(fixture)
    fixture.check=check;fixture.main()
