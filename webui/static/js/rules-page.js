// Data-only text rules. No plugin UI selection state, code editor or build step.
(() => {
    'use strict';
    const U = window.AdminUI;
    const kinds = {exact: '完全匹配', contains: '包含', prefix: '前缀'};
    const statuses = {disabled: '已禁用', skipped_builtin: '跳过内置命令', skipped_stop: '因停止策略跳过', canceled: '已取消',
        not_matched: '未匹配', matched: '已匹配', limited: '命中但受限', send_failed: '回复失败', replied: '预期回复（仅捕获）'};
    const reasons = {expanded_reply_limit: '展开后超过 8192 字节', expanded_reply_empty: '展开后为空', reply_attempt_limit: '已达 3 次回复尝试上限',
        reply_unavailable: '没有回复能力', timeout: '执行超时', canceled: '已取消', reply_failed: '发送失败'};
    async function render(container) {
        const scope = U.begin(container);
        let botId = '', set = null, selectedId = '', draft = null, baseline = '', isNew = false, generation = 0;
        const feedback = U.el('div', {id: 'rules-feedback'});
        const botSelect = U.select('管理机器人', [], '', 'rules-bot-select');
        const create = U.button('新建规则', newRule, 'rules-new', 'filled'); create.disabled = true;
        const refresh = U.button('刷新', () => load(true), 'rules-refresh');
        const search = U.el('mdui-text-field', {label: '搜索规则', id: 'rules-search', clearable: true});
        const filter = U.select('状态筛选', [{value: 'all', label: '全部'}, {value: 'enabled', label: '已启用'}, {value: 'disabled', label: '已禁用'}], 'all', 'rules-filter');
        const list = U.el('mdui-list', {id: 'rules-list'});
        const detail = U.el('mdui-card', {variant: 'outlined', class: 'plugin-detail', id: 'rules-details'});
        const sidebar = U.el('mdui-card', {variant: 'outlined', class: 'plugin-sidebar'}, [search, filter, list]);
        container.replaceChildren(U.el('div', {class: 'plugin-page-surface rules-page'}, [
            U.text('h1', '规则', 'page-title'),
            U.help('规则说明', ['支持完全匹配、包含和前缀；区分大小写，保留空白。', '优先级高的规则先执行；可限定群聊或 C2C 范围。保存后生效。', '停止策略只影响后续规则。内置命令优先；规则与插件可能重复回复。']),
            U.el('div', {class: 'plugin-toolbar'}, [botSelect, create, refresh]), feedback,
            U.el('div', {class: 'plugin-workspace'}, [sidebar, detail]),
        ]));
        scope.dirty = () => Boolean(draft && (isNew || U.canonical(draft) !== baseline));
        scope.on(search, 'input', renderList); scope.on(filter, 'change', renderList);
        scope.on(botSelect, 'change', async () => {
            const next = botSelect.value;
            if (next === botId) return;
            if (!await U.mayLeave()) {botSelect.value = botId; return;}
            botId = next; selectedId = ''; await load(false);
        });
        function path() { return `/api/bots/${encodeURIComponent(botId)}/rules`; }
        async function load(checkDirty) {
            if (checkDirty && !await U.mayLeave()) return;
            const token = ++generation;
            draft = null; set = null; isNew = false; create.disabled = true; list.replaceChildren();
            detail.replaceChildren(U.text('p', '正在加载规则…')); U.notice(feedback, '正在加载规则…', 'loading');
            try {
                const bots = (await scope.request('/api/bots')).sort((a, b) => a.id.localeCompare(b.id));
                if (!scope.current() || token !== generation) return;
                if (!bots.some(bot => bot.id === botId)) botId = bots[0]?.id || '';
                botSelect.replaceChildren(...bots.map(bot => U.el('mdui-menu-item', {value: bot.id, text: `${bot.name || bot.id} · ${bot.id}`})));
                botSelect.value = botId; botSelect.disabled = !botId;
                if (!botId) {
                    detail.replaceChildren(U.text('p', '请先添加机器人。'));
                    U.notice(feedback, '暂无机器人，请先添加。'); return;
                }
                const next = await scope.request(path());
                if (!scope.current() || token !== generation) return;
                apply(next, selectedId); U.notice(feedback, '');
            } catch (error) {
                if (!scope.current() || token !== generation) return;
                U.notice(feedback, U.errorMessage(error), 'error');
                detail.replaceChildren(U.text('p', '无法加载规则，请刷新重试。'));
            }
        }
        function apply(next, prefer) {
            set = next; create.disabled = false;
            const chosen = set.rules.find(rule => rule.id === prefer) || set.rules[0];
            selectRule(chosen); renderList();
        }
        function selectRule(rule) {
            selectedId = rule?.id || ''; draft = rule ? U.clone(rule) : null;
            baseline = draft ? U.canonical(draft) : ''; isNew = false; renderDetail();
        }
        function renderList() {
            const query = search.value.trim().toLocaleLowerCase(), mode = filter.value;
            const visible = (set?.rules || []).filter(rule => `${rule.name} ${rule.id} ${rule.trigger.value}`.toLocaleLowerCase().includes(query) &&
                (mode === 'all' || (mode === 'enabled' ? rule.enabled : !rule.enabled)));
            list.replaceChildren();
            for (const rule of visible) {
                const groupScope = rule.conditions.group_ids.length ? rule.conditions.group_ids.join(', ') : '所有群';
                const item = U.el('mdui-list-item', {descriptionLine: 3, rounded: true, active: selectedId === rule.id, 'data-rule-id': rule.id,
                    description: `${rule.enabled ? '已启用' : '已禁用'} · 优先级 ${rule.priority} · ${kinds[rule.trigger.type]} ${JSON.stringify(rule.trigger.value)} · ${rule.conditions.sources.join(' / ')} · ${groupScope}`});
                // MDUI list headlines use the default slot, not a headline property.
                item.append(U.text('span', rule.name));
                scope.on(item, 'click', async () => {
                    if (rule.id === selectedId || !await U.mayLeave()) return;
                    selectRule(rule); renderList(); detail.scrollIntoView({block: 'start', behavior: 'smooth'});
                }); list.append(item);
            }
            if (!visible.length) list.append(U.text('p', set?.rules.length ? '没有匹配的规则。' : '暂无规则，点击“新建规则”添加。', 'plugin-empty'));
        }
        async function newRule() {
            if (!set || scope.busy || !await U.mayLeave()) return;
            if (set.rules.length >= 100) {U.notice(feedback, '每个 Bot 最多 100 条规则。', 'error'); return;}
            const token = crypto.getRandomValues(new Uint8Array(16));
            const id = 'rule-' + Array.from(token, byte => byte.toString(16).padStart(2, '0')).join('');
            draft = {id, name: '', enabled: false, priority: 100,
                trigger: {type: 'contains', value: ''}, conditions: {sources: ['group', 'c2c'], group_ids: []},
                action: {type: 'reply_text', text: ''}, stop_on_match: true};
            selectedId = draft.id; isNew = true; baseline = ''; renderList(); renderDetail();
            detail.scrollIntoView({block: 'start', behavior: 'smooth'});
        }
        function dirtyLabel() {const host = detail.querySelector('#rules-dirty'); if (host) host.textContent = scope.dirty() ? '有未保存的修改' : '已保存';}
        function field(id, label, value, change, attributes = {}) {
            const node = U.el('mdui-text-field', {id, label, value: String(value), ...attributes});
            scope.on(node, 'input', () => {change(node.value); dirtyLabel();}); return node;
        }
        function renderDetail() {
            if (!draft) {detail.replaceChildren(U.text('p', '选择或新建规则。')); return;}
            const r = draft;
            const name = field('rule-name', '规则名称（最多 128 字节）', r.name, v => {r.name = v;});
            const kind = U.select('触发方式', Object.entries(kinds).map(([value, label]) => ({value, label})), r.trigger.type, 'rule-kind');
            scope.on(kind, 'change', () => {r.trigger.type = kind.value; dirtyLabel();});
            const keyword = field('rule-keyword', '关键词（保留空白，最多 1024 字节）', r.trigger.value, v => {r.trigger.value = v;}, {autosize: true, rows: 1});
            const source = U.select('消息来源', [{value: 'both', label: '群聊和 C2C'}, {value: 'group', label: '仅群聊'}, {value: 'c2c', label: '仅 C2C'}], r.conditions.sources.length === 2 ? 'both' : r.conditions.sources[0], 'rule-source');
            scope.on(source, 'change', () => {r.conditions.sources = source.value === 'both' ? ['group', 'c2c'] : [source.value]; dirtyLabel();});
            const groups = field('rule-groups', '指定群 ID（逗号或换行分隔，最多 32 个）', r.conditions.group_ids.join('\n'), v => {r.conditions.group_ids = v.split(/[,，\n]/).map(id => id.trim()).filter(Boolean);}, {autosize: true, rows: 2});
            const reply = field('rule-reply', '文本回复（最多 4096 字节）', r.action.text, v => {r.action.text = v;}, {autosize: true, rows: 3});
            const priority = field('rule-priority', '优先级（-1000 至 1000 的整数）', r.priority, v => {r.priority = v === '' ? null : Number(v);}, {type: 'number', min: -1000, max: 1000, step: 1});
            const enabled = U.el('mdui-switch', {id: 'rule-enabled', checked: r.enabled});
            scope.on(enabled, 'change', () => {r.enabled = enabled.checked; dirtyLabel();});
            const stop = U.el('mdui-checkbox', {id: 'rule-stop', checked: r.stop_on_match, text: '回复成功后停止后续规则'});
            scope.on(stop, 'change', () => {r.stop_on_match = stop.checked; dirtyLabel();});
            const actions = U.el('div', {class: 'plugin-actions'}, [
                U.button('保存规则', () => save(), 'rule-save', 'filled'),
                U.button('重置', async () => {if (await U.mayLeave()) {selectRule(set.rules.find(rule => rule.id === selectedId)); renderList();}}, 'rule-reset'),
            ]);
            if (!isNew) actions.append(U.button(r.enabled ? '立即禁用' : '立即启用', toggle, 'rule-toggle'), U.button('删除规则', remove, 'rule-delete'));
            detail.replaceChildren(U.text('h2', isNew ? '新建规则' : r.name), U.text('p', `${botId} · 修订 ${set.revision} · ${r.id}`, 'plugin-muted'),
                U.el('div', {id: 'rules-dirty', class: 'plugin-muted'}),
                U.el('div', {class: 'rules-form'}, [name, kind, keyword, source, groups,
                    U.text('p', '留空表示所有群，不影响 C2C；所有条件须同时满足。', 'plugin-muted'),
                    reply, U.text('p', '占位符：{用户ID}、{群ID}、{BotID}、{消息内容}；展开上限 8192 字节。', 'plugin-muted'),
                    priority, U.text('p', '同优先级按 ID 排序；每条消息最多尝试回复 3 次。', 'plugin-muted'),
                    U.el('label', {class: 'rules-switch-label'}, [enabled, U.text('span', '启用（保存后生效）')]), stop]), actions, testPanel());
            dirtyLabel();
        }
        function proposed(temporaryEnable = false) {
            if (!draft.name.trim()) throw Error('请填写规则名称。');
            if (draft.trigger.value === '') throw Error('关键词不能为空，空白按原文匹配。');
            if (!draft.action.text.trim()) throw Error('回复内容不能为空。');
            if (!Number.isInteger(draft.priority) || draft.priority < -1000 || draft.priority > 1000) throw Error('优先级必须是 -1000 至 1000 的整数。');
            const rule = U.clone(draft); if (temporaryEnable) rule.enabled = true;
            return [...set.rules.filter(old => old.id !== rule.id), rule];
        }
        async function commit(rules, selected, message) {
            if (scope.busy) return;
            scope.busy = true; U.controlsBusy(container, true); U.notice(feedback, '保存规则…', 'loading');
            try {
                const next = await scope.request(path(), {method: 'PUT', body: {revision: set.revision, rules}});
                if (!scope.current()) return;
                apply(next, selected); U.notice(feedback, message, 'success');
            } catch (error) {
                if (scope.current()) U.notice(feedback, error.code === 409 ? '版本冲突，规则已被其他操作更新。草稿已保留，请复制后刷新。' : U.errorMessage(error), 'error');
            } finally {scope.busy = false; if (scope.current()) U.controlsBusy(container, false);}
        }
        async function save() {
            let next; try {next = proposed();} catch (error) {U.notice(feedback, error.message, 'error'); return;}
            await commit(next, selectedId, draft.enabled ? '已保存并启用。' : '已保存，未启用。');
        }
        async function toggle() {
            if (scope.busy || !await U.mayLeave()) return;
            const id = selectedId, old = set.rules.find(rule => rule.id === id);
            await commit(set.rules.map(rule => rule.id === id ? {...rule, enabled: !old.enabled} : rule), id, old.enabled ? '已禁用。' : '已启用。');
        }
        async function remove() {
            if (scope.busy || !await U.mayLeave()) return;
            try {await mdui.confirm({headline: '删除规则？', description: '删除后无法恢复。', confirmText: '确认删除', cancelText: '取消'});} catch {return;}
            await commit(set.rules.filter(rule => rule.id !== selectedId), '', '规则已删除。');
        }
        function testPanel() {
            const output = U.el('div', {id: 'rules-test-result', 'aria-live': 'polite'});
            const message = U.el('mdui-text-field', {label: '模拟消息', id: 'rule-test-message', autosize: true, rows: 2});
            const source = U.select('模拟来源', [{value: 'group', label: '群聊'}, {value: 'c2c', label: 'C2C 私聊'}], 'group', 'rule-test-source');
            const group = U.el('mdui-text-field', {label: '群 ID（群聊测试必填）', id: 'rule-test-group'});
            const user = U.el('mdui-text-field', {label: '用户 ID（可选）', id: 'rule-test-user'});
            scope.on(source, 'change', () => {group.disabled = source.value === 'c2c';});
            const test = async draftMode => {
                if (scope.busy) return;
                let payload;
                try {
                    if (!message.value) throw Error('请输入模拟消息。');
                    if (source.value === 'group' && !group.value.trim()) throw Error('群聊测试需要群 ID。');
                    payload = {message: message.value, source: source.value, group_id: source.value === 'group' ? group.value.trim() : '', user_id: user.value.trim()};
                    if (draftMode) payload.rules = proposed(true);
                } catch (error) {U.notice(output, error.message, 'error'); return;}
                scope.busy = true; U.controlsBusy(container, true); U.notice(output, '测试中…', 'loading');
                try {
                    const result = await scope.request(path() + '/test', {method: 'POST', body: payload});
                    if (!scope.current()) return;
                    output.className = ''; output.replaceChildren(U.text('h4', `${result.draft ? '草稿' : '已保存规则'} · 修订 ${result.revision} · ${result.attempts} 次捕获回复`));
                    if (result.skip_reason) output.append(U.text('p', result.skip_reason === 'builtin_command' ? '内置命令优先，本次跳过规则。' : result.skip_reason, 'plugin-warning'));
                    for (const [index, trace] of result.traces.entries()) {
                        const card = U.el('section', {class: 'rules-trace', 'data-trace-rule': trace.rule_id}, [U.text('h4', `${index + 1}. ${trace.name} · 优先级 ${trace.priority}`),
                            U.text('p', statuses[trace.status] || trace.status), U.text('p', trace.rule_id, 'plugin-muted')]);
                        if (trace.evaluated) card.append(U.text('p', `触发器 ${trace.trigger_matched ? '✓' : '✗'} · 来源 ${trace.source_allowed ? '✓' : '✗'} · 群范围 ${trace.group_allowed ? '✓' : '✗'}`));
                        if (trace.reason) card.append(U.text('p', reasons[trace.reason] || trace.reason, 'plugin-warning'));
                        if (trace.reply_text) card.append(U.text('pre', trace.reply_text));
                        output.append(card);
                    }
                    if (!result.traces.length) output.append(U.text('p', '暂无规则。'));
                } catch (error) {if (scope.current()) U.notice(output, U.errorMessage(error), 'error');}
                finally {scope.busy = false; if (scope.current()) U.controlsBusy(container, false);}
            };
            return U.el('section', {class: 'plugin-section rules-test'}, [U.text('h3', '规则测试'),
                U.text('p', '仅模拟回复，不会发送 QQ 消息。', 'plugin-muted'),
                U.help('测试说明', ['草稿测试会临时启用此规则，不保存修改。其他规则的状态、优先级和停止策略保持不变；不调用插件。']),
                message, source, group, user, U.el('div', {class: 'plugin-actions'}, [
                    U.button('测试草稿（临时启用）', () => test(true), 'rule-test-draft', 'filled'),
                    U.button('测试已保存规则', () => test(false), 'rule-test-saved'),
                ]), output]);
        }
        await load(false);
    }
    window.RulesPage = {render};
})();
