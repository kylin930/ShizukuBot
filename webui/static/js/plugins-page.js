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
        const search = U.el('mdui-text-field', {label: '搜索插件', id: 'plugin-search', clearable: true});
        const filter = U.select('状态筛选', [
            {value: 'all', label: '全部'}, {value: 'enabled', label: '已启用'}, {value: 'disabled', label: '已禁用'},
            {value: 'error', label: '异常'}, {value: 'blocked', label: '策略排除'}, {value: 'running', label: '实例已启动'},
        ], 'all', 'plugin-filter');
        const botSelect = U.select('管理机器人', [], '', 'plugin-bot-select');
        sidebar.append(search, filter, list); body.append(sidebar, detail);
        toolbar.append(botSelect, U.button('刷新', () => reload(true), 'plugin-refresh'));
        container.replaceChildren(U.el('div', {class: 'plugin-page-surface'}, [U.text('h1', '代码插件', 'page-title'),
            toolbar, feedback, body]));
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
            editor = null; sidebar.inert = detail.inert = true;
            detail.setAttribute('aria-busy', 'true');
            if (!plugins.length) detail.replaceChildren(U.text('p', '加载插件…'));
            U.notice(feedback, '加载插件…', 'loading');
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
                U.notice(feedback, botId ? '' : '暂无机器人，请先添加');
                if (!plugins.some(plugin => plugin.id === selectedId)) selectedId = plugins[0]?.id || '';
                editor = null; renderList(); renderDetail(); sidebar.inert = detail.inert = false; detail.setAttribute('aria-busy', 'false'); return true;
            } catch (error) {
                if (!scope.current() || version !== revision) return;
                loadError = U.errorMessage(error); U.notice(feedback, loadError, 'error');
                detail.inert = false; detail.setAttribute('aria-busy', 'false'); detail.replaceChildren(U.text('p', '无法加载插件，请重试')); return false;
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
                    selectedId = plugin.id; U.context.pluginId = selectedId; editor = null; U.activateList(list, 'pluginId', selectedId); renderDetail();
                });
                list.append(item);
            }
            if (!visible.length) list.append(U.text('p', plugins.length ? '没有匹配的插件' : '暂无插件', 'plugin-empty'));
        }
        function renderDetail() {
            const plugin = plugins.find(p => p.id === selectedId);
            if (!plugin) {detail.replaceChildren(U.text('p', '选择插件查看详情')); return;}
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
            const save = U.button('保存并应用', () => mutate('save', plugin), 'plugin-save', 'filled');
            const reset = U.button('重置', async () => {if (await U.mayLeave()) {editor = null; renderDetail();}}, 'plugin-reset');
            save.disabled = reset.disabled = !botId;
            configHost.append(U.text('h3', '配置'), editor?.element || U.text('p', '请先选择机器人'), U.el('div', {class: 'plugin-actions'}, [save, reset]));
            if (plugin.config_error) configHost.append(U.errorDetails('配置无效：' + plugin.config_error));
            const toggle = U.button(plugin.enabled ? '禁用插件' : '启用插件', () => mutate('toggle', plugin), 'plugin-toggle', 'filled');
            toggle.disabled = !botId || (!plugin.enabled && !plugin.allowed);
            const restart = U.button('重启实例', () => mutate('restart', plugin), 'plugin-restart');
            restart.disabled = !botId || !plugin.allowed || !plugin.running;
            actions.append(toggle, restart, U.button('沙盒测试', () => U.openSandbox(botId, plugin.id), 'plugin-sandbox'));
            runtime.append(U.text('h3', '运行状态'), actions,
                U.help('重启说明', ['仅重启此插件实例。源码更新需要重新构建和部署。']));
            if (plugin.restart_required) runtime.append(U.text('p', '配置未应用，请重启实例', 'plugin-warning'));
            if (plugin.last_error) runtime.append(U.text('h4', '最近一次错误'), U.errorDetails(plugin.last_error));
            if (botId && !plugin.allowed) runtime.append(U.text('p', '此机器人已排除此插件。请在机器人配置中允许后再启用。', 'plugin-warning'));
            const advanced = U.el('details', {class: 'plugin-section'}, [U.text('summary', '高级信息与数据备份'),
                U.text('p', '权限：' + (plugin.permissions || []).join(', ')), U.text('h4', 'Manifest'),
                U.text('pre', JSON.stringify(Object.fromEntries(['api_version','id','name','version','description','author','runtime','permissions','config_schema'].map(k => [k, plugin[k]])), null, 2)),
                U.text('h4', '原始 Schema'), U.text('pre', JSON.stringify(plugin.config_schema, null, 2)),
                U.text('p', '备份含私有配置和状态。仅能恢复到空数据范围；恢复后保持禁用，不覆盖已有数据。'),
                U.button('备份配置与状态', () => backup(plugin.id), 'plugin-backup'),
            ]);
            const file = U.el('input', {type: 'file', accept: '.json,application/json', id: 'plugin-restore-file', 'aria-label': '选择插件数据备份'});
            advanced.append(file, U.button('恢复数据备份', () => restore(file), 'plugin-restore'));
            detail.replaceChildren(U.text('h2', plugin.name), U.el('p', {text: plugin.description || '', hidden: !plugin.description}), overview, configHost, runtime, advanced);
            U.enter(detail);
        }
        async function mutate(action, plugin) {
            if (scope.busy || !botId) return;
            if (!plugin.bot_exists || plugin.bot_id !== botId) {U.notice(feedback, '状态已变化，请刷新后重试', 'error'); return;}
            let value;
            try {if (action === 'save') value = editor.getValue();}
            catch (error) {U.notice(feedback, '配置无效：' + error.message, 'error'); return;}
            scope.busy = true; U.controlsBusy(container, true);
            U.notice(feedback, action === 'save' ? '正在保存并应用…' : '更新实例…', 'loading');
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
                if (!refreshed) {U.notice(feedback, '已提交，状态刷新失败：' + loadError + (result?.restart_error ? '；实例重启失败：' + result.restart_error : ''), 'error'); return;}
                if (result?.restart_error) U.notice(feedback, '配置已保存，但实例重启失败：' + result.restart_error, 'error');
                else U.notice(feedback, action === 'save' ? (result.applied ? '配置已应用' : '已保存') : '已更新', 'success');
            } catch (error) {if (scope.current()) U.notice(feedback, U.errorMessage(error), 'error');}
            finally {scope.busy = false; if (scope.current()) U.controlsBusy(container, false);}
        }
        async function backup(id) {
            try {
                const data = await scope.request(`/api/plugins/${encodeURIComponent(id)}/export`);
                if (!scope.current()) return;
                const url = URL.createObjectURL(new Blob([JSON.stringify(data, null, 2)], {type: 'application/json'}));
                const a = U.el('a', {href: url, download: id + '-data.json'}); a.click(); URL.revokeObjectURL(url);
                U.notice(feedback, '备份已生成，请妥善保管', 'success');
            } catch (error) {if (scope.current()) U.notice(feedback, U.errorMessage(error), 'error');}
        }
        async function restore(file) {
            if (scope.busy || !await U.mayLeave()) return;
            const chosen = file.files[0]; if (!chosen || chosen.size > 4 * 1024 * 1024) {U.notice(feedback, '请选择不超过 4 MiB 的 JSON 数据备份', 'error'); return;}
            scope.busy = true; U.controlsBusy(container, true);
            try {
                await scope.request('/api/plugins/data/restore', {method: 'POST', body: await chosen.text(), rawBody: true});
                if (!scope.current()) return;
                scope.busy = false; editor = null; await reload(false);
                U.notice(feedback, '数据已恢复，插件保持禁用', 'success');
            } catch (error) {if (scope.current()) U.notice(feedback, U.errorMessage(error), 'error');}
            finally {scope.busy = false; if (scope.current()) U.controlsBusy(container, false);}
        }
        await reload(false);
    }
    window.PluginPages = {renderPlugins};
})();
