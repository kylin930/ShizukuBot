// Functional sandbox: every request uses capture-only replies and temporary state.
(() => {
    'use strict';
    const U = window.PluginUI;
    function appendReply(host, reply) {
        const kind = typeof reply.kind === 'string' ? reply.kind : 'unknown';
        const names = new Map([['text','文本'],['markdown','Markdown 源文本'],['image','图片信息'],['rich_media','富媒体信息'],['keyboard','Markdown 与按钮数据'],['video','视频信息']]);
        const card = U.el('div', {class: 'sandbox-reply', 'data-reply-kind': kind});
        card.append(U.text('strong', names.get(kind) || '未知回复类型：' + kind));
        if (reply.text != null) card.append(U.text(kind === 'text' ? 'div' : 'pre', String(reply.text)));
        if (reply.url) {
            const source = String(reply.url);
            let safe = false;
            try {const parsed = new URL(source); safe = ['http:','https:'].includes(parsed.protocol) && !parsed.username && !parsed.password;} catch { /* Display invalid URLs only as text. */ }
            if (safe) card.append(U.el('a', {href: source, target: '_blank', rel: 'noopener noreferrer', text: source, referrerPolicy: 'no-referrer'}));
            else {card.append(U.text('pre', source.slice(0, 4096)), U.text('small', '此链接不提供可点击预览。')); if (source.length > 4096) card.append(U.el('details', {}, [U.text('summary', '链接已截断，展开完整内容'), U.text('pre', source)]));}
        }
        if (reply.keyboard != null) card.append(U.text('pre', typeof reply.keyboard === 'string' ? reply.keyboard : JSON.stringify(reply.keyboard, null, 2)));
        if (!names.has(kind)) card.append(U.text('pre', JSON.stringify(reply, null, 2)));
        if (['image','rich_media','video'].includes(kind)) card.append(U.text('small', '媒体信息（不加载外部资源）', 'plugin-muted'));
        if (kind === 'markdown' || kind === 'keyboard') card.append(U.text('small', '按源文本显示。', 'plugin-muted'));
        host.append(card);
    }
    function appendExecution(host, execution) {
        const labels = new Map([['replied','已完成并捕获回复'],['no_reply','处理完成，无回复'],['unmatched','无匹配处理器'],['failed','执行失败'],['timeout','执行超时'],['canceled','执行已取消']]);
        let label = labels.get(execution.outcome);
        if (!label) label = execution.status === 'failed' ? '执行失败' : '已完成（后端未报告匹配情况）';
        const row = U.el('div', {class: 'sandbox-execution', 'data-outcome': execution.outcome || execution.status});
        row.append(U.text('strong', `${execution.plugin_id}：${label}`));
        const facts = [];
        if (typeof execution.reply_count === 'number') facts.push(`捕获 ${execution.reply_count} 条回复`);
        if (typeof execution.duration_ms === 'number') facts.push(`耗时 ${execution.duration_ms.toFixed(1)} ms`);
        if (typeof execution.matched === 'boolean') facts.push(execution.matched ? '已匹配' : '未匹配');
        if (facts.length) row.append(U.text('div', facts.join(' · '), 'plugin-muted'));
        host.append(row);
    }
    async function render(container) {
        const scope = U.begin(container);
        let bots = [], plugins = [], selected = '', revision = 0;
        let turns = [];
        const feedback = U.el('div', {id: 'sandbox-feedback'});
        const chat = U.el('div', {id: 'sandbox-chat', class: 'sandbox-chat', role: 'log', 'aria-live': 'polite'});
        const botSelect = U.select('测试范围', [], '', 'sandbox-bot-select');
        const debugId = U.el('mdui-text-field', {label: '独立测试标识', value: 'debug-local', id: 'sandbox-debug-id'});
        const pluginSelect = U.select('测试插件', [], '', 'sandbox-plugin-select');
        const type = U.select('消息来源', [{value:'c2c',label:'C2C 私聊'}, {value:'group',label:'群聊'}], 'c2c', 'sandbox-type');
        const input = U.el('mdui-text-field', {label: '输入模拟消息，Enter 发送，Shift+Enter 换行', rows: 2, id: 'sandbox-input'});
        const send = U.button('发送测试消息', sendMessage, 'sandbox-send', 'filled'); send.disabled = true;
        const clear = U.button('清空会话', () => {turns = []; renderTurns();}, 'sandbox-clear');
        const toolbar = U.el('div', {class: 'sandbox-toolbar'}, [botSelect, debugId, pluginSelect, type, clear]);
        const note = U.el('div', {id: 'sandbox-scope-note', class: 'plugin-muted'});
        const scopeHelp = U.help('范围说明', []);
        container.replaceChildren(U.el('div', {class: 'plugin-page-surface'}, [U.text('h1', '沙盒测试', 'page-title'), toolbar, U.text('p', '仅模拟执行，不会发送 QQ 消息。', 'plugin-muted'), note, scopeHelp, feedback,
            U.el('mdui-card', {variant: 'outlined', class: 'sandbox-workspace'}, [chat, U.el('div', {class: 'sandbox-compose'}, [input, send])])]));
        scope.on(botSelect, 'change', async () => {if (scope.busy) return; selected = botSelect.value; turns = []; renderTurns(); await loadPlugins();});
        scope.on(debugId, 'change', async () => {if (!scope.busy) {turns = []; renderTurns(); await loadPlugins();}});
        scope.on(pluginSelect, 'change', updateNote);
        scope.on(input, 'keydown', event => {if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) {event.preventDefault(); sendMessage();}});
        function independent() {return selected === 'independent';}
        function botId() {return independent() ? debugId.value.trim() : selected.slice(4);}
        function updateNote() {
            const plugin = plugins.find(p => p.id === pluginSelect.value);
            U.notice(note, !independent() && plugin?.enabled === false ? '此插件未启用，仍可测试。' : '');
            scopeHelp.querySelectorAll('p').forEach(node => node.remove());
            scopeHelp.append(U.text('p', independent() ? '使用默认配置，不读取机器人配置。' : '使用此机器人已保存的插件配置，不包含表单中的未保存修改。'),
                U.text('p', '每次测试使用独立临时状态，不启用插件，也不请求外部媒体。'));

        }
        async function loadPlugins() {
            const version = ++revision; debugId.hidden = !independent(); send.disabled = true;
            if (!botId() || new TextEncoder().encode(botId()).length > 256) {U.notice(feedback, '请输入不超过 256 字节的独立调试标识。', 'error'); return;}
            U.notice(feedback, '正在加载调试插件…', 'loading');
            try {
                // Independent mode only needs compiled catalog metadata, never another Bot's config.
                const url = independent() ? '/api/plugins' : `/api/bots/${encodeURIComponent(botId())}/sandbox/plugins`;
                const data = await scope.request(url);
                if (!scope.current() || version !== revision) return;
                plugins = data;
                const choices = [...(independent() ? [] : [{value:'',label:'全部已启用插件'}]), ...plugins.map(p => ({value:p.id,label:p.name + (!independent() && !p.enabled ? '（未启用）' : '')}))];
                pluginSelect.replaceChildren(...choices.map(item => U.el('mdui-menu-item', {value:item.value,text:item.label})));
                pluginSelect.value = plugins.some(p => p.id === U.context.pluginId) ? U.context.pluginId : plugins[0]?.id || '';
                send.disabled = !plugins.length; updateNote(); U.notice(feedback, plugins.length ? '' : '暂无可测试插件。');
            } catch (error) {if (scope.current() && version === revision) U.notice(feedback, U.errorMessage(error), 'error');}
        }
        function renderTurns() {
            chat.replaceChildren();
            if (!turns.length) chat.append(U.text('p', '输入消息开始测试。', 'plugin-empty'));
            for (const turn of turns) {
                const block = U.el('section', {class: 'sandbox-turn'});
                block.append(U.text('div', `${turn.independent ? '独立调试' : 'Bot'} ${turn.botId} · ${turn.type === 'group' ? '群聊' : 'C2C'} · ${turn.pluginId || '全部启用插件'}`, 'plugin-muted'),
                    U.text('div', turn.content, 'sandbox-request'));
                if (turn.pending) block.append(U.text('p', '正在执行…'));
                if (turn.data) {
                    const data = turn.data;
                    if (Array.isArray(data.results) && data.results.length) data.results.forEach(reply => appendReply(block, reply));
                    else if (data.replies?.length) data.replies.forEach(reply => appendReply(block, {kind:'text',text:reply}));
                    (data.executions || []).forEach(execution => appendExecution(block, execution));
                    (data.errors || []).forEach(error => block.append(U.errorDetails(error)));
                    if (!data.executions?.length) block.append(U.text('p', '没有执行插件，请选择插件重试。“全部”仅运行已启用且允许的插件。'));
                    block.append(U.text('small', `本次请求耗时 ${turn.elapsed.toFixed(1)} ms`, 'plugin-muted'));
                }
                if (turn.error) block.append(U.errorDetails(turn.error));
                chat.append(block);
            }
            chat.scrollTop = chat.scrollHeight;
        }
        async function sendMessage() {
            if (scope.busy || send.disabled) return;
            const content = input.value.trim();
            if (!content) {U.notice(feedback, '请输入模拟消息。', 'error'); return;}
            if (new TextEncoder().encode(content).length > 65536) {U.notice(feedback, '消息不得超过 64 KiB。', 'error'); return;}
            const chosenBot = botId();
            if (!chosenBot || new TextEncoder().encode(chosenBot).length > 256) {U.notice(feedback, '独立调试标识无效。', 'error'); return;}
            const turn = {botId:chosenBot,pluginId:pluginSelect.value,type:type.value,content,independent:independent(),pending:true};
            turns.push(turn); if (turns.length > 100) turns.shift(); input.value = ''; scope.busy = true; U.controlsBusy(container, true); send.loading = true;
            U.notice(feedback, ''); renderTurns(); const started = performance.now();
            try {
                const data = await scope.request(`/api/bots/${encodeURIComponent(chosenBot)}/sandbox/send`, {method:'POST',body:{type:turn.type,content,plugin_id:turn.pluginId,independent:turn.independent}});
                if (!scope.current()) return;
                turn.data = data; turn.elapsed = performance.now() - started;
            } catch (error) {if (scope.current()) turn.error = U.errorMessage(error);}
            finally {
                scope.busy = false;
                if (scope.current()) {turn.pending = false; U.controlsBusy(container, false); send.loading = false; renderTurns(); input.focus();}
            }
        }
        renderTurns();
        try {
            bots = (await scope.request('/api/bots')).sort((a,b) => a.id.localeCompare(b.id));
            if (!scope.current()) return;
            selected = bots.some(bot => bot.id === U.context.botId) ? 'bot:' + U.context.botId : bots[0] ? 'bot:' + bots[0].id : 'independent';
            botSelect.replaceChildren(...bots.map(bot => U.el('mdui-menu-item', {value:'bot:' + bot.id,text:`${bot.name || bot.id} · ${bot.id}`})), U.el('mdui-menu-item', {value:'independent',text:'独立测试'}));
            botSelect.value = selected; await loadPlugins();
        } catch (error) {if (scope.current()) {send.disabled = true; U.notice(feedback, U.errorMessage(error), 'error');}}
    }
    window.SandboxPage = {render, appendReply, appendExecution};
})();
