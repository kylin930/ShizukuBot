// Shared administration utilities, plugin-specific editors and sandbox helpers.
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
            if (error.name !== 'AbortError' && node.isConnected) {
                const host = node.closest('.plugin-page-surface')?.querySelector('#plugin-feedback, #sandbox-feedback, #rules-feedback');
                if (host) notice(host, errorMessage(error), 'error');
                else window.showSnackbar?.('操作失败，请检查输入');
            }
        }));
        return node;
    }
    function select(label, options, value, id) {
        return el('mdui-select', {label, id, value}, options.map(option => el('mdui-menu-item', {value: option.value, text: option.label})));
    }
    const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)');
    const motions = new Set();
    const moving = new WeakMap();
    function cancelMotion() { for (const animation of motions) animation.cancel(); motions.clear(); }
    reducedMotion.addEventListener('change', () => {if (reducedMotion.matches) cancelMotion();});
    function enter(node, page = false) {
        moving.get(node)?.cancel();
        if (!node?.isConnected || reducedMotion.matches || typeof node.animate !== 'function') return;
        const style = getComputedStyle(document.documentElement);
        const token = style.getPropertyValue(page ? '--mdui-motion-duration-medium1' : '--mdui-motion-duration-short3').trim();
        const duration = token ? parseFloat(token) * (token.endsWith('ms') ? 1 : 1000) : (page ? 250 : 150);
        const animation = node.animate([{opacity: .78, transform: `translateY(${page ? 6 : 3}px)`}, {opacity: 1, transform: 'none'}], {
            duration, easing: style.getPropertyValue('--mdui-motion-easing-emphasized-decelerate').trim() || 'cubic-bezier(.05,.7,.1,1)',
        });
        moving.set(node, animation); motions.add(animation);
        animation.finished.catch(() => {}).finally(() => {motions.delete(animation); if (moving.get(node) === animation) moving.delete(node);});
        return animation;
    }
    function activateList(list, key, id) {
        for (const item of list.children) if (item.dataset[key]) item.active = item.dataset[key] === id;
    }
    function reveal(node) { if (window.matchMedia('(max-width: 900px)').matches) node.scrollIntoView({block: 'start', behavior: reducedMotion.matches ? 'auto' : 'smooth'}); }
    function notice(host, message, kind = 'info') {
        host.replaceChildren(); host.className = '';
        if (!message) return;
        if (kind === 'success') {window.showSnackbar?.(message); return;}
        // A loading button already identifies the in-flight operation.
        if (kind === 'loading' && [...(host.closest('.plugin-page-surface')?.querySelectorAll('mdui-button') || [])].some(button => button.loading)) return;
        host.className = 'plugin-notice ' + kind;
        host.setAttribute('role', kind === 'error' ? 'alert' : 'status');
        if (kind === 'loading') host.append(el('mdui-circular-progress', {class: 'plugin-spinner', 'aria-label': '加载中'}));
        host.append(text('span', message));
    }
    function help(label, paragraphs) {
        return el('details', {class: 'ui-help'}, [text('summary', label), ...paragraphs.map(value => text('p', value))]);
    }
    function errorMessage(error) {
        if (error.status === 401) return '登录已过期，请重新登录';
        if (error.status === 403) return '权限不足，请检查账号和请求来源';
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
            error.status = response.status; error.code = body?.code; throw error;
        }
        if (!body) throw Error('服务器响应无效，请重试');
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
    function dispose() {active?.cleanup?.(); active?.controller.abort(); active = null; cancelMotion(); }
    async function confirmDiscard() {
        try { await mdui.confirm({closeOnEsc: true, headline: '有未保存的配置', description: '离开会放弃本次修改', confirmText: '放弃修改', cancelText: '继续编辑'}); return true; }
        catch { return false; }
    }
    async function mayLeave() {
        if (!active) return true;
        if (active.busy) {window.showSnackbar?.('操作中，请稍候'); return false;}
        if (!active.dirty()) return true;
        if (!leavePromise) leavePromise = confirmDiscard().finally(() => {leavePromise = null;});
        return leavePromise;
    }
    function controlsBusy(root, busy) {
        const trigger = busy ? document.activeElement?.closest('mdui-button, mdui-button-icon') : null;
        root.inert = busy;
        root.setAttribute('aria-busy', String(busy));
        if (busy) {
            if (trigger && root.contains(trigger)) {root.dataset.busyButton = trigger.id; trigger.loading = true;}
        }
        root.querySelectorAll('mdui-button, mdui-button-icon, mdui-select, mdui-text-field, mdui-switch, mdui-checkbox, textarea, input').forEach(control => {
            if (busy) {if (!Object.hasOwn(control.dataset, 'wasDisabled')) control.dataset.wasDisabled = String(Boolean(control.disabled)); control.disabled = true;}
            else if (Object.hasOwn(control.dataset, 'wasDisabled')) {control.disabled = control.dataset.wasDisabled === 'true'; delete control.dataset.wasDisabled;}
        });
        if (!busy && root.dataset.busyButton) {
            const trigger = document.getElementById(root.dataset.busyButton);
            if (trigger && root.contains(trigger)) {trigger.loading = false; trigger.focus({preventScroll: true});}
            delete root.dataset.busyButton;
        }
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
    // Neutral page helpers share lifecycle guards without exposing plugin selection state.
    window.AdminUI = {el, text, clone, canonical, button, select, notice, request, begin, dispose, mayLeave,
        controlsBusy, errorMessage, errorDetails, help, enter, activateList, reveal};
    window.PluginUI = {...window.AdminUI, JSONEditor, context, openSandbox};
})();
