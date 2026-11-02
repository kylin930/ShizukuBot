// Administration of compiled Go plugins. All scopes come from /api/bots.
(() => {
    'use strict';
    const U = window.PluginUI;
    const stateNames = {running: '正常', unloaded: '未加载', error: '异常'};
    async function renderPlugins(container) {
        const scope = U.begin(container);
        let bots = [], plugins = [], botId = '', selectedId = U.context.pluginId, editor = null, revision = 0, loadError = '';
        const feedback = U.el('div', {id: 'plugin-feedback'});
        const toolbar = U.el('div', {class: 'plugin-toolbar'});
        const body = U.el('div', {class: 'plugin-workspace'});
        const sidebar = U.el('mdui-card', {variant: 'outlined', class: 'plugin-sidebar'});
        const detail = U.el('mdui-card', {variant: 'outlined', class: 'plugin-detail', id: 'plugin-details'});
        const list = U.el('mdui-list', {id: 'plugins-list'});
        const search = U.el('mdui-text-field', {label: '搜索插件名称、ID 或简介', id: 'plugin-search', clearable: true});
        const filter = U.select('状态筛选', [
            {value: 'all', label: '全部'}, {value: 'enabled', label: '已启用'}, {value: 'disabled', label: '已禁用'},
            {value: 'error', label: '异常'}, {value: 'blocked', label: '策略排除'}, {value: 'running', label: '实例已启动'},
        ], 'all', 'plugin-filter');
        const botSelect = U.select('管理机器人', [], '', 'plugin-bot-select');
        sidebar.append(search, filter, list); body.append(sidebar, detail);
        toolbar.append(botSelect, U.button('刷新状态', () => reload(true), 'plugin-refresh'));
        container.replaceChildren(U.el('div', {class: 'plugin-page-surface'}, [U.text('h1', '代码插件', 'page-title'),
            U.text('p', '管理已编译插件的配置和实例。Go 源码变更仍需重新构建和部署。', 'plugin-muted'), toolbar, feedback, body]));
        scope.dirty = () => Boolean(editor?.isDirty());
        scope.on(search, 'input', renderList); scope.on(filter, 'change', renderList);
        scope.on(botSelect, 'change', async () => {
            const next = botSelect.value;
            if (next === botId) return;
            if (!await U.mayLeave()) {botSelect.value = botId; return;}
            botId = next; U.context.botId = next; editor = null;
            await reload(false);
        });
        async function reload(checkDirty) {
            if (checkDirty && !await U.mayLeave()) return;
            const version = ++revision;
            editor = null; plugins = []; list.replaceChildren(); detail.replaceChildren(U.text('p', '正在加载当前机器人的插件状态…'));
            U.notice(feedback, '正在加载插件状态…', 'loading');
            try {
                const nextBots = await scope.request('/api/bots');
                if (!scope.current() || version !== revision) return;
                bots = nextBots.sort((a, b) => a.id.localeCompare(b.id));
                if (!bots.some(bot => bot.id === botId)) botId = bots.some(bot => bot.id === U.context.botId) ? U.context.botId : bots[0]?.id || '';
                botSelect.replaceChildren(...bots.map(bot => U.el('mdui-menu-item', {value: bot.id, text: `${bot.name || bot.id} · ${bot.id}`})));
                botSelect.value = botId; botSelect.disabled = !bots.length; filter.disabled = !botId; if (!botId) filter.value = 'all';
                U.context.botId = botId;
                const data = await scope.request('/api/plugins' + (botId ? '?bot_id=' + encodeURIComponent(botId) : ''));
                if (!scope.current() || version !== revision) return;
                plugins = data;
                U.notice(feedback, botId ? '' : '尚无机器人。可查看已编译插件；请先在机器人页添加真实 Bot 后管理配置和启停。');
                if (!plugins.some(plugin => plugin.id === selectedId)) selectedId = plugins[0]?.id || '';
                editor = null; renderList(); renderDetail(); return true;
            } catch (error) {
                if (!scope.current() || version !== revision) return;
                loadError = U.errorMessage(error); U.notice(feedback, loadError, 'error');
                detail.replaceChildren(U.text('p', '未能加载当前 Bot 的插件数据。请检查权限或重试。')); return false;
            }
        }
        function renderList() {
            const query = search.value.trim().toLocaleLowerCase(), mode = filter.value;
            const visible = plugins.filter(plugin => {
                const searchable = `${plugin.name} ${plugin.id} ${plugin.description || ''}`.toLocaleLowerCase();
                const matches = mode === 'all' || (mode === 'enabled' && plugin.enabled) || (mode === 'disabled' && !plugin.enabled) ||
                    (mode === 'error' && (plugin.state === 'error' || plugin.config_error)) || (mode === 'blocked' && botId && !plugin.allowed) || (mode === 'running' && plugin.running);
                return searchable.includes(query) && matches;
            });
            list.replaceChildren();
            for (const plugin of visible) {
                const status = botId ? `${plugin.enabled ? '已启用' : '已禁用'} · ${stateNames[plugin.state] || '未知'}${plugin.allowed ? '' : ' · 策略排除'}` : '已编译注册';
                const item = U.el('mdui-list-item', {headline: plugin.name, description: `${plugin.id} · ${plugin.version} · ${status}`, active: plugin.id === selectedId, 'data-plugin-id': plugin.id, rounded: true});
                if (plugin.description) item.append(U.text('span', plugin.description, 'plugin-list-description'));
                item.addEventListener('click', async () => {
                    if (plugin.id === selectedId || !await U.mayLeave()) return;
                    selectedId = plugin.id; U.context.pluginId = selectedId; editor = null; renderList(); renderDetail();
                });
                list.append(item);
            }
            if (!visible.length) list.append(U.text('p', plugins.length ? '没有符合筛选条件的插件。' : '此二进制没有注册插件。', 'plugin-empty'));
        }
        function renderDetail() {
            const plugin = plugins.find(p => p.id === selectedId);
            if (!plugin) {detail.replaceChildren(U.text('p', '选择插件查看详情。')); return;}
            U.context.pluginId = plugin.id;
            const actions = U.el('div', {class: 'plugin-actions'});
            const configHost = U.el('section', {class: 'plugin-section'});
            const runtime = U.el('section', {class: 'plugin-section'});
            const overview = U.el('dl', {class: 'plugin-facts'});
            const facts = [
                ['插件 ID', plugin.id], ['版本', plugin.version], ['作者', plugin.author || '未填写'], ['运行时', plugin.runtime],
                ['当前 Bot', botId || '未选择'], ['注册', plugin.compiled ? '已编译' : '未知'], ['生产配置', !botId ? '未选择 Bot' : plugin.enabled ? '已启用' : '已禁用'],
                ['选择策略', !botId ? '未选择 Bot' : plugin.allowed ? '允许此插件' : '排除此插件'],
                ['插件实例', !botId ? '未选择 Bot' : plugin.running ? '已启动' : '未启动'], ['实例状态', !botId ? '未选择 Bot' : stateNames[plugin.state] || plugin.state || '未知'],
                ['Bot 连接', plugin.bot_running ? '运行中' : '未连接'],
            ];
            for (const [name, value] of facts) overview.append(U.text('dt', name), U.text('dd', value));
            editor = botId ? new (U.ConfigEditor || U.JSONEditor)(plugin.config_schema, plugin.config || {}) : null;
            const save = U.button('保存并应用配置', () => mutate('save', plugin), 'plugin-save', 'filled');
            const reset = U.button('重置修改', async () => {if (await U.mayLeave()) {editor = null; renderDetail();}}, 'plugin-reset');
            save.disabled = reset.disabled = !botId;
            configHost.append(U.text('h3', '配置'), editor?.element || U.text('p', '请选择真实机器人后配置。'), U.el('div', {class: 'plugin-actions'}, [save, reset]));
            if (plugin.config_error) configHost.append(U.errorDetails('已保存配置校验失败：' + plugin.config_error));
            const toggle = U.button(plugin.enabled ? '禁用插件' : '启用插件', () => mutate('toggle', plugin), 'plugin-toggle', 'filled');
            toggle.disabled = !botId || (!plugin.enabled && !plugin.allowed);
            const restart = U.button('重启插件实例', () => mutate('restart', plugin), 'plugin-restart');
            restart.disabled = !botId || !plugin.allowed || !plugin.running;
            actions.append(toggle, restart, U.button('进入沙盒测试', () => U.openSandbox(botId, plugin.id), 'plugin-sandbox'));
            runtime.append(U.text('h3', '运行状态'), actions,
                U.text('p', '重启只重新创建当前二进制中的插件实例，不读取修改后的 Go 源码，不会重启 ShizukuBot。', 'plugin-muted'));
            if (plugin.restart_required) runtime.append(U.text('p', '已保存的配置尚未应用到当前实例。', 'plugin-warning'));
            if (plugin.last_error) runtime.append(U.text('h4', '最近一次错误'), U.errorDetails(plugin.last_error));
            if (botId && !plugin.allowed) runtime.append(U.text('p', '此 Bot 的插件选择策略排除了该插件。可先保存配置；启用前请在机器人配置中允许它。', 'plugin-warning'));
            const advanced = U.el('details', {class: 'plugin-section'}, [U.text('summary', '高级信息与数据备份'),
                U.text('p', '权限：' + (plugin.permissions || []).join(', ')), U.text('h4', 'Manifest'),
                U.text('pre', JSON.stringify(Object.fromEntries(['api_version','id','name','version','description','author','runtime','permissions','config_schema'].map(k => [k, plugin[k]])), null, 2)),
                U.text('h4', '原始 Schema'), U.text('pre', JSON.stringify(plugin.config_schema, null, 2)),
                U.text('p', '备份包含插件自己的私有配置和状态。恢复仅支持空数据范围，保持禁用，不覆盖已有数据。'),
                U.button('备份配置与状态', () => backup(plugin.id), 'plugin-backup'),
            ]);
            const file = U.el('input', {type: 'file', accept: '.json,application/json', id: 'plugin-restore-file', 'aria-label': '选择插件数据备份'});
            advanced.append(file, U.button('恢复数据备份', () => restore(file), 'plugin-restore'));
            detail.replaceChildren(U.text('h2', plugin.name), U.text('p', plugin.description || '未填写简介。'), overview, configHost, runtime, advanced);
        }
        async function mutate(action, plugin) {
            if (scope.busy || !botId) return;
            if (!plugin.bot_exists || plugin.bot_id !== botId) {U.notice(feedback, '当前 Bot 的状态尚未刷新，请重试。', 'error'); return;}
            let value;
            try {if (action === 'save') value = editor.getValue();}
            catch (error) {U.notice(feedback, '配置校验失败：' + error.message, 'error'); return;}
            scope.busy = true; U.controlsBusy(container, true);
            U.notice(feedback, action === 'save' ? '正在保存并应用配置…' : '正在执行实例操作…', 'loading');
            const chosenBot = botId;
            try {
                let result;
                if (action === 'save') result = await scope.request(`/api/plugins/${encodeURIComponent(plugin.id)}`, {method: 'PUT', body: {bot_id: chosenBot, config: value, apply: true}});
                else result = await scope.request(`/api/plugins/${encodeURIComponent(plugin.id)}/${action === 'toggle' ? 'toggle' : 'restart'}`, {method: 'POST', body: {bot_id: chosenBot, ...(action === 'toggle' ? {enabled: !plugin.enabled} : {})}});
                if (!scope.current()) return;
                editor = null;
                scope.busy = false;
                const refreshed = await reload(false);
                if (!scope.current()) return;
                if (!refreshed) {U.notice(feedback, '操作已提交，但刷新实际状态失败：' + loadError + (result?.restart_error ? '；插件实例重启失败：' + result.restart_error : ''), 'error'); return;}
                if (result?.restart_error) U.notice(feedback, '配置已保存，但重启插件实例失败：' + result.restart_error, 'error');
                else U.notice(feedback, action === 'save' ? (result.applied ? '配置已保存，插件实例已应用新配置。' : '配置已保存。') : '操作完成，已刷新实际状态。', 'success');
            } catch (error) {if (scope.current()) U.notice(feedback, U.errorMessage(error), 'error');}
            finally {scope.busy = false; if (scope.current()) U.controlsBusy(container, false);}
        }
        async function backup(id) {
            try {
                const data = await scope.request(`/api/plugins/${encodeURIComponent(id)}/export`);
                if (!scope.current()) return;
                const url = URL.createObjectURL(new Blob([JSON.stringify(data, null, 2)], {type: 'application/json'}));
                const a = U.el('a', {href: url, download: id + '-data.json'}); a.click(); URL.revokeObjectURL(url);
                U.notice(feedback, '私有数据备份已生成。请妥善保管。');
            } catch (error) {if (scope.current()) U.notice(feedback, U.errorMessage(error), 'error');}
        }
        async function restore(file) {
            if (scope.busy || !await U.mayLeave()) return;
            const chosen = file.files[0]; if (!chosen || chosen.size > 4 * 1024 * 1024) {U.notice(feedback, '请选择不超过 4 MiB 的 JSON 数据备份。', 'error'); return;}
            scope.busy = true; U.controlsBusy(container, true);
            try {
                await scope.request('/api/plugins/data/restore', {method: 'POST', body: await chosen.text(), rawBody: true});
                if (!scope.current()) return;
                scope.busy = false; editor = null; await reload(false);
                U.notice(feedback, '数据已恢复，插件保持禁用。', 'success');
            } catch (error) {if (scope.current()) U.notice(feedback, U.errorMessage(error), 'error');}
            finally {scope.busy = false; if (scope.current()) U.controlsBusy(container, false);}
        }
        await reload(false);
    }
    window.PluginPages = {renderPlugins};
})();
