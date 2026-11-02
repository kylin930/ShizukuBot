"""QQ diagnostics on the real MDUI Bot page; no QQ network access or credentials."""
import asyncio
import importlib.util
import os
from pathlib import Path
from playwright.async_api import async_playwright

async def check(base, password, artifacts, chromium):
    async with async_playwright() as engine:
        proxy = {'server': os.environ['HTTPS_PROXY'], 'bypass': '127.0.0.1,localhost'} if os.environ.get('HTTPS_PROXY') else None
        browser = await engine.chromium.launch(executable_path=chromium, args=['--no-sandbox', '--disable-dev-shm-usage'], proxy=proxy)
        context = await browser.new_context(viewport={'width': 1440, 'height': 1000})
        login = await context.request.post(base+'/login', data={'username':'admin','password':password}, headers={'Content-Type':'application/json'})
        assert (await login.json())['code'] == 0
        page = await context.new_page()
        errors = []
        page.on('pageerror', lambda e: errors.append(str(e)))
        await page.goto(base+'/#/bots', wait_until='domcontentloaded')
        await page.wait_for_selector('.qq-diagnostics')
        assert await page.evaluate('Boolean(customElements.get("mdui-card"))')
        await page.locator('.qq-diagnostics summary').first.click()
        text = await page.locator('.qq-diagnostics').first.inner_text()
        assert '尚未验证' in text and 'QQ 客户端送达需人工验收' in text
        assert '本地沙盒回复成功' not in text
        await page.screenshot(path=str(artifacts/'qq-diagnostics-desktop.png'), full_page=True)
        await page.set_viewport_size({'width':390,'height':844})
        await page.screenshot(path=str(artifacts/'qq-diagnostics-mobile.png'), full_page=True)
        assert not errors, errors
        # Presentation fixture only: error metadata is escaped, never HTML.
        result = await page.evaluate('''() => {
            window.__qqXSS = 0;
            const node = document.createElement('div');
            node.innerHTML = renderQQDiagnostics({id:'<img src=x onerror="window.__qqXSS=1">', diagnostics:{last_send_failure:'<script>window.__qqXSS=1</script>'}});
            document.body.append(node);
            const safe = !node.querySelector('script,img') && !window.__qqXSS;
            node.remove(); return safe;
        }''')
        assert result
        await browser.close()
        print('PASS real MDUI QQ diagnostics desktop/mobile, unverified states, client-delivery distinction; separate escaping fixture')

if __name__ == '__main__':
    spec = importlib.util.spec_from_file_location('p21_fixture', Path(__file__).with_name('p2-browser-smoke.py'))
    fixture = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(fixture)
    fixture.check = check
    fixture.main()
