"""Optional real Chromium regression checks; not a backend/build dependency.

Requires Python Playwright and an installed Chromium. It runs an isolated copy of
runtime data with disabled mock bots. Screenshots show the actual application;
synthetic schema/security/failed-response fixtures are tested separately.
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


def options():
    parser = argparse.ArgumentParser()
    parser.add_argument('--binary', required=True)
    parser.add_argument('--chromium', default=shutil.which('chromium'))
    parser.add_argument('--artifacts', default=None)
    return parser.parse_args()


async def check(base, password, artifacts, chromium):
    async with async_playwright() as engine:
        proxy = {'server': os.environ['HTTPS_PROXY'], 'bypass': '127.0.0.1,localhost'} if os.environ.get('HTTPS_PROXY') else None
        browser = await engine.chromium.launch(executable_path=chromium, args=['--no-sandbox', '--disable-dev-shm-usage'], proxy=proxy)
        context = await browser.new_context(viewport={'width': 1440, 'height': 1000}, reduced_motion='reduce')
        login = await context.request.post(base + '/login', data={'username': 'admin', 'password': password}, headers={'Content-Type': 'application/json'})
        assert login.status == 200 and (await login.json())['code'] == 0
        page = await context.new_page()
        page_errors = []
        page.on('pageerror', lambda error: page_errors.append(error.stack or str(error)))

        async def select(selector, value):
            await page.locator(selector).evaluate('(node,value)=>{node.value=value;node.dispatchEvent(new Event("change",{bubbles:true}));}', value)

        def field(key):
            return page.locator('mdui-text-field[data-schema-path=' + json.dumps('$[' + json.dumps(key) + ']') + ']').locator('input')

        async def wait_value(key, value):
            await page.wait_for_function('([key,value])=>[...document.querySelectorAll("mdui-text-field[data-schema-path]")].some(n=>n.getAttribute("data-schema-path")==="$["+JSON.stringify(key)+"]"&&n.value===value)', arg=[key, value])

        async def navigate(route):
            assert await page.evaluate('(route)=>navigateTo(route)', route)

        await page.goto(base + '/#/plugins', wait_until='domcontentloaded', timeout=45000)
        await page.wait_for_selector('#plugin-save')
        assert await page.evaluate('Boolean(customElements.get("mdui-select"))'), 'Actual MDUI components did not load'
        assert await page.locator('#plugins-list [data-plugin-id]').count() == 4
        await field('keyword').fill('alpha-key')
        await field('reply').fill('alpha-reply')
        await page.locator('#plugin-json-mode').click()
        assert json.loads(await page.locator('.plugin-json').input_value())['keyword'] == 'alpha-key'
        await page.locator('#plugin-form-mode').click()
        await wait_value('keyword', 'alpha-key')
        await page.locator('#plugin-save').click()
        await page.wait_for_function('document.querySelector("#plugin-feedback").textContent.includes("配置已保存")')
        await select('#plugin-bot-select', 'beta')
        await wait_value('keyword', '你好')
        await field('keyword').fill('beta-key')
        await page.locator('#plugin-save').click()
        await page.wait_for_function('document.querySelector("#plugin-feedback").textContent.includes("配置已保存")')
        data = await (await context.request.get(base + '/api/plugins/autoreply?bot_id=alpha')).json()
        assert data['data']['config']['keyword'] == 'alpha-key', 'Bot config crossed scopes'
        await field('keyword').fill('unsaved')
        await page.locator('[data-plugin-id="hello"]').click()
        await page.get_by_text('继续编辑', exact=True).click()
        await wait_value('keyword', 'unsaved')
        await page.locator('#plugin-reset').click()
        await page.get_by_text('放弃修改', exact=True).click()
        await wait_value('keyword', 'beta-key')
        await page.locator('[data-plugin-id="counter"]').click()
        assert await page.locator('#plugin-toggle').evaluate('(e)=>e.disabled'), 'Selection policy was hidden/ignored'
        await page.locator('[data-plugin-id="autoreply"]').click()

        # Delay a real response, rather than substituting fake configuration.
        async def delayed(route):
            response = await route.fetch()
            await asyncio.sleep(.4)
            await route.fulfill(response=response)
        await page.route('**/api/plugins?bot_id=alpha', delayed)
        async with page.expect_request(lambda request: request.url.endswith('/api/plugins?bot_id=alpha')):
            await select('#plugin-bot-select', 'alpha')
        await select('#plugin-bot-select', 'beta')
        await wait_value('keyword', 'beta-key')
        await asyncio.sleep(.5)
        await wait_value('keyword', 'beta-key')
        await page.unroute('**/api/plugins?bot_id=alpha', delayed)
        await select('#plugin-bot-select', 'alpha')
        await wait_value('keyword', 'alpha-key')
        await page.locator('#plugin-toggle').click()
        await page.wait_for_function('document.querySelector("#plugin-toggle")?.textContent==="禁用插件"')
        await field('keyword').fill('alpha-applied')
        await page.locator('#plugin-save').click()
        await page.wait_for_function('document.querySelector("#plugin-feedback").textContent.includes("已应用新配置")')
        await page.locator('#plugin-toggle').click()
        await page.wait_for_function('document.querySelector("#plugin-toggle")?.textContent==="启用插件"')
        await page.locator('#plugin-toggle').click()
        await page.wait_for_function('document.querySelector("#plugin-toggle")?.textContent==="禁用插件"')
        await page.locator('#plugin-restart').click()
        await page.wait_for_function('document.querySelector("#plugin-feedback").textContent.includes("操作完成")')
        await page.locator('#plugin-search').locator('input').fill('Ping')
        assert await page.locator('#plugins-list [data-plugin-id]').count() == 1
        await page.locator('#plugin-search').locator('input').fill('')
        await page.screenshot(path=str(artifacts / 'desktop-plugins.png'), full_page=True)
        await page.set_viewport_size({'width': 390, 'height': 844})
        assert not await page.evaluate('document.documentElement.scrollWidth>innerWidth'), 'Mobile plugin page overflow'
        await page.locator('#plugin-details').scroll_into_view_if_needed()
        await page.screenshot(path=str(artifacts / 'mobile-plugins.png'), full_page=True)
        await page.set_viewport_size({'width': 1440, 'height': 1000})
        await page.locator('#plugin-sandbox').click()
        await page.wait_for_selector('#sandbox-input')
        assert await page.locator('#sandbox-bot-select').evaluate('(e)=>e.value') == 'bot:alpha'
        assert await page.locator('#sandbox-plugin-select').evaluate('(e)=>e.value') == 'autoreply'
        await select('#sandbox-type', 'group')
        await page.locator('#sandbox-input').locator('textarea').fill('alpha-applied')
        await page.locator('#sandbox-send').click()
        await page.wait_for_selector('[data-outcome="replied"]')
        await select('#sandbox-plugin-select', 'hello')
        await select('#sandbox-type', 'c2c')
        await page.locator('#sandbox-input').locator('textarea').fill('/ping')
        await page.locator('#sandbox-send').click()
        await page.wait_for_function('[...document.querySelectorAll(".sandbox-reply")].some(n=>n.textContent.includes("pong"))')
        hello = await (await context.request.get(base + '/api/plugins/hello?bot_id=alpha')).json()
        assert hello['data']['enabled'] is False, 'Debug enabled a production plugin'
        await page.screenshot(path=str(artifacts / 'desktop-sandbox.png'), full_page=True)
        await page.set_viewport_size({'width': 390, 'height': 844})
        assert not await page.evaluate('document.documentElement.scrollWidth>innerWidth'), 'Mobile sandbox overflow'
        await page.screenshot(path=str(artifacts / 'mobile-sandbox.png'), full_page=True)
        await page.set_viewport_size({'width': 1440, 'height': 1000})
        await select('#sandbox-bot-select', 'independent')
        await page.wait_for_function('!document.querySelector("#sandbox-debug-id").hidden')
        await select('#sandbox-plugin-select', 'counter')
        for _ in range(2):
            await page.locator('#sandbox-input').locator('textarea').fill('/count')
            await page.locator('#sandbox-send').click()
            await page.wait_for_function('!document.querySelector("#sandbox-send").disabled')
        counter_text = await page.locator('[data-reply-kind="text"]').all_text_contents()
        assert sum('计数: 1' in text for text in counter_text) == 2, 'Sandbox state was reused'

        assert not page_errors, ('before synthetic fixtures', page_errors[:3])
        # Synthetic data exercises all schema/reply shapes without pretending they
        # were sent by QQ. Screenshots above contain only actual server responses.
        await page.evaluate('''async () => {
            const settle=async root=>{await Promise.resolve();await Promise.allSettled([...root.querySelectorAll('*')].map(n=>n.updateComplete).filter(Boolean));await new Promise(resolve=>requestAnimationFrame(resolve));};
            const U=PluginUI, assert=(ok,message)=>{if(!ok)throw Error(message)};
            const schema={type:'object',required:['name','n','active','nested','choices'],properties:{name:{type:'string'},n:{type:'integer',minimum:1,maximum:5},active:{type:'boolean'},nested:{type:'object',required:['value'],properties:{value:{type:'string'}}},mode:{type:'string',enum:['a','b']},choices:{type:'array',items:{type:'number',minimum:0}}}};
            const original={name:'name',n:2,active:false,nested:{value:'x'},mode:'a',choices:[1,2]};
            const editor=new U.ConfigEditor(schema,original);document.body.append(editor.element);await settle(editor.element);
            assert(editor.element.querySelectorAll('mdui-switch').length===1,'boolean widget');
            assert(U.Schema.validate(schema,original).length===0,'valid config rejected');
            const add=[...editor.element.querySelectorAll('mdui-button')].find(n=>n.textContent==='添加项目');add.click();await settle(editor.element);assert(editor.getValue().choices.length===3,'array add');
            const remove=[...editor.element.querySelectorAll('mdui-button')].find(n=>n.textContent==='移除项目');remove.click();await settle(editor.element);assert(editor.getValue().choices.length===2,'array remove');
            const optional=editor.element.querySelector('mdui-checkbox');optional.click();await settle(editor.element);assert(!Object.hasOwn(editor.getValue(),'mode'),'optional field removal');
            assert(U.Schema.validate(schema,{...original,n:2.5,mode:'c'}).length===2,'number/enum checks');
            assert(U.Schema.validate(schema,{...original,nested:{}}).some(e=>e.path.includes('value')),'required path');
            const input=[...editor.element.querySelectorAll('mdui-text-field')].find(n=>n.label==='name');input.value='changed';input.dispatchEvent(new Event('input'));await settle(editor.element);
            editor.switchMode('json');await settle(editor.element);assert(JSON.parse(editor.raw).name==='changed','form to JSON');
            editor.raw=JSON.stringify({...editor.getValue(),name:'from-json'});editor.switchMode('form');await settle(editor.element);assert(editor.getValue().name==='from-json'&&original.name==='name','JSON to form or mutation');
            const fallback=new U.ConfigEditor({type:'object'},{unknown:{untouched:1}});assert(fallback.mode==='json'&&JSON.parse(fallback.raw).unknown.untouched===1,'unknown data lost');
            const numeric=[...editor.element.querySelectorAll('mdui-text-field')].find(n=>n.label==='n');numeric.value='2.5';numeric.dispatchEvent(new Event('input'));await settle(editor.element);editor.switchMode('json');assert(editor.mode==='form'&&editor.isDirty(),'invalid number silently discarded');
            const unknown=new U.ConfigEditor({type:'object',properties:{value:{type:'unsupported'}}},{value:{keep:true}});assert(unknown.mode==='json'&&JSON.parse(unknown.raw).value.keep,'unsupported schema lost data');
            await settle(editor.element);editor.element.remove();
            const fixture=document.createElement('div');document.body.append(fixture);window.__p2xss=0;
            for(const kind of ['text','markdown','image','rich_media','keyboard','video']) SandboxPage.appendReply(fixture,{kind,text:'<img src=x onerror="window.__p2xss=1">',url:'javascript:window.__p2xss=1',keyboard:'<script>window.__p2xss=1</script>'});
            fixture.append(U.errorDetails('<script>window.__p2xss=1</script>'));
            const metadata=U.el('mdui-list-item',{headline:'<img src=x onerror=window.__p2xss=1>',description:'<script>window.__p2xss=1</script>'});fixture.append(metadata);await settle(fixture);
            assert(!metadata.shadowRoot?.querySelector('img,script'),'plugin metadata injection');
            assert(fixture.querySelectorAll('[data-reply-kind]').length===6,'structured reply missing');
            assert(!fixture.querySelector('img,script,iframe,a[href^="javascript:"]')&&!window.__p2xss,'HTML/URL injection');
            SandboxPage.appendExecution(fixture,{plugin_id:'legacy',status:'completed'});assert(fixture.textContent.includes('后端未报告匹配情况'),'no-reply incorrectly inferred as unmatched');
            fixture.remove();
        }''')

        await asyncio.sleep(.05)
        assert not page_errors, ('after synthetic fixtures', page_errors[:3])
        # Presentation-only response fixture for a restart failure. The Go API
        # tests separately cause a real extension initialization failure.
        async def restart_failure(route):
            response = await route.fetch()
            data = await response.json()
            data['data'].update(saved=True, applied=False, restart_error='browser restart failure fixture')
            await route.fulfill(response=response, json=data)
        await navigate('/plugins')
        await page.wait_for_selector('#plugin-save')
        await page.route('**/api/plugins/autoreply', restart_failure)
        await field('keyword').fill('feedback-check')
        await page.locator('#plugin-save').click()
        await page.wait_for_function('document.querySelector("#plugin-feedback").textContent.includes("配置已保存，但重启插件实例失败")')
        await page.unroute('**/api/plugins/autoreply', restart_failure)

        # Validate serialization of an enabled Bot without starting its connection.
        await navigate('/config')
        await page.wait_for_selector('.bot-config-item')
        await page.evaluate('configData.bots[0].enabled=true')
        async with page.expect_request(lambda request: request.method == 'PUT' and request.url.endswith('/api/config')) as captured:
            await page.locator('#config-save').click()
        body = (await captured.value).post_data_json
        assert body['bots'][0]['enabled'] is True, 'Saving global config forced enabled=false'
        assert body['bots'][0]['mode'] == 'webhook', 'Saving config lost bot properties'
        # Empty and authorization states are separate presentation fixtures.
        async def no_bots(route):
            await route.fulfill(json={'code': 0, 'data': []})
        await page.route('**/api/bots', no_bots)
        await navigate('/plugins')
        await page.wait_for_selector('#plugin-save')
        assert await page.locator('#plugin-save').evaluate('(e)=>e.disabled'), 'No-bot management was editable'
        assert '尚无机器人' in await page.locator('#plugin-feedback').inner_text()
        await page.unroute('**/api/bots', no_bots)
        async def forbidden(route):
            await route.fulfill(status=403, body='administrator required')
        await page.route('**/api/bots', forbidden)
        await page.locator('#plugin-refresh').click()
        await page.wait_for_function('document.querySelector("#plugin-feedback").textContent.includes("权限不足")')
        await page.unroute('**/api/bots', forbidden)
        assert not page_errors, page_errors[:3]
        await browser.close()
        print('PASS real MDUI desktop/mobile, Bot/config isolation, form/JSON round trips, unsaved guard, stale responses, instance operations, sandbox and 6 structured reply types')
        print('PASS synthetic schema/validation/XSS/failure-feedback and enabled-serialization fixtures; screenshots contain actual application data')


def main():
    args = options()
    if not args.chromium:
        raise SystemExit('Chromium unavailable; browser checks not run')
    root = Path(tempfile.mkdtemp(prefix='shizuku-p2-ui-'))
    artifacts = Path(args.artifacts).resolve() if args.artifacts else root / 'screenshots'
    artifacts.mkdir(parents=True, exist_ok=True)
    (root / 'config').mkdir()
    (root / 'webui').symlink_to(Path(__file__).resolve().parents[1] / 'webui', target_is_directory=True)
    with socket.socket() as listener:
        listener.bind(('127.0.0.1', 0))
        port = listener.getsockname()[1]
    settings = {'webui': {'port': port, 'enabled': True, 'password': secrets.token_urlsafe(32)}, 'bots': [
        {'id': 'alpha', 'name': 'Alpha 测试机器人', 'app_id': 'mock', 'app_secret': 'mock', 'enabled': False, 'mode': 'webhook'},
        {'id': 'beta', 'name': 'Beta 测试机器人', 'app_id': 'mock', 'app_secret': 'mock', 'enabled': False, 'mode': 'webhook', 'plugins': ['hello', 'autoreply']},
    ]}
    config = root / 'config/config.json'
    config.write_text(json.dumps(settings)); config.chmod(0o600)
    with (root / 'server.log').open('w') as log:
        server = subprocess.Popen([str(Path(args.binary).resolve())], cwd=root, stdout=log, stderr=log)
        try:
            for _ in range(200):
                if server.poll() is not None:
                    raise RuntimeError('Server exited before readiness')
                try:
                    with socket.create_connection(('127.0.0.1', port), timeout=.2):
                        if (root / 'data/.admin-initial-password').exists():
                            break
                except OSError:
                    pass
                time.sleep(.05)
            else:
                raise RuntimeError('Server readiness timeout')
            password = (root / 'data/.admin-initial-password').read_text().strip()
            asyncio.run(check('http://127.0.0.1:' + str(port), password, artifacts, args.chromium))
            print('Actual browser screenshots:', artifacts)
        finally:
            server.terminate()
            server.wait(timeout=15)
    print('Private test data retained locally:', root)


if __name__ == '__main__':
    main()
