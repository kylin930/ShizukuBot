// Schema-driven editor for the SDK's deliberately small configuration dialect.
(() => {
    'use strict';
    const U = window.PluginUI;
    const own = (value, key) => value != null && Object.hasOwn(value, key);
    const object = value => value !== null && typeof value === 'object' && !Array.isArray(value);
    const pathText = path => '$' + path.map(key => typeof key === 'number' ? `[${key}]` : `[${JSON.stringify(key)}]`).join('');
    const equal = (a, b) => U.canonical(a) === U.canonical(b);
    function validate(schema, supplied, path = [], errors = []) {
        let value = supplied;
        if (value === undefined && own(schema, 'default')) value = U.clone(schema.default);
        if (value === undefined) return errors;
        const error = message => errors.push({path: pathText(path), message});
        const type = schema.type || 'object';
        if (type === 'object') {
            if (!object(value)) {error('应为对象'); return errors;}
            const properties = schema.properties || {};
            for (const key of Object.keys(value)) {
                if (!own(properties, key)) errors.push({path: pathText([...path, key]), message: 'Schema 未声明此字段'});
                else validate(properties[key], value[key], [...path, key], errors);
            }
            for (const key of schema.required || []) {
                if (!own(value, key) && !own(properties[key] || {}, 'default')) errors.push({path: pathText([...path, key]), message: '必填字段尚未设置'});
            }
        } else if (type === 'string' && typeof value !== 'string') error('应为字符串');
        else if (type === 'boolean' && typeof value !== 'boolean') error('应为布尔值');
        else if (type === 'number' || type === 'integer') {
            if (typeof value !== 'number' || !Number.isFinite(value) || (type === 'integer' && !Number.isInteger(value))) error(type === 'integer' ? '应为整数' : '应为有效数值');
            else {
                if (own(schema, 'minimum') && value < schema.minimum) error('不得小于 ' + schema.minimum);
                if (own(schema, 'maximum') && value > schema.maximum) error('不得大于 ' + schema.maximum);
            }
        } else if (type === 'array') {
            if (!Array.isArray(value)) error('应为数组');
            else if (schema.items) value.forEach((item, index) => validate(schema.items, item, [...path, index], errors));
        }
        if (schema.enum?.length && !schema.enum.some(candidate => equal(candidate, value))) error('请选择枚举中的值');
        return errors;
    }
    function formSupport(schema, value, depth = 0, budget = {count: 0}) {
        if (depth > 8 || ++budget.count > 500) return false;
        const type = schema.type || 'object';
        if (!['object','string','boolean','integer','number','array'].includes(type)) return false;
        if (value === null) return false;
        if (type === 'object') {
            if (value !== undefined && !object(value)) return false;
            const properties = schema.properties || {};
            if (Object.keys(value || {}).some(key => !own(properties, key))) return false;
            return Object.entries(properties).every(([key, child]) => formSupport(child, own(value, key) ? value[key] : undefined, depth + 1, budget));
        }
        if (type === 'array') return Boolean(schema.items) && (value === undefined || (Array.isArray(value) && value.length <= 100 && value.every(item => formSupport(schema.items, item, depth + 1, budget))));
        if (value !== undefined && ((type === 'string' && typeof value !== 'string') || (type === 'boolean' && typeof value !== 'boolean') || (['integer','number'].includes(type) && typeof value !== 'number'))) return false;
        return true;
    }
    function defaultValue(schema) {
        if (own(schema, 'default')) return U.clone(schema.default);
        if (schema.enum?.length) return U.clone(schema.enum[0]);
        switch (schema.type || 'object') {
            case 'string': return '';
            case 'boolean': return false;
            case 'number': case 'integer': return Math.max(schema.minimum ?? 0, Math.min(schema.maximum ?? 0, 0));
            case 'array': return [];
            default: {
                const value = {};
                for (const [key, child] of Object.entries(schema.properties || {})) if (own(child, 'default') || schema.required?.includes(key)) Object.defineProperty(value, key, {value: defaultValue(child), enumerable: true, writable: true, configurable: true});
                return value;
            }
        }
    }
    class ConfigEditor {
        constructor(schema = {}, value = {}) {
            this.schema = U.clone(schema); this.value = U.clone(value); this.initial = U.canonical(value);
            this.invalid = new Map(); this.controls = new Map(); this.raw = JSON.stringify(value, null, 2);
            this.mode = formSupport(this.schema, this.value) ? 'form' : 'json';
            this.feedback = U.el('div', {'aria-live': 'polite'});
            this.body = U.el('div', {class: 'schema-body'});
            this.formButton = U.button('表单编辑', () => this.switchMode('form'), 'plugin-form-mode');
            this.jsonButton = U.button('JSON 编辑', () => this.switchMode('json'), 'plugin-json-mode');
            this.element = U.el('div', {class: 'schema-editor'}, [U.el('div', {class: 'plugin-actions'}, [this.formButton, this.jsonButton]), this.feedback, this.body]);
            this.render();
        }
        get(path) {
            let value = this.value;
            for (const key of path) {if (!own(value, key)) return undefined; value = value[key];}
            return value;
        }
        set(path, value) {
            const next = U.clone(this.value); let parent = next;
            let schema = this.schema;
            for (const key of path.slice(0, -1)) {
                schema = typeof key === 'number' ? schema.items : schema.properties[key];
                if (!own(parent, key)) Object.defineProperty(parent, key, {value: defaultValue(schema), enumerable: true, writable: true, configurable: true});
                parent = parent[key];
            }
            const key = path[path.length - 1];
            if (value === undefined) delete parent[key];
            else Object.defineProperty(parent, key, {value: U.clone(value), enumerable: true, writable: true, configurable: true});
            this.value = next;
        }
        isDirty() {
            if (this.invalid.size) return true;
            try {return U.canonical(this.mode === 'json' ? JSON.parse(this.raw) : this.value) !== this.initial;} catch {return true;}
        }
        getValue() {
            let value;
            try {value = this.mode === 'json' ? JSON.parse(this.raw) : this.value;}
            catch (error) {U.notice(this.feedback, 'JSON 语法错误：' + error.message, 'error'); throw Error('JSON 语法错误');}
            const errors = object(value) ? validate(this.schema, value) : [{path: '$', message: '配置根节点必须为对象'}];
            for (const [path, message] of this.invalid) errors.push({path, message});
            for (const [path, control] of this.controls) {control.error = errors.some(error => error.path === path);}
            if (errors.length) {
                const message = errors.map(error => error.path + '：' + error.message).join('\n');
                U.notice(this.feedback, message, 'error'); throw Error(message);
            }
            U.notice(this.feedback, ''); return U.clone(value);
        }
        switchMode(mode) {
            if (mode === this.mode) return;
            try {
                if (this.mode === 'json') {
                    const value = JSON.parse(this.raw);
                    if (!object(value)) throw Error('配置根节点必须为对象');
                    if (!formSupport(this.schema, value)) throw Error('此配置暂不支持表单，请使用 JSON 编辑；内容已保留。');
                    this.value = U.clone(value);
                } else {
                    if (this.invalid.size) throw Error('请先修正无效数值，再切换模式。');
                    this.raw = JSON.stringify(this.value, null, 2);
                }
                this.mode = mode; this.render();
            } catch (error) {U.notice(this.feedback, error.message, 'error');}
        }
        render() {
            this.body.replaceChildren(); this.controls.clear(); U.notice(this.feedback, '');
            this.formButton.variant = this.mode === 'form' ? 'filled' : 'outlined';
            this.jsonButton.variant = this.mode === 'json' ? 'filled' : 'outlined';
            if (this.mode === 'json') {
                if (!formSupport(this.schema, this.value)) U.notice(this.feedback, '此配置使用 JSON 编辑。');
                const input = U.el('textarea', {class: 'plugin-json', value: this.raw, 'aria-label': '插件配置 JSON', spellcheck: false});
                input.addEventListener('input', () => {this.raw = input.value;}); this.body.append(input); return;
            }
            if (!Object.keys(this.schema.properties || {}).length) {this.body.append(U.text('p', '此插件无需配置。')); return;}
            this.renderObject(this.schema, [], this.body);
        }
        renderObject(schema, path, host) {
            for (const [key, child] of Object.entries(schema.properties || {})) this.renderField(child, [...path, key], key, (schema.required || []).includes(key), host);
        }
        renderField(schema, path, label, required, host) {
            const current = this.get(path), type = schema.type || 'object', key = pathText(path);
            const section = U.el('div', {class: 'schema-field', 'data-schema-field': key});
            section.append(U.text('div', label + (required ? ' *' : ''), 'schema-label'));
            if (schema.description) section.append(U.text('p', schema.description, 'plugin-muted'));
            if (own(schema, 'default')) section.append(U.text('small', '默认值：' + JSON.stringify(schema.default), 'plugin-muted'));
            host.append(section);
            if (!required) {
                const enabled = U.el('mdui-checkbox', {checked: current !== undefined, text: '设置此字段', 'aria-label': '设置字段 ' + label});
                enabled.addEventListener('change', async () => {
                    if (this.invalid.size) {enabled.checked = current !== undefined; U.notice(this.feedback, '请先修正无效数值再修改字段。', 'error'); return;}
                    this.set(path, enabled.checked ? defaultValue(schema) : undefined);
                    // MDUI performs checked/validity work after its update promise.
                    // Preserve the draft immediately, then detach only after that work.
                    await enabled.updateComplete;
                    await new Promise(resolve => requestAnimationFrame(resolve));
                    if (enabled.isConnected) this.render();
                });
                section.append(enabled);
                if (current === undefined) {section.append(U.text('span', own(schema, 'default') ? ' 未设置时使用默认值' : ' 未设置', 'plugin-muted')); return;}
            }
            const value = current === undefined ? defaultValue(schema) : current;
            if (schema.enum?.length) {
                const selected = schema.enum.findIndex(candidate => equal(candidate, value));
                const control = U.select(label, schema.enum.map((candidate, index) => ({value: String(index), label: typeof candidate === 'string' ? candidate : JSON.stringify(candidate)})), selected < 0 ? '' : String(selected));
                control.setAttribute('data-schema-path', key);
                control.addEventListener('change', () => {if (/^\d+$/.test(control.value)) this.set(path, schema.enum[Number(control.value)]);});
                section.append(control); this.controls.set(key, control); return;
            }
            if (type === 'object') {this.renderObject(schema, path, section); return;}
            if (type === 'array') {
                const array = Array.isArray(value) ? value : [];
                array.forEach((item, index) => {
                    const row = U.el('div', {class: 'schema-array-item'});
                    this.renderField(schema.items, [...path, index], '项目 ' + (index + 1), true, row);
                    row.append(U.button('移除项目', () => {if (this.invalid.size) {U.notice(this.feedback, '请先修正无效数值再移除项目。', 'error'); return;} const next = U.clone(this.get(path)); next.splice(index, 1); this.set(path, next); this.render();})); section.append(row);
                });
                const add = U.button('添加项目', () => {if (this.invalid.size) {U.notice(this.feedback, '请先修正无效数值再添加项目。', 'error'); return;} this.set(path, [...(this.get(path) || []), defaultValue(schema.items)]); this.render();});
                add.disabled = array.length >= 100; section.append(add); return;
            }
            if (type === 'boolean') {
                const control = U.el('mdui-switch', {checked: value === true, 'aria-label': label, 'data-schema-path': key});
                control.addEventListener('change', () => this.set(path, control.checked)); section.append(control); this.controls.set(key, control); return;
            }
            const numeric = type === 'integer' || type === 'number';
            const control = U.el('mdui-text-field', {label, type: numeric ? 'number' : 'text', value: String(value ?? ''), 'data-schema-path': key});
            if (numeric) {
                if (own(schema, 'minimum')) control.min = schema.minimum;
                if (own(schema, 'maximum')) control.max = schema.maximum;
                control.step = type === 'integer' ? 1 : 'any';
            }
            control.addEventListener('input', () => {
                if (!numeric) {this.set(path, control.value); return;}
                const number = Number(control.value);
                if (!control.value.trim() || !Number.isFinite(number) || (type === 'integer' && !Number.isInteger(number))) {
                    this.invalid.set(key, type === 'integer' ? '请输入整数' : '请输入有效数值'); control.error = true;
                } else {this.invalid.delete(key); control.error = false; this.set(path, number);}
            });
            section.append(control); this.controls.set(key, control);
        }
    }
    U.ConfigEditor = ConfigEditor;
    U.Schema = {validate, formSupport, defaultValue};
})();
