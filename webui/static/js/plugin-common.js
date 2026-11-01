// Shared utilities for plugin administration and capture-only sandbox pages.
// Plain browser JavaScript; no bundler or runtime dependency.
(() => {
    'use strict';
    let active = null;
    let leavePromise = null;
    const context = {botId: '', pluginId: ''};
    function el(tag, attributes = {}, children = []) {
        const node = document.createElement(tag);
        for (const [key, value] of Object.entries(attributes)) {
            if (key === 'text') node.textContent = value ?? '';
            else if (key === 'class') node.className = value;
            else if (key.startsWith('aria-') || key.startsWith('data-') || key === 'role') node.setAttribute(key, value);
            else if (key !== 'value') node[key] = value;
        }
        for (const child of children) if (child != null) node.append(typeof child === 'string' ? document.createTextNode(child) : child);
        if (Object.hasOwn(attributes, 'value')) node.value = attributes.value;
        return node;
    }
    const text = (tag, value, className = '') => el(tag, {text: value, class: className});
    const clone = value => JSON.parse(JSON.stringify(value));
    const canonical = value => {
        if (Array.isArray(value)) return '[' + value.map(canonical).join(',') + ']';
        if (value && typeof value === 'object') return '{' + Object.keys(value).sort().map(k => JSON.stringify(k) + ':' + canonical(value[k])).join(',') + '}';
        return JSON.stringify(value);
    };
    function button(label, action, id, variant = 'outlined') {
        const node = el('mdui-button', {text: label, variant, id});
        node.addEventListener('click', () => Promise.resolve().then(action).catch(error => {
            if (error.name !== 'AbortError') window.showSnackbar?.(error.message);
        }));
        return node;
    }
    function select(label, options, value, id) {
        return el('mdui-select', {label, id, value}, options.map(option => el('mdui-menu-item', {value: option.value, text: option.label})));
    }
    function notice(host, message, kind = 'info') {
        host.replaceChildren(); host.className = 'plugin-notice ' + kind;
        host.setAttribute('role', kind === 'error' ? 'alert' : 'status');
        if (kind === 'loading') host.append(el('mdui-circular-progress', {class: 'plugin-spinner'}));
        if (message) host.append(text('span', message)); else host.className = '';
    }
    function errorMessage(error) {
        if (error.status === 401) return '登录已失效，请重新登录后重试。';
        if (error.status === 403) return '权限不足：此操作需要管理员权限，或请求来源未通过检查。';
        return error.message || '请求失败';
    }
    async function request(url, options = {}) {
        const {rawBody, ...init} = options;
        if (Object.hasOwn(init, 'body')) {
            init.headers = {...init.headers, 'Content-Type': 'application/json'};
            if (!rawBody) init.body = JSON.stringify(init.body);
        }
        const response = await fetch(url, init);
        const source = await response.text();
        let body; try { body = JSON.parse(source); } catch { body = null; }
        if (!response.ok || (body && typeof body.code === 'number' && body.code !== 0)) {
            const error = Error(body?.message || source.slice(0, 1000) || `HTTP ${response.status}`);
            error.status = response.status; throw error;
        }
        if (!body) throw Error('服务器返回了无效的 JSON，请重试。');
        return typeof body.code === 'number' ? body.data : body;
    }
    function begin(container) {
        dispose();
        const controller = new AbortController();
        const scope = {
            container, controller, busy: false, dirty: () => false,
            current: () => active === scope && !controller.signal.aborted && container.isConnected,
            request: (url, options = {}) => request(url, {...options, signal: controller.signal}),
            on: (node, event, fn) => node.addEventListener(event, fn, {signal: controller.signal}),
        };
        active = scope;
        scope.on(window, 'beforeunload', event => { if (scope.dirty()) {event.preventDefault(); event.returnValue = '';} });
        return scope;
    }
    function dispose() { active?.controller.abort(); active = null; }
    async function confirmDiscard() {
        try { await mdui.confirm({headline: '有未保存的配置', description: '离开会放弃本次修改。', confirmText: '放弃修改', cancelText: '继续编辑'}); return true; }
        catch { return false; }
    }
    async function mayLeave() {
        if (!active) return true;
        if (active.busy) {window.showSnackbar?.('正在执行操作，请完成后再切换。'); return false;}
        if (!active.dirty()) return true;
        if (!leavePromise) leavePromise = confirmDiscard().finally(() => {leavePromise = null;});
        return leavePromise;
    }
    function controlsBusy(root, busy) {
        root.querySelectorAll('mdui-button, mdui-button-icon, mdui-select, mdui-text-field, mdui-switch, mdui-checkbox, textarea, input').forEach(control => {
            if (busy) {control.dataset.wasDisabled = String(Boolean(control.disabled)); control.disabled = true;}
            else if (Object.hasOwn(control.dataset, 'wasDisabled')) {control.disabled = control.dataset.wasDisabled === 'true'; delete control.dataset.wasDisabled;}
        });
    }
    function errorDetails(message) {
        const details = el('details', {class: 'plugin-error-details'}, [text('summary', String(message).slice(0, 180)), text('pre', message)]);
        return details;
    }
    // Stage B fallback. The schema editor replaces this class without changing page state flow.
    class JSONEditor {
        constructor(schema, value) {
            this.initial = canonical(value); this.input = el('textarea', {class: 'plugin-json', value: JSON.stringify(value, null, 2), 'aria-label': '插件配置 JSON'});
            this.element = this.input;
        }
        getValue() { return JSON.parse(this.input.value); }
        isDirty() {try {return canonical(this.getValue()) !== this.initial;} catch {return true;} }
    }
    async function openSandbox(botId, pluginId) {
        const old = {...context}; context.botId = botId; context.pluginId = pluginId;
        if (!await window.navigateTo('/sandbox')) Object.assign(context, old);
    }
    window.PluginUI = {el, text, clone, canonical, button, select, notice, request, begin, dispose, mayLeave,
        controlsBusy, errorMessage, errorDetails, JSONEditor, context, openSandbox};
})();
