"""Built-in switches and native MDUI help tooltips on the isolated P2 fixture."""
import asyncio
import importlib.util
import json
import os
from pathlib import Path
from playwright.async_api import async_playwright

async def check(base,password,artifacts,chromium):
    async with async_playwright() as engine:
        proxy={'server':os.environ['HTTPS_PROXY'],'bypass':'127.0.0.1,localhost'} if os.environ.get('HTTPS_PROXY') else None
        browser=await engine.chromium.launch(executable_path=chromium,args=['--no-sandbox','--disable-dev-shm-usage'],proxy=proxy)
        context=await browser.new_context(viewport={'width':1440,'height':900})
        login=await context.request.post(base+'/login',data={'username':'admin','password':password},headers={'Content-Type':'application/json'})
        assert (await login.json())['code']==0
        page=await context.new_page();errors=[];page.on('pageerror',lambda e:errors.append(page.url+' '+(e.stack or str(e))))
        await page.goto(base+'/#/config',wait_until='domcontentloaded');await page.wait_for_selector('#theme-mode')
        for selector in ['#cmd-help-enabled','#cmd-status-enabled','#cmd-shizuku-in-help']:
            assert await page.locator(selector).evaluate('(n)=>n.checked')
        async def set_switch(selector,value):
            await page.locator(selector).evaluate('(n,v)=>{n.checked=v;n.dispatchEvent(new Event("change",{bubbles:true}))}',value)
        await set_switch('#cmd-help-enabled',False);await set_switch('#cmd-status-enabled',False);await set_switch('#cmd-shizuku-in-help',False)
        await page.locator('#config-save').click();await page.wait_for_function('!pageScope.busy && configData.builtin_commands.help_enabled===false')
        response=await context.request.get(base+'/api/config');saved=(await response.json())['data']['builtin_commands']
        assert saved['help_enabled'] is False and saved['status_enabled'] is False and saved['show_shizuku_in_help'] is False
        await page.reload(wait_until='domcontentloaded');await page.wait_for_selector('#theme-mode')
        assert await page.locator('#cmd-status-enabled').evaluate('(n)=>n.checked') is False
        hint=page.locator('#builtin-help-hint mdui-tooltip');trigger=hint.locator('mdui-button')
        await trigger.locator('button').focus()
        await page.wait_for_function('document.querySelector("#builtin-help-hint mdui-tooltip").open')
        await hint.locator('[part="popup"]').wait_for(state='visible')
        assert '#shizuku' in await hint.locator('[part="content"]').inner_text()
        await page.keyboard.press('Escape');await hint.locator('[part="popup"]').wait_for(state='hidden')
        await trigger.hover();await hint.locator('[part="popup"]').wait_for(state='visible')
        await page.locator('.builtin-controls-card').evaluate('(n)=>n.scrollIntoView({block:"start"})')
        await page.screenshot(path=str(artifacts/'builtin-controls-desktop.png'),animations='disabled')
        mobile_context=await browser.new_context(viewport={'width':390,'height':844},is_mobile=True,has_touch=True,storage_state=await context.storage_state())
        mobile=await mobile_context.new_page();mobile.on('pageerror',lambda e:errors.append('mobile '+(e.stack or str(e))))
        await mobile.goto(base+'/#/config',wait_until='domcontentloaded');await mobile.wait_for_selector('#theme-mode')
        mobile_hint=mobile.locator('#builtin-help-hint mdui-tooltip')
        await mobile_hint.locator('mdui-button').tap();await mobile_hint.locator('[part="popup"]').wait_for(state='visible')
        rect=await mobile_hint.locator('[part="popup"]').bounding_box();assert rect and rect['x']>=0 and rect['x']+rect['width']<=390,rect
        await mobile.screenshot(path=str(artifacts/'builtin-controls-mobile.png'),animations='disabled')
        await mobile.locator('mdui-top-app-bar-title').tap();await mobile_hint.locator('[part="popup"]').wait_for(state='hidden')
        await mobile_context.close()
        await trigger.hover();await hint.locator('[part=popup]').wait_for(state='visible')
        # Leave with an open native tooltip; repeated route teardown must be safe.
        await page.evaluate('navigateTo("/rules")');await page.wait_for_selector('#rules-new')
        assert await page.locator('details.ui-help').count()==0
        for route in ['plugins','rules','community','rules','sandbox']:
            await page.evaluate('(r)=>navigateTo("/"+r)',route)
            tip=page.locator('mdui-tooltip.ui-help').first;await tip.wait_for();await tip.locator('mdui-button button').focus()
            await tip.locator('[part="popup"]').wait_for(state='visible')
        await page.evaluate('navigateTo("/plugins")');await page.wait_for_selector('#plugin-save')
        await page.locator('#plugin-details mdui-tooltip mdui-button button').first.focus()
        await page.locator('#plugin-details mdui-tooltip [part=popup]').first.wait_for(state='visible')
        await page.locator('[data-plugin-id="hello"]').click()
        await page.wait_for_function('document.querySelector("#plugin-details h2")?.textContent.includes("Ping")')
        await page.evaluate('navigateTo("/about")');await page.wait_for_selector('#about-runtime dd')
        # UI-owned hint content uses text properties, so hostile text cannot become markup.
        assert await page.evaluate('''async()=>{
            window.__hintXss=0;const hint=AdminUI.help('安全提示',['<img src=x onerror="window.__hintXss=1">']);
            document.querySelector('.page-view').append(hint);await hint.updateComplete;
            return !hint.querySelector('img,script')&&!hint.shadowRoot.querySelector('img,script')&&!window.__hintXss;
        }''')
        await page.evaluate('navigateTo("/config")');await page.wait_for_selector('#theme-mode')
        await set_switch('#cmd-help-enabled',True);await page.evaluate('void navigateTo("/about")')
        await page.get_by_text('继续编辑',exact=True).click();assert await page.evaluate('currentRoute')=='/config'
        await page.set_viewport_size({'width':360,'height':800});assert await page.evaluate('document.scrollingElement.scrollWidth<=innerWidth && document.querySelector("#app-content").scrollWidth<=document.querySelector("#app-content").clientWidth')
        await page.emulate_media(reduced_motion='reduce')
        assert await page.locator('#builtin-help-hint mdui-tooltip').evaluate('(n)=>getComputedStyle(n).getPropertyValue("--mdui-motion-duration-short4").trim()')=='0ms'
        assert not errors,errors
        (artifacts/'verification.json').write_text(json.dumps({'saved':saved,'focusEscapeHoverTouch':True,'routeTeardown':True,'browserErrors':errors},ensure_ascii=False,indent=2))
        await browser.close();print('PASS independent builtin switches, persisted config, native tooltip focus/Esc/hover/touch, navigation, draft guard, 360px layout and XSS')

if __name__=='__main__':
    spec=importlib.util.spec_from_file_location('fixture',Path(__file__).with_name('p2-browser-smoke.py'));fixture=importlib.util.module_from_spec(spec);spec.loader.exec_module(fixture)
    fixture.check=check;fixture.main()
