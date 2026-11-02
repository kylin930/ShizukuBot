"""Real MDUI rule administration regression. Optional Python Playwright + Chromium.

Reuses P2.1's isolated server fixture: two disabled mock bots, no QQ connection.
Screenshots are taken before synthetic HTTP-error/empty fixtures and contain real
API data. No Node.js tooling or runtime is required by the application.
"""
import asyncio
import importlib.util
import json
import os
from pathlib import Path

from playwright.async_api import async_playwright


async def check(base, password, artifacts, chromium):
    async with async_playwright() as engine:
        proxy = {'server': os.environ['HTTPS_PROXY'], 'bypass': '127.0.0.1,localhost'} if os.environ.get('HTTPS_PROXY') else None
        browser = await engine.chromium.launch(executable_path=chromium, args=['--no-sandbox', '--disable-dev-shm-usage'], proxy=proxy)
        context = await browser.new_context(viewport={'width': 1440, 'height': 1000}, reduced_motion='reduce')
        login = await context.request.post(base + '/login', data={'username': 'admin', 'password': password}, headers={'Content-Type': 'application/json'})
        assert login.status == 200 and (await login.json())['code'] == 0
        page = await context.new_page()
        errors = []
        page.on('pageerror', lambda error: errors.append(error.stack or str(error)))
        async def select(selector, value):
            await page.locator(selector).evaluate('(n,v)=>{n.value=v;n.dispatchEvent(new Event("change",{bubbles:true}));}', value)
        async def fill(selector, value):
            await page.locator(selector).locator('input,textarea').first.fill(value)
        async def value(selector, expected):
            await page.wait_for_function('([s,v])=>document.querySelector(s)?.value===v', arg=[selector, expected])
        async def feedback(text):
            await page.wait_for_function('(text)=>document.querySelector("#rules-feedback")?.textContent.includes(text)', arg=text)
        async def navigate(route):
            assert await page.evaluate('(r)=>navigateTo(r)', route)
        async def saved(bot):
            result = await (await context.request.get(base + '/api/bots/' + bot + '/rules')).json()
            assert result['code'] == 0
            return result['data']

        await page.goto(base + '/#/rules', wait_until='domcontentloaded', timeout=45000)
        await page.wait_for_function('document.querySelector("#rules-new")?.disabled===false')
        assert await page.evaluate('Boolean(customElements.get("mdui-select"))'), 'Actual MDUI did not load'
        assert await page.locator('.desktop-nav [data-route="/rules"]').count() == 1
        assert await page.locator('.mobile-nav [data-route="/rules"]').count() == 1
        # randomUUID is absent on ordinary remote HTTP; getRandomValues still works.
        await page.evaluate("Object.defineProperty(window.crypto,'randomUUID',{value:undefined,configurable:true})")
        await page.locator('#rules-new').click()
        assert await page.locator('#rule-enabled').evaluate('(n)=>n.checked') is False
        await fill('#rule-name', '问候回复')
        await fill('#rule-keyword', '你好')
        await fill('#rule-reply', '你好！{用户ID}')
        await fill('#rule-groups', 'group-a')
        await fill('#rule-test-message', '你好')
        await fill('#rule-test-group', 'group-a')
        await fill('#rule-test-user', 'user-a')
        await page.locator('#rule-test-draft').click()
        await page.wait_for_function('document.querySelector("#rules-test-result").textContent.includes("你好！user-a")')
        assert '草稿' in await page.locator('#rules-test-result').inner_text()
        assert not (await saved('alpha'))['rules'], 'Preview saved production data'
        await page.locator('#rule-save').click()
        await feedback('保持禁用')
        data = await saved('alpha'); rule_id = data['rules'][0]['id']
        assert data['revision'] == 1 and data['rules'][0]['enabled'] is False
        assert '问候回复' in await page.locator('[data-rule-id]').inner_text(), 'Rule name missing from MDUI default slot'
        await page.locator('#rule-toggle').click()
        await feedback('规则已启用')
        assert (await saved('alpha'))['rules'][0]['enabled'] is True
        await fill('#rule-test-message', '你好')
        await fill('#rule-test-group', 'group-a')
        await fill('#rule-test-user', 'user-a')
        await page.locator('#rule-test-saved').click()
        await page.wait_for_function('document.querySelector("#rules-test-result").textContent.includes("已保存规则")')
        assert '你好！user-a' in await page.locator('#rules-test-result').inner_text()
        await page.evaluate("()=>{for(let n=document.querySelector('.rules-page');n;n=n.parentElement)n.scrollTop=0;window.scrollTo(0,0);}")
        await page.screenshot(path=str(artifacts / 'desktop-rules.png'), full_page=True)
        await page.locator('#rules-test-result').scroll_into_view_if_needed()
        await page.screenshot(path=str(artifacts / 'desktop-rule-test.png'), full_page=True)
        await page.set_viewport_size({'width': 390, 'height': 844})
        assert not await page.evaluate('document.documentElement.scrollWidth>innerWidth'), 'Mobile rules overflow'
        await page.evaluate("()=>{for(let n=document.querySelector('.rules-page');n;n=n.parentElement)n.scrollTop=0;window.scrollTo(0,0);}")
        await page.screenshot(path=str(artifacts / 'mobile-rules.png'), full_page=True)
        await page.locator('#rules-test-result').scroll_into_view_if_needed()
        await page.screenshot(path=str(artifacts / 'mobile-rule-test.png'), full_page=True)
        # Real mobile operations, including editing and switching Bot.
        await fill('#rule-name', '手机编辑的问候')
        await fill('#rule-reply', '手机回复')
        await page.locator('#rule-save').click(); await feedback('立即启用')
        assert (await saved('alpha'))['rules'][0]['action']['text'] == '手机回复'
        await page.locator('#rule-toggle').click(); await feedback('规则已禁用')
        await select('#rules-bot-select', 'beta')
        await page.wait_for_function('document.querySelector("#rules-list").textContent.includes("暂无规则")')
        await page.locator('#rules-new').click()
        await fill('#rule-name', 'Beta 菜单')
        await select('#rule-kind', 'exact')
        await fill('#rule-keyword', '菜单 ')
        await select('#rule-source', 'c2c')
        await fill('#rule-reply', 'Beta 回复')
        await page.locator('#rule-save').click(); await feedback('保持禁用')
        assert (await saved('beta'))['rules'][0]['trigger']['value'] == '菜单 ', 'Whitespace was altered'
        assert (await saved('alpha'))['rules'][0]['name'] == '手机编辑的问候'
        await select('#rules-filter', 'enabled')
        await page.wait_for_function('document.querySelector("#rules-list").textContent.includes("没有符合")')
        await select('#rules-filter', 'all')
        await fill('#rules-search', 'missing')
        assert await page.locator('[data-rule-id]').count() == 0
        await fill('#rules-search', '')
        await page.locator('#rule-delete').click()
        await page.get_by_text('取消', exact=True).click()
        assert len((await saved('beta'))['rules']) == 1
        await page.locator('#rule-delete').click()
        await page.get_by_text('确认删除', exact=True).click(); await feedback('规则已删除')
        assert not (await saved('beta'))['rules']
        await select('#rules-bot-select', 'alpha'); await value('#rule-name', '手机编辑的问候')
        await fill('#rule-name', '未保存的修改')
        await select('#rules-bot-select', 'beta')
        await page.get_by_text('继续编辑', exact=True).click()
        await value('#rule-name', '未保存的修改'); await value('#rules-bot-select', 'alpha')
        # Page navigation shares the same dirty guard, without plugin-page state.
        pending = asyncio.create_task(page.evaluate('navigateTo("/plugins")'))
        await page.get_by_text('继续编辑', exact=True).click(); assert await pending is False
        await page.locator('#rule-reset').click()
        await page.get_by_text('放弃修改', exact=True).click(); await value('#rule-name', '手机编辑的问候')
        # Concurrent admin edit through the real API causes a real revision conflict.
        current = await saved('alpha'); current['rules'][0]['name'] = '另一位管理员'
        changed = await (await context.request.put(base + '/api/bots/alpha/rules', data={'revision': current['revision'], 'rules': current['rules']})).json()
        assert changed['code'] == 0
        await fill('#rule-name', '冲突草稿'); await page.locator('#rule-save').click(); await feedback('版本冲突')
        await value('#rule-name', '冲突草稿')
        assert (await saved('alpha'))['rules'][0]['name'] == '另一位管理员'
        await page.locator('#rules-refresh').click(); await page.get_by_text('放弃修改', exact=True).click(); await value('#rule-name', '另一位管理员')
        await page.set_viewport_size({'width': 1440, 'height': 1000})
        # Delayed real reads cannot overwrite a newer Bot selection.
        async def delayed(route):
            response = await route.fetch(); await asyncio.sleep(.35); await route.fulfill(response=response)
        await page.route('**/api/bots/beta/rules', delayed)
        async with page.expect_request(lambda request: request.url.endswith('/api/bots/beta/rules')):
            await select('#rules-bot-select', 'beta')
        await select('#rules-bot-select', 'alpha'); await value('#rule-name', '另一位管理员')
        await asyncio.sleep(.4); await value('#rule-name', '另一位管理员')
        await page.unroute('**/api/bots/beta/rules', delayed)
        # Actual stored hostile strings must remain text in both MDUI and traces.
        hostile = '<img src=x onerror="window.__rulesxss=1">'
        await fill('#rule-name', hostile); await fill('#rule-keyword', hostile); await fill('#rule-reply', hostile)
        await page.locator('#rule-save').click(); await feedback('保持禁用')
        await fill('#rule-test-message', hostile); await fill('#rule-test-group', 'group-a')
        await page.locator('#rule-test-draft').click()
        await page.wait_for_function('document.querySelector("#rules-test-result pre")')
        assert hostile in await page.locator('#rules-test-result').inner_text()
        assert await page.evaluate('!window.__rulesxss && !document.querySelector(".rules-page img,.rules-page script")')
        assert await page.locator('[data-rule-id="' + rule_id + '"]').evaluate('(n)=>!n.shadowRoot.querySelector("img,script")')
        await fill('#rule-name', '问候回复'); await fill('#rule-keyword', '你好'); await fill('#rule-reply', '你好！')
        await page.locator('#rule-save').click(); await feedback('保持禁用')
        await select('#rule-test-source', 'c2c'); await fill('#rule-test-message', '/help')
        await page.locator('#rule-test-draft').click()
        await page.wait_for_function('document.querySelector("#rules-test-result").textContent.includes("内置命令优先")')
        assert not errors, ('before HTTP fixtures', errors[:3])
        # Presentation-only failures, deliberately after screenshots of real API data.
        async def denied_save(route):
            if route.request.method == 'PUT': await route.fulfill(status=500, json={'code': 500, 'message': '保存失败测试'})
            else: await route.continue_()
        await page.route('**/api/bots/alpha/rules', denied_save)
        await fill('#rule-name', '保存失败保留草稿'); await page.locator('#rule-save').click(); await feedback('保存失败测试')
        await value('#rule-name', '保存失败保留草稿')
        assert (await saved('alpha'))['rules'][0]['name'] == '问候回复'
        await page.unroute('**/api/bots/alpha/rules', denied_save)
        await page.locator('#rules-refresh').click(); await page.get_by_text('放弃修改', exact=True).click(); await value('#rule-name', '问候回复')
        async def forbidden(route): await route.fulfill(status=403, body='administrator required')
        await page.route('**/api/bots/alpha/rules', forbidden)
        await page.locator('#rules-refresh').click(); await feedback('权限不足')
        assert await page.locator('#rules-new').evaluate('(n)=>n.disabled')
        await page.unroute('**/api/bots/alpha/rules', forbidden)
        async def no_bots(route): await route.fulfill(json={'code': 0, 'data': []})
        await page.route('**/api/bots', no_bots)
        await page.locator('#rules-refresh').click(); await feedback('尚无机器人')
        assert await page.locator('#rules-new').evaluate('(n)=>n.disabled')
        await page.unroute('**/api/bots', no_bots)
        assert not errors, errors[:3]
        await browser.close()
        print('PASS real MDUI desktop/mobile: create, edit, delete/cancel, enable/disable, Bot isolation, search/filter, preview, dirty guards, concurrent conflict, stale reads, stored XSS text')
        print('PASS separate save-error/auth/empty presentation fixtures; screenshots use real API data')


if __name__ == '__main__':
    spec = importlib.util.spec_from_file_location('p21_fixture', Path(__file__).with_name('p2-browser-smoke.py'))
    fixture = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(fixture)
    fixture.check = check
    fixture.main()
