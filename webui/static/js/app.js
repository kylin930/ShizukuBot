// ShizukuBot WebUI 应用逻辑 - MDUI2

// ==================== 全局状态 ====================
let currentRoute = '/stats';
let statsRefreshTimer = null;
let logWebSocket = null;
let historyChart = null;
let hourlyChart = null;
let historyPage = 1;
const historyPageSize = 50;

// API 接口列表
const apiList = [
    { method: 'GET',    path: '/api/bots',                      desc: '获取所有机器人列表' },
    { method: 'GET',    path: '/api/bots/{id}',                 desc: '获取单个机器人信息' },
    { method: 'POST',   path: '/api/bots',                      desc: '创建新机器人' },
    { method: 'PUT',    path: '/api/bots/{id}',                 desc: '更新机器人配置' },
    { method: 'DELETE', path: '/api/bots/{id}',                 desc: '删除机器人' },
    { method: 'POST',   path: '/api/bots/{id}/start',           desc: '启动机器人' },
    { method: 'POST',   path: '/api/bots/{id}/stop',            desc: '停止机器人' },
    { method: 'POST',   path: '/api/bots/{id}/message/group',   desc: '发送群消息' },
    { method: 'POST',   path: '/api/bots/{id}/message/c2c',     desc: '发送C2C消息' },
    { method: 'GET',    path: '/api/stats',                     desc: '获取汇总统计数据' },
    { method: 'GET',    path: '/api/bots/{id}/stats',           desc: '获取单个机器人统计' },
    { method: 'GET',    path: '/api/config',                    desc: '获取当前配置' },
    { method: 'PUT',    path: '/api/config',                    desc: '更新配置' },
    { method: 'GET',    path: '/api/logs',                      desc: '获取历史日志' },
];

// ==================== API 封装 ====================
async function apiGet(url) {
    const resp = await fetch(url);
    return resp.json();
}

async function apiPost(url, data) {
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data),
    });
    return resp.json();
}

async function apiPut(url, data) {
    const resp = await fetch(url, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data),
    });
    return resp.json();
}

async function apiDelete(url) {
    const resp = await fetch(url, { method: 'DELETE' });
    return resp.json();
}

// ==================== 路由管理 ====================
function navigateTo(route) {
    currentRoute = route;
    window.location.hash = route;
    renderPage();

    // 更新移动端导航项激活状态
    document.querySelectorAll('.nav-item').forEach(item => {
        item.active = item.dataset.route === route;
    });

    // 更新桌面端 NavigationRail 选中项
    const rail = document.querySelector('.desktop-nav');
    if (rail) {
        const routeMap = {
            '/stats': 'stats',
            '/bots': 'bots',
            '/logs': 'logs',
            '/api-test': 'api-test',
            '/plugins': 'plugins',
            '/community': 'community',
            '/sandbox': 'sandbox',
            '/config': 'config',
            '/users': 'users',
            '/about': 'about',
        };
        rail.value = routeMap[route] || 'stats';
    }
}

function renderPage() {
    const content = document.getElementById('app-content');
    
    content.classList.remove('fade-in');
    content.classList.add('fade-out');

    setTimeout(() => {
        if (statsRefreshTimer) {
            clearInterval(statsRefreshTimer);
            statsRefreshTimer = null;
        }
        if (logWebSocket) {
            logWebSocket.close();
            logWebSocket = null;
        }

        switch (currentRoute) {
            case '/stats':     renderStats(content); break;
            case '/bots':      renderBots(content); break;
            case '/logs':      renderLogs(content); break;
            case '/api-test':  renderApiTest(content); break;
            case '/plugins':   renderPlugins(content); break;
            case '/community': renderCommunity(content); break;
            case '/sandbox':   renderSandbox(content); break;
            case '/config':    renderConfig(content); break;
            case '/users':     renderUsers(content); break;
            case '/about':     renderAbout(content); break;
            default:           renderStats(content); break;
        }

        content.classList.remove('fade-out');
        content.classList.add('fade-in');
    }, 500);
}

// ==================== 统计页 ====================
async function renderStats(container) {
    container.innerHTML = `
        <div class="page-title">统计</div>

        <!-- 机器人管理 -->
        <mdui-card variant="outlined" class="bots-card">
            <div style="font-size: 16px; font-weight: 500; margin-bottom: 12px; display: flex; align-items: center; gap: 8px;">
                <mdui-icon name="smart_toy"></mdui-icon> 机器人管理            </div>
            <div id="bots-list"></div>
        </mdui-card>

        <!-- 统计卡片 -->
        <div class="stats-grid">
            <mdui-card variant="outlined" class="stat-card">
                <div class="stat-card-label"><mdui-icon name="group"></mdui-icon> 用户数量</div>
                <div class="stat-card-value" id="stat-users">-</div>
            </mdui-card>
            <mdui-card variant="outlined" class="stat-card">
                <div class="stat-card-label"><mdui-icon name="groups"></mdui-icon> 群组数量</div>
                <div class="stat-card-value" id="stat-groups">-</div>
            </mdui-card>
            <mdui-card variant="outlined" class="stat-card">
                <div class="stat-card-label"><mdui-icon name="today"></mdui-icon> 今日DAU</div>
                <div class="stat-card-value" id="stat-dau">-</div>
            </mdui-card>
            <mdui-card variant="outlined" class="stat-card">
                <div class="stat-card-label"><mdui-icon name="speed"></mdui-icon> 当前QPS</div>
                <div class="stat-card-value" id="stat-qps">-</div>
            </mdui-card>
            <mdui-card variant="outlined" class="stat-card">
                <div class="stat-card-label"><mdui-icon name="call_received"></mdui-icon> 接收消息</div>
                <div class="stat-card-value" id="stat-received">-</div>
            </mdui-card>
            <mdui-card variant="outlined" class="stat-card">
                <div class="stat-card-label"><mdui-icon name="send"></mdui-icon> 发送消息</div>
                <div class="stat-card-value" id="stat-sent">-</div>
            </mdui-card>
        </div>

        <!-- 图表 -->
        <div class="charts-grid">
            <mdui-card variant="outlined" class="chart-card">
                <div style="font-size: 16px; font-weight: 500; margin-bottom: 12px">历史消息数量</div>
                <div class="chart-container"><canvas id="history-chart"></canvas></div>
            </mdui-card>
            <mdui-card variant="outlined" class="chart-card">
                <div style="font-size: 16px; font-weight: 500; margin-bottom: 12px">每小时消息数量</div>
                <div class="chart-container"><canvas id="hourly-chart"></canvas></div>
            </mdui-card>
        </div>
    `;

    await loadBotsList();
    await updateStats();
    initHistoryChart();
    initHourlyChart();
    statsRefreshTimer = setInterval(() => {
        updateStats();
        loadBotsList();
    }, 5000);
}

async function loadBotsList() {
    try {
        const data = await apiGet('/api/bots');
        const listEl = document.getElementById('bots-list');
        if (!listEl) return;

        if (data.code !== 0 || !data.data || data.data.length === 0) {
            listEl.innerHTML = '<div style="padding:16px;color:var(--mdui-color-on-surface-variant)">暂无机器人，请在配置页添加</div>';
            return;
        }

        listEl.innerHTML = data.data.map(bot => {
                const statusColor = bot.running ? '#4caf50' : '#9e9e9e';
                const statusText = bot.running ? '运行中' : '已停止';
                const modeText = bot.mode === 'websocket' ? 'WebSocket' : 'Webhook';
                const modeColor = bot.mode === 'websocket' ? '#1976d2' : '#f57c00';
                const btnText = bot.running ? '停止' : '启动';
                const btnIcon = bot.running ? 'stop' : 'play_arrow';
                const btnVariant = bot.running ? 'outlined' : 'filled';
                const btnAction = bot.running ? 'stop' : 'start';

                return `
                    <div class="bot-item" data-bot-id="${bot.id}">
                        <div class="bot-item-info">
                            <span class="bot-status-dot" style="background:${statusColor}"></span>
                            <div class="bot-item-text">
                                <div class="bot-item-name">${escapeHtml(bot.name || bot.id)}</div>
                                <div class="bot-item-meta">
                                    ID: ${escapeHtml(bot.id)} · AppID: ${escapeHtml(bot.app_id)} · 
                                    <span class="bot-mode" style="color:${modeColor}">${modeText}</span> · ${statusText}
                                </div>
                            </div>
                        </div>
                        <mdui-button variant="${btnVariant}" icon="${btnIcon}" data-action="${btnAction}" data-id="${bot.id}">
                            ${btnText}
                        </mdui-button>
                    </div>
                `;
            }).join('');

        // 绑定按钮事件
        listEl.querySelectorAll('[data-action]').forEach(btn => {
            btn.addEventListener('click', async () => {
                const id = btn.dataset.id;
                const action = btn.dataset.action;
                btn.loading = true;
                try {
                    if (action === 'start') {
                        await apiPost(`/api/bots/${id}/start`, {});
                    } else {
                        await apiPost(`/api/bots/${id}/stop`, {});
                    }
                    await loadBotsList();
                } catch (e) {
                    showSnackbar('操作失败: ' + e.message);
                }
                btn.loading = false;
            });
        });
    } catch (e) {
        console.error('加载机器人列表失败', e);
    }
}

async function updateStats() {
    try {
        const data = await apiGet('/api/stats');
        if (data.code === 0 && data.data) {
            const s = data.data;
            document.getElementById('stat-users').textContent = s.user_count || 0;
            document.getElementById('stat-groups').textContent = s.group_count || 0;
            document.getElementById('stat-dau').textContent = s.today_dau || 0;
            document.getElementById('stat-qps').textContent = s.qps || 0;
            document.getElementById('stat-received').textContent = s.received_messages || 0;
            document.getElementById('stat-sent').textContent = s.sent_messages || 0;

            if (s.hourly_stats && hourlyChart) {
                const sentData = s.hourly_stats.map(h => h.sent || 0);
                const receivedData = s.hourly_stats.map(h => h.received || 0);
                hourlyChart.data.datasets[0].data = sentData;
                hourlyChart.data.datasets[1].data = receivedData;
                hourlyChart.update();
            }
        }
    } catch (e) {
        console.error('更新统计失败:', e);
    }
}

function initHistoryChart() {
    const ctx = document.getElementById('history-chart');
    if (!ctx) return;
    historyChart = new Chart(ctx, {
        type: 'line',
        data: {
            labels: [],
            datasets: [
                { label: '发送', data: [], borderColor: 'rgb(25, 118, 210)', tension: 0.3, fill: false },
                { label: '接收', data: [], borderColor: 'rgb(245, 124, 0)', tension: 0.3, fill: false },
            ],
        },
        options: { responsive: true, maintainAspectRatio: false },
    });
}

function initHourlyChart() {
    const ctx = document.getElementById('hourly-chart');
    if (!ctx) return;
    hourlyChart = new Chart(ctx, {
        type: 'bar',
        data: {
            labels: Array.from({ length: 24 }, (_, i) => i + ':00'),
            datasets: [
                { label: '发送', data: Array(24).fill(0), backgroundColor: 'rgba(25, 118, 210, 0.6)' },
                { label: '接收', data: Array(24).fill(0), backgroundColor: 'rgba(245, 124, 0, 0.6)' },
            ],
        },
        options: { responsive: true, maintainAspectRatio: false },
    });
}

// ==================== 日志页 ====================
function renderLogs(container) {
    container.innerHTML = `
        <div class="page-title">日志</div>
        <div class="logs-controls">
            <mdui-text-field label="搜索" icon="search" id="log-search"></mdui-text-field>
            <mdui-select value="all" id="log-filter">
                <mdui-menu-item value="all">全部</mdui-menu-item>
                <mdui-menu-item value="debug">Debug</mdui-menu-item>
                <mdui-menu-item value="info">Info</mdui-menu-item>
                <mdui-menu-item value="warn">Warn</mdui-menu-item>
                <mdui-menu-item value="error">Error</mdui-menu-item>
            </mdui-select>
            <mdui-button variant="tonal" icon="refresh" id="log-refresh">刷新</mdui-button>
        </div>
        <div class="logs-container">
            <mdui-card variant="outlined" class="log-card">
                <div class="log-card-header">
                    <span class="log-card-title">实时日志</span>
                    <mdui-chip icon="radio_button_checked" selected>实时</mdui-chip>
                </div>
                <div class="logs-area" id="realtime-logs"></div>
            </mdui-card>
            <mdui-card variant="outlined" class="log-card">
                <div class="log-card-header">
                    <span class="log-card-title">历史日志</span>
                </div>
                <div class="logs-area" id="history-logs"></div>
                <div class="pagination" id="log-pagination"></div>
            </mdui-card>
        </div>
    `;

    initRealtimeLogs();
    loadHistoryLogs();

    document.getElementById('log-refresh').addEventListener('click', loadHistoryLogs);
    document.getElementById('log-filter').addEventListener('change', () => {
        historyPage = 1;
        loadHistoryLogs();
    });
    document.getElementById('log-search').addEventListener('input', () => {
        historyPage = 1;
        loadHistoryLogs();
    });
}

function initRealtimeLogs() {
    const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${wsProtocol}//${window.location.host}/ws/logs`;
    logWebSocket = new WebSocket(wsUrl);

    const area = document.getElementById('realtime-logs');

    logWebSocket.onmessage = (event) => {
        const log = JSON.parse(event.data);
        appendLogItem(area, log);
        // 限制最多200条
        while (area.children.length > 200) {
            area.removeChild(area.firstChild);
        }
    };

    logWebSocket.onerror = (e) => console.error('WebSocket错误:', e);
}

function appendLogItem(container, log) {
    const item = document.createElement('div');
    item.className = 'log-item';
    const time = new Date(log.timestamp).toLocaleTimeString('zh-CN');
    const levelClass = `log-level-${log.level.toLowerCase()}`;
    item.innerHTML = `
        <span class="log-time">${time}</span>
        <span class="log-level ${levelClass}">[${log.level}]</span>
        <span class="log-message">${escapeHtml(log.message)}</span>
    `;
    container.appendChild(item);
    container.scrollTop = container.scrollHeight;
}

async function loadHistoryLogs() {
    const filter = document.getElementById('log-filter');
    const search = document.getElementById('log-search');
    const level = filter ? filter.value : 'all';
    const keyword = search ? search.value : '';
    const offset = (historyPage - 1) * historyPageSize;

    try {
        const params = new URLSearchParams({
            limit: historyPageSize,
            offset: offset,
        });
        if (level !== 'all') params.set('level', level);
        if (keyword) params.set('keyword', keyword);

        const data = await apiGet(`/api/logs?${params.toString()}`);
        const area = document.getElementById('history-logs');
        area.innerHTML = '';

        if (data.code === 0 && data.data && data.data.logs) {
            data.data.logs.forEach(log => appendLogItem(area, log));
        } else {
            area.innerHTML = '<div style="padding:16px;color:var(--mdui-color-on-surface-variant)">暂无日志</div>';
        }

        renderPagination(data.data?.total || 0);
    } catch (e) {
        console.error('加载历史日志失败:', e);
    }
}

function renderPagination(total) {
    const container = document.getElementById('log-pagination');
    const totalPages = Math.ceil(total / historyPageSize);
    if (totalPages <= 1) {
        container.innerHTML = '';
        return;
    }

    let html = '';
    if (historyPage > 1) {
        html += `<mdui-button-icon icon="chevron_left" data-page="${historyPage - 1}"></mdui-button-icon>`;
    }
    html += `<span style="padding:0 12px">${historyPage} / ${totalPages}</span>`;
    if (historyPage < totalPages) {
        html += `<mdui-button-icon icon="chevron_right" data-page="${historyPage + 1}"></mdui-button-icon>`;
    }
    container.innerHTML = html;

    container.querySelectorAll('[data-page]').forEach(btn => {
        btn.addEventListener('click', () => {
            historyPage = parseInt(btn.dataset.page);
            loadHistoryLogs();
        });
    });
}

// ==================== 接口调试页 ====================
function renderApiTest(container) {
    container.innerHTML = `
        <div class="page-title">接口调试</div>
        <div class="api-test-container">
            <mdui-card variant="outlined" class="api-list-card">
                <div class="api-list-title">API 列表</div>
                <mdui-list id="api-list">
                    ${apiList.map((api, i) => `
                        <mdui-list-item
                            headline="${api.method} ${api.path}"
                            description="${api.desc}"
                            data-index="${i}"
                            class="api-list-item">
                        </mdui-list-item>
                    `).join('')}
                </mdui-list>
            </mdui-card>
            <div class="api-form">
                <mdui-card variant="outlined" class="api-form-card">
                    <div style="font-size:16px;font-weight:500;margin-bottom:16px">请求</div>
                    <div class="api-form-row">
                        <mdui-segmented-button-group selects="single" id="api-method">
                            <mdui-segmented-button value="GET">GET</mdui-segmented-button>
                            <mdui-segmented-button value="POST">POST</mdui-segmented-button>
                            <mdui-segmented-button value="PUT">PUT</mdui-segmented-button>
                            <mdui-segmented-button value="DELETE">DELETE</mdui-segmented-button>
                        </mdui-segmented-button-group>
                    </div>
                    <div class="api-form-row">
                        <mdui-text-field label="路径" id="api-path" full-width></mdui-text-field>
                    </div>
                    <div class="api-form-row">
                        <mdui-text-field
                            label="请求体 (JSON)"
                            id="api-body"
                            full-width
                            rows="4"></mdui-text-field>
                    </div>
                    <mdui-button variant="filled" icon="send" id="api-send">发送请求</mdui-button>
                </mdui-card>
                <mdui-card variant="outlined" class="api-form-card">
                    <div style="font-size:16px;font-weight:500;margin-bottom:16px">响应</div>
                    <pre class="response-pre" id="api-response">点击"发送请求"查看结果</pre>
                </mdui-card>
            </div>
        </div>
    `;

    // 默认选中第一个接口
    const methodGroup = document.getElementById('api-method');
    methodGroup.value = 'GET';
    document.getElementById('api-path').value = '/api/bots';

    document.querySelectorAll('.api-list-item').forEach(item => {
        item.addEventListener('click', () => {
            const idx = parseInt(item.dataset.index);
            const api = apiList[idx];
            methodGroup.value = api.method;
            document.getElementById('api-path').value = api.path;
        });
    });

    document.getElementById('api-send').addEventListener('click', sendApiRequest);
}

async function sendApiRequest() {
    const method = document.getElementById('api-method').value;
    const path = document.getElementById('api-path').value;
    const bodyText = document.getElementById('api-body').value;
    const respArea = document.getElementById('api-response');

    respArea.textContent = '请求中...';

    try {
        const opts = { method };
        if (bodyText && (method === 'POST' || method === 'PUT')) {
            opts.headers = { 'Content-Type': 'application/json' };
            opts.body = bodyText;
        }
        const resp = await fetch(path, opts);
        const text = await resp.text();
        try {
            const json = JSON.parse(text);
            respArea.textContent = JSON.stringify(json, null, 2);
        } catch {
            respArea.textContent = text;
        }
    } catch (e) {
        respArea.textContent = '请求失败: ' + e.message;
    }
}

// ==================== 插件页 ====================
let currentPlugin = null;
let graphCanvas = null;
let graph = null;

async function renderPlugins(container) {
    container.innerHTML = `
        <div class="page-title">插件</div>
        <div class="plugins-layout">
            <!-- 插件列表 -->
            <div class="plugins-sidebar">
                <mdui-button variant="filled" icon="add" id="create-plugin-btn" full-width>新建插件</mdui-button>
                <mdui-list id="plugins-list" style="margin-top:16px">
                    <div style="padding:16px;color:var(--mdui-color-on-surface-variant)">加载中...</div>
                </mdui-list>
            </div>
            <!-- 流程图编辑器 -->
            <div class="plugin-editor">
                <div class="editor-toolbar" id="editor-toolbar" style="display:none">
                    <mdui-text-field label="插件名称" id="plugin-name" style="width:200px"></mdui-text-field>
                    <mdui-text-field label="描述" id="plugin-desc" style="flex:1"></mdui-text-field>
                    <mdui-button variant="filled" icon="save" id="save-plugin-btn">保存</mdui-button>
                    <mdui-button variant="outlined" icon="download" id="export-plugin-btn">导出</mdui-button>
                    <mdui-switch label="启用" id="plugin-enabled"></mdui-switch>
                </div>
                <div class="editor-canvas-container" id="canvas-container">
                    <canvas id="plugin-canvas" width="800" height="600"></canvas>
                </div>
            </div>
        </div>
    `;

    await loadPluginsList();

    document.getElementById('create-plugin-btn').addEventListener('click', createNewPlugin);
}

async function loadPluginsList() {
    try {
        const data = await apiGet('/api/plugins');
        const listEl = document.getElementById('plugins-list');

        if (data.code !== 0 || !data.data || data.data.length === 0) {
            listEl.innerHTML = '<div style="padding:16px;color:var(--mdui-color-on-surface-variant)">暂无插件，点击上方按钮创建</div>';
            return;
        }

        listEl.innerHTML = data.data.map(p => `
            <mdui-list-item
                headline="${escapeHtml(p.name)}"
                description="${escapeHtml(p.description || '暂无描述')}"
                data-plugin-id="${p.id}"
                class="plugin-list-item ${currentPlugin && currentPlugin.id === p.id ? 'active' : ''}"
                icon="${p.enabled ? 'check_circle' : 'radio_button_unchecked'}">
                <mdui-button-icon slot="end-icon" icon="delete" data-delete="${p.id}"></mdui-button-icon>
            </mdui-list-item>
        `).join('');

        // 绑定点击事件
        listEl.querySelectorAll('.plugin-list-item').forEach(item => {
            item.addEventListener('click', () => {
                loadPluginEditor(item.dataset.pluginId);
            });
        });

        // 绑定删除事件
        listEl.querySelectorAll('[data-delete]').forEach(btn => {
            btn.addEventListener('click', async (e) => {
                e.stopPropagation();
                const id = btn.dataset.delete;
                try {
                    await mdui.confirm({
                        headline: '删除插件',
                        description: '确定删除此插件？此操作不可恢复。',
                        confirmText: '删除',
                        cancelText: '取消'
                    });
                    await apiDelete(`/api/plugins/${id}`);
                    await loadPluginsList();
                    if (currentPlugin && currentPlugin.id === id) {
                        currentPlugin = null;
                        resetEditor();
                    }
                } catch {
                    // 用户取消
                }
            });
        });
    } catch (e) {
        console.error('加载插件列表失败:', e);
    }
}

async function createNewPlugin() {
    let name;
    try {
        name = await mdui.prompt({
            headline: '新建插件',
            description: '请输入插件名称：',
            confirmText: '创建',
            cancelText: '取消',
            textFieldOptions: { value: '新插件' }
        });
    } catch {
        return;
    }
    if (!name) return;

    try {
        const data = await apiPost('/api/plugins', {
            name: name,
            description: '',
            bot_id: '*',
            graph: { nodes: [], links: [] }
        });

        if (data.code === 0) {
            await loadPluginsList();
            loadPluginEditor(data.data.id);
        } else {
            showSnackbar('创建失败: ' + data.message);
        }
    } catch (e) {
        showSnackbar('创建失败: ' + e.message);
    }
}

async function loadPluginEditor(pluginId) {
    try {
        const data = await apiGet(`/api/plugins/${pluginId}`);
        if (data.code !== 0) {
            showSnackbar('加载插件失败');
            return;
        }

        currentPlugin = data.data;
        document.getElementById('editor-toolbar').style.display = 'flex';
        document.getElementById('plugin-name').value = currentPlugin.name;
        document.getElementById('plugin-desc').value = currentPlugin.description || '';
        document.getElementById('plugin-enabled').checked = currentPlugin.enabled;

        // 初始化 LiteGraph（先注册自定义节点再配置图）
        initGraphEditor(currentPlugin.graph, currentPlugin.custom_nodes);

        // 更新列表激活状态
        document.querySelectorAll('.plugin-list-item').forEach(item => {
            item.classList.toggle('active', item.dataset.pluginId === pluginId);
        });

        // 绑定保存按钮
        document.getElementById('save-plugin-btn').onclick = saveCurrentPlugin;
        document.getElementById('export-plugin-btn').onclick = exportCurrentPlugin;
        document.getElementById('plugin-enabled').onchange = togglePluginEnabled;
    } catch (e) {
        console.error('加载插件编辑器失败', e);
    }
}

function initGraphEditor(graphData, pluginCustomNodes) {
    const canvas = document.getElementById('plugin-canvas');
    const container = document.getElementById('canvas-container');

    canvas.width = container.clientWidth;
    canvas.height = container.clientHeight;

    // 先加载全局自定义节点
    var loadGlobal = typeof loadCustomNodes === 'function' ? loadCustomNodes() : Promise.resolve();

    // 再注册插件内嵌的自定义节点
    loadGlobal.then(function() {
        if (pluginCustomNodes && Array.isArray(pluginCustomNodes)) {
            for (var i = 0; i < pluginCustomNodes.length; i++) {
                if (typeof registerCustomNodeType === 'function') {
                    registerCustomNodeType(pluginCustomNodes[i]);
                }
            }
        }

        // 最后再配置图，确保所有节点类型都已注册
        graph = new LGraph();

        graphCanvas = new LGraphCanvas(canvas, graph, {
            autoresize: true,
            allow_interaction: true,
            read_only: false
        });

        graphCanvas.background_image = null;

        if (graphData && graphData.nodes) {
            graph.configure(graphData);
        }

        canvas.addEventListener('contextmenu', function(e) {
            e.preventDefault();
            e.stopPropagation();
            return false;
        });

        canvas.addEventListener('mousedown', function(e) {
            if (e.button === 2) {
                e.preventDefault();
                if (graphCanvas) {
                    LGraphCanvas.active_canvas = graphCanvas;
                    graphCanvas.adjustMouseEvent(e);
                    const node = graph.getNodeOnPos(e.canvasX, e.canvasY, graphCanvas.visible_nodes, 5);
                    if (typeof graphCanvas.processContextMenu === 'function') {
                        graphCanvas.processContextMenu(node, e);
                    }
                }
            }
        });

        graphCanvas.showSearchBox = () => {};
    });
}

async function saveCurrentPlugin() {
    if (!currentPlugin) return;

    currentPlugin.name = document.getElementById('plugin-name').value;
    currentPlugin.description = document.getElementById('plugin-desc').value;

    // 导出图数据
    const graphData = graph.serialize();

    try {
        const data = await apiPut(`/api/plugins/${currentPlugin.id}/graph`, graphData);
        if (data.code === 0) {
            showSnackbar('插件已保存');
            await loadPluginsList();
        } else {
            showSnackbar('保存失败: ' + data.message);
        }
    } catch (e) {
        showSnackbar('保存失败: ' + e.message);
    }
}

async function togglePluginEnabled() {
    if (!currentPlugin) return;

    try {
        await apiPost(`/api/plugins/${currentPlugin.id}/toggle`, {});
        currentPlugin.enabled = !currentPlugin.enabled;
        document.getElementById('plugin-enabled').checked = currentPlugin.enabled;
        await loadPluginsList();
    } catch (e) {
        showSnackbar('切换状态失败: ' + e.message);
    }
}

async function exportCurrentPlugin() {
    if (!currentPlugin) return;

    try {
        const response = await fetch(`/api/plugins/${currentPlugin.id}/export`);
        if (!response.ok) {
            throw new Error('导出失败');
        }
        const blob = await response.blob();
        const url = window.URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = `${currentPlugin.name}_${currentPlugin.id}.json`;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        window.URL.revokeObjectURL(url);
        showSnackbar('插件导出成功');
    } catch (e) {
        showSnackbar('导出失败: ' + e.message);
    }
}

// ==================== 配置页 ====================
async function renderConfig(container) {
    container.innerHTML = `
        <div class="page-title">配置</div>

        <!-- WebUI 配置 -->
        <mdui-card variant="outlined" class="config-card">
            <div style="font-size:16px;font-weight:500;margin-bottom:16px">WebUI 设置</div>
            <div style="display:flex;gap:16px;flex-wrap:wrap;margin-bottom:16px">
                <mdui-text-field label="端口" type="number" id="webui-port" style="width:200px"></mdui-text-field>
                <mdui-switch label="启用" id="webui-enabled"></mdui-switch>
            </div>
            <mdui-text-field label="密码" type="password" id="webui-password" toggle-password></mdui-text-field>
        </mdui-card>

        <!-- 插件市场配置 -->
        <mdui-card variant="outlined" class="config-card">
            <div style="font-size:16px;font-weight:500;margin-bottom:16px">插件市场</div>
            <mdui-text-field label="Market API 地址" id="market-api" help-text="插件市场服务端的 API 地址"></mdui-text-field>
        </mdui-card>

        <!-- 内置指令回复模板 -->
        <mdui-card variant="outlined" class="config-card">
            <div style="font-size:16px;font-weight:500;margin-bottom:16px">内置指令回复</div>
            <div style="display:flex;flex-direction:column;gap:16px">
                <mdui-text-field label="/help 回复" id="cmd-help" rows="3" help-text="占位符：{指令列表} {bot名称}"></mdui-text-field>
                <mdui-text-field label="/status 回复" id="cmd-status" rows="3" help-text="占位符：{bot名称} {接收消息数} {发送消息数}"></mdui-text-field>
                <mdui-text-field label="#shizuku 回复" id="cmd-shizuku" rows="4" help-text="占位符：{core版本} {运行时间} {操作系统} {go版本} {bot名称}"></mdui-text-field>
            </div>
        </mdui-card>

        <!-- 机器人配置 -->
        <mdui-card variant="outlined" class="config-card">
            <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:16px">
                <div style="font-size:16px;font-weight:500">机器人列表</div>
                <mdui-button variant="filled" icon="add" id="add-bot-btn">添加机器人</mdui-button>
            </div>
            <div id="bots-config-list"></div>
        </mdui-card>

        <!-- 原始配置 -->
        <mdui-card variant="outlined" class="config-card">
            <div style="font-size:16px;font-weight:500;margin-bottom:16px">config.json (原始)</div>
            <textarea class="config-textarea" id="config-textarea" spellcheck="false"></textarea>
        </mdui-card>

        <div class="config-actions">
            <mdui-button variant="filled" icon="save" id="config-save">保存配置</mdui-button>
            <mdui-button variant="outlined" icon="refresh" id="config-reload">重新加载</mdui-button>
        </div>
    `;

    await loadConfig();

    document.getElementById('add-bot-btn').addEventListener('click', addBotConfig);
    document.getElementById('config-save').addEventListener('click', saveConfig);
    document.getElementById('config-reload').addEventListener('click', loadConfig);
}

let configData = null;

async function loadConfig() {
    try {
        const data = await apiGet('/api/config');
        if (data.code === 0 && data.data) {
            configData = data.data;

            // 填充 WebUI 配置
            const webui = configData.webui || {};
            document.getElementById('webui-port').value = webui.port || 4545;
            document.getElementById('webui-enabled').checked = webui.enabled !== false;
            document.getElementById('webui-password').value = webui.password || '';

            // 填充插件市场配置
            const marketApiInput = document.getElementById('market-api');
            if (marketApiInput) {
                marketApiInput.value = configData.market_api || 'http://localhost:8081/api';
            }

            // 填充内置指令回复模板
            const builtin = configData.builtin_commands || {};
            const cmdHelp = document.getElementById('cmd-help');
            const cmdStatus = document.getElementById('cmd-status');
            const cmdShizuku = document.getElementById('cmd-shizuku');
            if (cmdHelp) cmdHelp.value = builtin.help || '';
            if (cmdStatus) cmdStatus.value = builtin.status || '';
            if (cmdShizuku) cmdShizuku.value = builtin.shizuku || '';

            // 渲染机器人配置列表
            renderBotsConfigList();

            // 填充原始 JSON
            document.getElementById('config-textarea').value = JSON.stringify(configData, null, 2);
        }
    } catch (e) {
        showSnackbar('加载配置失败: ' + e.message);
    }
}

function renderBotsConfigList() {
    const listEl = document.getElementById('bots-config-list');
    const bots = configData.bots || [];

    if (bots.length === 0) {
        listEl.innerHTML = '<div style="padding:16px;color:var(--mdui-color-on-surface-variant)">暂无机器人配置</div>';
        return;
    }

    listEl.innerHTML = bots.map((bot, index) => `
        <div class="bot-config-item" data-index="${index}">
            <mdui-divider style="margin-bottom:12px"></mdui-divider>
            <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(200px,1fr));gap:12px;margin-bottom:12px">
                <mdui-text-field label="ID" value="${escapeHtml(bot.id)}" class="bot-config-id" readonly></mdui-text-field>
                <mdui-text-field label="名称" value="${escapeHtml(bot.name)}" class="bot-config-name"></mdui-text-field>
                <mdui-text-field label="AppID" value="${escapeHtml(bot.app_id)}" class="bot-config-appid"></mdui-text-field>
                <mdui-text-field label="AppSecret" value="${escapeHtml(bot.app_secret)}" type="password" toggle-password class="bot-config-appsecret"></mdui-text-field>
                <div style="display:flex;flex-direction:column;gap:8px">
                    <label style="font-size:14px;color:var(--mdui-color-on-surface-variant)">连接模式</label>
                    <mdui-select value="${bot.mode || 'websocket'}" class="bot-config-mode">
                        <mdui-menu-item value="websocket">WebSocket</mdui-menu-item>
                        <mdui-menu-item value="webhook">Webhook</mdui-menu-item>
                    </mdui-select>
                </div>
            </div>
            <div style="display:flex;justify-content:flex-end;gap:8px">
                <mdui-button variant="outlined" icon="delete" class="delete-bot-btn" data-index="${index}">删除</mdui-button>
            </div>
        </div>
    `).join('');

    // 绑定删除按钮事件
    listEl.querySelectorAll('.delete-bot-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            const index = parseInt(btn.dataset.index);
            configData.bots.splice(index, 1);
            renderBotsConfigList();
        });
    });
}

function addBotConfig() {
    if (!configData.bots) {
        configData.bots = [];
    }
    const newId = 'bot' + Date.now();
    configData.bots.push({
        id: newId,
        name: '',
        app_id: '',
        app_secret: '',
        enabled: false,
        mode: 'websocket'
    });
    renderBotsConfigList();
}

async function saveConfig() {
    try {
        // 更新 configData
        configData.webui = {
            port: parseInt(document.getElementById('webui-port').value) || 4545,
            enabled: document.getElementById('webui-enabled').checked,
            password: document.getElementById('webui-password').value
        };

        // 更新插件市场配置
        const marketApiInput = document.getElementById('market-api');
        if (marketApiInput) {
            configData.market_api = marketApiInput.value.trim();
        }

        // 更新内置指令回复模板
        const cmdHelp = document.getElementById('cmd-help');
        const cmdStatus = document.getElementById('cmd-status');
        const cmdShizuku = document.getElementById('cmd-shizuku');
        configData.builtin_commands = {
            help: cmdHelp ? cmdHelp.value : '',
            status: cmdStatus ? cmdStatus.value : '',
            shizuku: cmdShizuku ? cmdShizuku.value : ''
        };

        // 更新机器人配置
        const botItems = document.querySelectorAll('.bot-config-item');
        botItems.forEach(item => {
            const index = parseInt(item.dataset.index);
            configData.bots[index] = {
                id: item.querySelector('.bot-config-id').value,
                name: item.querySelector('.bot-config-name').value,
                app_id: item.querySelector('.bot-config-appid').value,
                app_secret: item.querySelector('.bot-config-appsecret').value,
                enabled: false,
                mode: item.querySelector('.bot-config-mode').value
            };
        });

        // 更新原始 JSON
        document.getElementById('config-textarea').value = JSON.stringify(configData, null, 2);

        // 保存到服务器
        const data = await apiPut('/api/config', configData);
        if (data.code === 0) {
            showSnackbar('配置保存成功');
        } else {
            showSnackbar('保存失败: ' + data.message);
        }
    } catch (e) {
        showSnackbar('保存失败: ' + e.message);
    }
}

// ==================== 关于页 ====================
// ==================== 用户管理页 ====================
async function renderUsers(container) {
    container.innerHTML = `
        <div class="page-title">用户管理</div>
        <mdui-card variant="outlined" class="config-card">
            <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:16px">
                <div style="font-size:16px;font-weight:500">用户列表</div>
                <mdui-button variant="filled" icon="person_add" id="add-user-btn">添加用户</mdui-button>
            </div>
            <div id="users-list">加载中...</div>
        </mdui-card>
    `;

    await loadUsers();
    document.getElementById('add-user-btn').addEventListener('click', showAddUserDialog);
}

let usersList = [];

async function loadUsers() {
    try {
        const data = await apiGet('/api/users');
        if (data.code === 0) {
            usersList = data.data || [];
            renderUsersList();
        } else {
            document.getElementById('users-list').innerHTML = 
                '<div style="padding:16px;color:var(--mdui-color-error)">' + escapeHtml(data.message) + '</div>';
        }
    } catch (e) {
        document.getElementById('users-list').innerHTML = 
            '<div style="padding:16px;color:var(--mdui-color-error)">加载失败: ' + escapeHtml(e.message) + '</div>';
    }
}

function renderUsersList() {
    const listEl = document.getElementById('users-list');
    if (usersList.length === 0) {
        listEl.innerHTML = '<div style="padding:16px;color:var(--mdui-color-on-surface-variant)">暂无用户</div>';
        return;
    }

    listEl.innerHTML = usersList.map(user => `
        <div style="display:flex;align-items:center;justify-content:space-between;padding:12px 0;border-bottom:1px solid var(--mdui-color-surface-variant)">
            <div style="display:flex;align-items:center;gap:12px">
                <mdui-icon name="person" style="font-size:24px;color:var(--mdui-color-primary)"></mdui-icon>
                <div>
                    <div style="font-weight:500">${escapeHtml(user.username)}</div>
                    <div style="font-size:12px;color:var(--mdui-color-on-surface-variant)">
                        ${user.is_admin ? '管理员' : '普通用户'}
                    </div>
                </div>
            </div>
            <div style="display:flex;gap:8px">
                <mdui-button variant="outlined" size="small" icon="key" onclick="showChangePasswordDialog('${escapeHtml(user.username)}')">修改密码</mdui-button>
                ${user.username !== 'admin' ? 
                    `<mdui-button variant="outlined" size="small" icon="delete" onclick="deleteUser('${escapeHtml(user.username)}')">删除</mdui-button>` : ''}
            </div>
        </div>
    `).join('');
}

function showAddUserDialog() {
    mdui.dialog({
        headline: '添加用户',
        body: `
            <div style="display:flex;flex-direction:column;gap:16px;padding:8px 0">
                <mdui-text-field label="用户名" id="new-username"></mdui-text-field>
                <mdui-text-field label="密码" type="password" toggle-password id="new-password"></mdui-text-field>
                <mdui-switch label="管理员权限" id="new-is-admin"></mdui-switch>
            </div>
        `,
        actions: [
            { text: '取消', variant: 'text' },
            { 
                text: '确定', 
                variant: 'filled',
                onClick: async (dialog) => {
                    const username = dialog.querySelector('#new-username').value;
                    const password = dialog.querySelector('#new-password').value;
                    const isAdmin = dialog.querySelector('#new-is-admin').checked;

                    if (!username || !password) {
                        showSnackbar('用户名和密码不能为空');
                        return false;
                    }

                    try {
                        const data = await apiPost('/api/users', {
                            username: username,
                            password: password,
                            is_admin: isAdmin
                        });
                        if (data.code === 0) {
                            showSnackbar('添加成功');
                            loadUsers();
                        } else {
                            showSnackbar(data.message || '添加失败');
                            return false;
                        }
                    } catch (e) {
                        showSnackbar('添加失败: ' + e.message);
                        return false;
                    }
                }
            }
        ]
    });
}

function showChangePasswordDialog(username) {
    mdui.dialog({
        headline: '修改密码 - ' + username,
        body: `
            <div style="padding:8px 0">
                <mdui-text-field label="新密码" type="password" toggle-password id="change-password"></mdui-text-field>
            </div>
        `,
        actions: [
            { text: '取消', variant: 'text' },
            { 
                text: '确定', 
                variant: 'tonal',
                onClick: async (dialog) => {
                    const password = dialog.querySelector('#change-password').value;
                    if (!password) {
                        showSnackbar('密码不能为空');
                        return false;
                    }
                    try {
                        const data = await apiPut('/api/users/' + encodeURIComponent(username) + '/password', {
                            password: password
                        });
                        if (data.code === 0) {
                            showSnackbar('修改成功');
                        } else {
                            showSnackbar(data.message || '修改失败');
                            return false;
                        }
                    } catch (e) {
                        showSnackbar('修改失败: ' + e.message);
                        return false;
                    }
                }
            }
        ]
    });
}

async function deleteUser(username) {
    try {
        await mdui.confirm({
            headline: '删除用户',
            description: '确定要删除用户：' + username + ' 吗？此操作不可恢复。',
            confirmText: '删除',
            cancelText: '取消'
        });
    } catch {
        return;
    }

    try {
        const data = await apiDelete('/api/users/' + encodeURIComponent(username));
        if (data.code === 0) {
            showSnackbar('删除成功');
            loadUsers();
        } else {
            showSnackbar(data.message || '删除失败');
        }
    } catch (e) {
        showSnackbar('删除失败: ' + e.message);
    }
}

function renderAbout(container) {
    container.innerHTML = `
        <div class="page-title">关于</div>
        <div class="about-container">
            <mdui-card variant="outlined" class="about-card">
                <div class="about-logo">
                    <mdui-icon name="smart_toy" style="font-size:48px;color:var(--mdui-color-primary)"></mdui-icon>
                </div>
                <div class="about-title">ShizukuBot</div>
                <div class="about-subtitle">基于 Go + MDUI2 的 QQ 机器人管理框架</div>
                <div class="version-info">
                    <div class="version-item">
                        <div class="version-label">Core</div>
                        <div class="version-value">1.0.0</div>
                    </div>
                    <div class="version-item">
                        <div class="version-label">WebUI</div>
                        <div class="version-value">1.0.0</div>
                    </div>
                </div>
                <mdui-divider></mdui-divider>
                <div style="text-align:left;margin-top:24px">
                    <div class="tech-stack-title">技术栈</div>
                    <div class="tech-grid">
                        <div class="tech-item">
                            <mdui-icon name="terminal"></mdui-icon>
                            <span>Go</span>
                        </div>
                        <div class="tech-item">
                            <mdui-icon name="widgets"></mdui-icon>
                            <span>MDUI2</span>
                        </div>
                        <div class="tech-item">
                            <mdui-icon name="show_chart"></mdui-icon>
                            <span>Chart.js</span>
                        </div>
                        <div class="tech-item">
                            <mdui-icon name="http"></mdui-icon>
                            <span>gorilla/mux</span>
                        </div>
                        <div class="tech-item">
                            <mdui-icon name="smart_toy"></mdui-icon>
                            <span>botgo SDK</span>
                        </div>
                        <div class="tech-item">
                            <mdui-icon name="webhook"></mdui-icon>
                            <span>Webhook</span>
                        </div>
                        <div class="tech-item">
                            <mdui-icon name="data_object"></mdui-icon>
                            <span>JSON Config</span>
                        </div>
                        <div class="tech-item">
                            <mdui-icon name="devices"></mdui-icon>
                            <span>Web Components</span>
                        </div>
                    </div>
                </div>
            </mdui-card>
        </div>
    `;
}

// ==================== 机器人管理页 ====================
let allPluginsCache = [];

async function loadAllPlugins() {
    try {
        const data = await apiGet('/api/plugins');
        allPluginsCache = (data.code === 0 ? data.data : data) || [];
    } catch (e) {
        allPluginsCache = [];
    }
    return allPluginsCache;
}

async function renderBots(container) {
    container.innerHTML = `
        <div class="page-title">
            机器人管理            <mdui-button variant="filled" icon="add" id="add-bot-btn" style="float:right">添加机器人</mdui-button>
        </div>
        <div id="bots-manage-list" style="display:flex;flex-direction:column;gap:16px">
            <div style="text-align:center;color:var(--mdui-color-on-surface-variant);padding:40px 0">加载中...</div>
        </div>
    `;

    document.getElementById('add-bot-btn')?.addEventListener('click', () => showBotDialog(null, container));

    await loadAllPlugins();
    await loadBotsManageList(container);
}

async function loadBotsManageList(container) {
    const listEl = document.getElementById('bots-manage-list');
    if (!listEl) return;

    try {
        const data = await apiGet('/api/bots');
        const bots = (data.code === 0 ? data.data : data) || [];

        if (bots.length === 0) {
            listEl.innerHTML = '<div style="text-align:center;color:var(--mdui-color-on-surface-variant);padding:40px 0">暂无机器人，点击右上角添加</div>';
            return;
        }

        listEl.innerHTML = bots.map(bot => renderBotCard(bot)).join('');

        // 绑定按钮事件
        listEl.querySelectorAll('[data-action]').forEach(btn => {
            btn.addEventListener('click', async () => {
                const id = btn.dataset.id;
                const action = btn.dataset.action;
                btn.loading = true;
                try {
                    if (action === 'start') {
                        await apiPost(`/api/bots/${id}/start`, {});
                    } else if (action === 'stop') {
                        await apiPost(`/api/bots/${id}/stop`, {});
                    } else if (action === 'edit') {
                        showBotDialog(id, container);
                    } else if (action === 'plugins') {
                        showBotPluginsDialog(id, container);
                    } else if (action === 'config') {
                        showBotConfigDialog(id, container);
                    } else if (action === 'delete') {
                        try {
                            await mdui.confirm({
                                headline: '删除机器人',
                                description: '确定删除机器人：' + id + ' 吗？此操作不可恢复。',
                                confirmText: '删除',
                                cancelText: '取消'
                            });
                            await apiDelete(`/api/bots/${id}`);
                            await loadBotsManageList(container);
                        } catch {
                            // 用户取消
                        }
                    }
                } catch (e) {
                    showSnackbar('操作失败: ' + e.message);
                }
                btn.loading = false;
            });
        });
    } catch (e) {
        listEl.innerHTML = '<div style="text-align:center;color:var(--mdui-color-error);padding:40px 0">加载失败: ' + escapeHtml(e.message) + '</div>';
    }
}

function renderBotCard(bot) {
    const statusColor = bot.running ? '#4caf50' : '#9e9e9e';
    const statusText = bot.running ? '运行中' : '已停止';
    const modeText = bot.mode === 'websocket' ? 'WebSocket' : 'Webhook';
    const modeColor = bot.mode === 'websocket' ? '#1976d2' : '#f57c00';
    const btnText = bot.running ? '停止' : '启动';
    const btnIcon = bot.running ? 'stop' : 'play_arrow';
    const btnVariant = bot.running ? 'outlined' : 'filled';
    const btnAction = bot.running ? 'stop' : 'start';
    const pluginCount = bot.plugins && bot.plugins.length ? bot.plugins.length : '全部';
    const prefix = bot.command_prefix || '/';

    return `
        <mdui-card variant="outlined" style="padding:16px">
            <div style="display:flex;justify-content:space-between;align-items:flex-start;flex-wrap:wrap;gap:12px">
                <div style="flex:1;min-width:250px">
                    <div style="display:flex;align-items:center;gap:8px;margin-bottom:8px">
                        <span class="bot-status-dot" style="background:${statusColor}"></span>
                        <span style="font-size:18px;font-weight:500">${escapeHtml(bot.name || bot.id)}</span>
                        <span class="bot-mode" style="color:${modeColor};font-size:13px">${modeText}</span>
                        <span style="font-size:13px;color:var(--mdui-color-on-surface-variant)">${statusText}</span>
                    </div>
                    <div style="color:var(--mdui-color-on-surface-variant);font-size:13px;line-height:1.8">
                        <div>ID: ${escapeHtml(bot.id)}</div>
                        <div>AppID: ${escapeHtml(bot.app_id)}</div>
                        <div>指令前缀: <code style="background:var(--mdui-color-surface-variant);padding:2px 6px;border-radius:4px">${escapeHtml(prefix)}</code></div>
                        <div>装载插件: ${pluginCount}${pluginCount !== '全部' ? ' 个' : ''}</div>
                    </div>
                </div>
                <div style="display:flex;gap:8px;flex-wrap:wrap">
                    <mdui-button variant="${btnVariant}" icon="${btnIcon}" data-action="${btnAction}" data-id="${bot.id}">${btnText}</mdui-button>
                    <mdui-button variant="outlined" icon="extension" data-action="plugins" data-id="${bot.id}">插件</mdui-button>
                    <mdui-button variant="outlined" icon="tune" data-action="config" data-id="${bot.id}">配置</mdui-button>
                    <mdui-button variant="outlined" icon="edit" data-action="edit" data-id="${bot.id}">编辑</mdui-button>
                    <mdui-button-icon icon="delete" data-action="delete" data-id="${bot.id}" style="color:var(--mdui-color-error)"></mdui-button-icon>
                </div>
            </div>
        </mdui-card>
    `;
}

// 添加/编辑机器人对话框
function showBotDialog(botId, container) {
    const isEdit = !!botId;
    let bot = { id: '', name: '', app_id: '', app_secret: '', mode: 'websocket', enabled: true };

    if (isEdit) {
        const data = apiGet('/api/bots/' + botId);
        data.then(d => {
            const b = (d.code === 0 ? d.data : d);
            renderForm(b || bot);
        });
    } else {
        renderForm(bot);
    }

    function renderForm(b) {
        mdui.dialog({
            headline: isEdit ? '编辑机器人' : '添加机器人',
            body: `
                <div style="display:flex;flex-direction:column;gap:12px;padding:8px 0">
                    <mdui-text-field label="机器人ID" value="${escapeHtml(b.id)}" ${isEdit ? 'disabled' : ''} id="form-id"></mdui-text-field>
                    <mdui-text-field label="名称" value="${escapeHtml(b.name)}" id="form-name"></mdui-text-field>
                    <mdui-text-field label="AppID" value="${escapeHtml(b.app_id)}" id="form-appid"></mdui-text-field>
                    <mdui-text-field label="AppSecret" value="${escapeHtml(b.app_secret || '')}" id="form-secret" type="password" toggle-password></mdui-text-field>
                    <mdui-select label="连接模式" value="${b.mode || 'websocket'}" id="form-mode">
                        <mdui-menu-item value="websocket">WebSocket</mdui-menu-item>
                        <mdui-menu-item value="webhook">Webhook</mdui-menu-item>
                    </mdui-select>
                    <mdui-switch ${b.enabled ? 'checked' : ''} id="form-enabled">启用</mdui-switch>
                </div>
            `,
            actions: [
                { text: '取消', variant: 'text' },
                { 
                    text: '保存', 
                    variant: 'filled',
                    onClick: async (dialog) => {
                        const payload = {
                            id: dialog.querySelector('#form-id').value,
                            name: dialog.querySelector('#form-name').value,
                            app_id: dialog.querySelector('#form-appid').value,
                            app_secret: dialog.querySelector('#form-secret').value,
                            mode: dialog.querySelector('#form-mode').value,
                            enabled: dialog.querySelector('#form-enabled').checked
                        };
                        if (!payload.id || !payload.app_id || !payload.app_secret) {
                            showSnackbar('ID、AppID、AppSecret不能为空');
                            return false;
                        }
                        try {
                            if (isEdit) {
                                await apiPut('/api/bots/' + botId, payload);
                            } else {
                                await apiPost('/api/bots', payload);
                            }
                            await loadBotsManageList(container);
                            showSnackbar(isEdit ? '已更新' : '已添加');
                        } catch (e) {
                            showSnackbar('保存失败: ' + e.message);
                            return false;
                        }
                    }
                }
            ]
        });
    }
}

// 机器人插件配置对话框（多选）
function showBotPluginsDialog(botId, container) {
    apiGet('/api/bots/' + botId).then(data => {
        const bot = (data.code === 0 ? data.data : data) || {};
        const selectedIds = new Set(bot.plugins || []);
        const plugins = allPluginsCache;

        mdui.dialog({
            headline: '配置 ' + escapeHtml(bot.name || botId) + ' 的插件',
            body: `
                <div style="padding:8px 0">
                    <div style="margin-bottom:12px;color:var(--mdui-color-on-surface-variant);font-size:13px">
                        勾选要装载的插件，不勾选任何插件则装载所有启用的插件
                    </div>
                    <div id="plugin-checkbox-list" style="display:flex;flex-direction:column;gap:4px;max-height:400px;overflow-y:auto">
                        ${plugins.length === 0 ? '<div style="padding:20px;text-align:center;color:var(--mdui-color-on-surface-variant)">暂无插件</div>' :
                            plugins.map(p => `
                                <mdui-checkbox ${selectedIds.has(p.id) ? 'checked' : ''} value="${escapeHtml(p.id)}" id="plugin-${escapeHtml(p.id)}">
                                    ${escapeHtml(p.name || p.id)}${p.enabled ? '' : ' (未启用)'}
                                </mdui-checkbox>
                            `).join('')}
                    </div>
                </div>
            `,
            actions: [
                { text: '取消', variant: 'text' },
                { 
                    text: '保存', 
                    variant: 'filled',
                    onClick: async (dialog) => {
                        const checked = [];
                        plugins.forEach(p => {
                            const cb = dialog.querySelector('#plugin-' + CSS.escape(p.id));
                            if (cb && cb.checked) checked.push(p.id);
                        });
                        try {
                            await apiPut('/api/bots/' + botId, { plugins: checked });
                            await loadBotsManageList(container);
                            showSnackbar('插件配置已保存');
                        } catch (e) {
                            showSnackbar('保存失败: ' + e.message);
                            return false;
                        }
                    }
                }
            ]
        });
    });
}

// 机器人独立配置对话框
function showBotConfigDialog(botId, container) {
    apiGet('/api/bots/' + botId).then(data => {
        const bot = (data.code === 0 ? data.data : data) || {};

        mdui.dialog({
            headline: escapeHtml(bot.name || botId) + ' - 配置',
            body: `
                <div style="display:flex;flex-direction:column;gap:12px;padding:8px 0">
                    <mdui-text-field label="指令前缀" value="${escapeHtml(bot.command_prefix || '/')}" id="cfg-prefix" helper="符号 / ! #"></mdui-text-field>
                    <mdui-text-field label="管理员用户ID（逗号分隔）" value="${escapeHtml((bot.admin_users || []).join(','))}" id="cfg-admins" helper="多个ID用英文逗号分隔"></mdui-text-field>
                </div>
            `,
            actions: [
                { text: '取消', variant: 'text' },
                { 
                    text: '保存', 
                    variant: 'filled',
                    onClick: async (dialog) => {
                        const prefix = dialog.querySelector('#cfg-prefix').value || '/';
                        const adminsStr = dialog.querySelector('#cfg-admins').value || '';
                        const admins = adminsStr.split(',').map(s => s.trim()).filter(s => s);
                        try {
                            await apiPut('/api/bots/' + botId, { command_prefix: prefix, admin_users: admins });
                            await loadBotsManageList(container);
                            showSnackbar('配置已保存');
                        } catch (e) {
                            showSnackbar('保存失败: ' + e.message);
                            return false;
                        }
                    }
                }
            ]
        });
    });
}

// ==================== 沙盒页面 ====================
let sandboxType = 'c2c';
let sandboxMessages = [];
let sandboxPluginId = ''; // 空表示执行所有启用插件
let sandboxPluginsCache = [];

async function renderSandbox(container) {
    // 沙盒使用虚拟机器人，不依赖生产环境的真实机器人
    const sandboxBotId = 'sandbox';

    // 加载插件列表
    try {
        const resp = await fetch(`/api/bots/${sandboxBotId}/sandbox/plugins`);
        const data = await resp.json();
        sandboxPluginsCache = Array.isArray(data) ? data : (data.data || []);
    } catch (e) {
        sandboxPluginsCache = [];
    }

    const pluginOptions = [
        { id: '', name: '全部启用插件' },
        ...sandboxPluginsCache.map(p => ({ id: p.id, name: p.name || p.id, enabled: p.enabled }))
    ];

    container.innerHTML = `
        <div class="page-title">沙盒测试</div>
        <div style="margin-bottom:16px;display:flex;gap:12px;align-items:center;flex-wrap:wrap">
            <mdui-segmented-button-group value="${sandboxType}" id="sandbox-type-select">
                <mdui-segmented-button value="c2c">私聊</mdui-segmented-button>
                <mdui-segmented-button value="group">群聊</mdui-segmented-button>
            </mdui-segmented-button-group>
            <mdui-select id="sandbox-plugin-select" label="测试插件" value="${sandboxPluginId}" style="min-width:200px">
                ${pluginOptions.map(p => `<mdui-menu-item value="${escapeHtml(p.id)}">${escapeHtml(p.name)}${p.enabled === false ? ' (未启用)' : ''}</mdui-menu-item>`).join('')}
            </mdui-select>
            <mdui-button variant="outlined" icon="delete_sweep" id="sandbox-clear">清空对话</mdui-button>
        </div>
        <mdui-card variant="outlined" style="display:flex;flex-direction:column;height:calc(100vh - 220px);overflow:hidden">
            <div id="sandbox-chat" style="flex:1;overflow-y:auto;padding:16px;display:flex;flex-direction:column;gap:8px">
                ${sandboxMessages.length === 0 ? '<div style="text-align:center;color:var(--mdui-color-on-surface-variant);padding:40px 0">在下方输入消息测试机器人回复</div>' : ''}
                ${sandboxMessages.map(m => renderSandboxMessage(m)).join('')}
            </div>
            <mdui-divider></mdui-divider>
            <div style="padding:12px;display:flex;gap:8px;align-items:center">
                <mdui-text-field id="sandbox-input" label="输入消息" variant="outlined" style="flex:1" rows="1"></mdui-text-field>
                <mdui-button-icon icon="send" id="sandbox-send"></mdui-button-icon>
            </div>
        </mdui-card>
    `;

    const typeSelect = document.getElementById('sandbox-type-select');
    const pluginSelect = document.getElementById('sandbox-plugin-select');
    const chatArea = document.getElementById('sandbox-chat');
    const inputField = document.getElementById('sandbox-input');
    const sendBtn = document.getElementById('sandbox-send');
    const clearBtn = document.getElementById('sandbox-clear');

    typeSelect?.addEventListener('change', (e) => { sandboxType = e.target.value; });
    pluginSelect?.addEventListener('change', (e) => { sandboxPluginId = e.target.value; });

    clearBtn?.addEventListener('click', () => {
        sandboxMessages = [];
        renderSandbox(container);
    });

    const sendMessage = async () => {
        const content = inputField?.value?.trim();
        if (!content) return;

        sandboxMessages.push({ type: 'user', content, sandboxType });
        inputField.value = '';

        chatArea.innerHTML = sandboxMessages.map(m => renderSandboxMessage(m)).join('');
        chatArea.scrollTop = chatArea.scrollHeight;

        try {
            const resp = await fetch(`/api/bots/${sandboxBotId}/sandbox/send`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ type: sandboxType, content, plugin_id: sandboxPluginId })
            });
            const data = await resp.json();

            if (data.replies && data.replies.length > 0) {
                data.replies.forEach(reply => {
                    sandboxMessages.push({ type: 'bot', content: reply });
                });
            } else {
                sandboxMessages.push({ type: 'bot', content: '(无回复)' });
            }
        } catch (e) {
            sandboxMessages.push({ type: 'error', content: '请求失败: ' + e.message });
        }

        chatArea.innerHTML = sandboxMessages.map(m => renderSandboxMessage(m)).join('');
        chatArea.scrollTop = chatArea.scrollHeight;
    };

    sendBtn?.addEventListener('click', sendMessage);
    inputField?.addEventListener('keydown', (e) => {
        if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault();
            sendMessage();
        }
    });
}

function renderSandboxMessage(msg) {
    if (msg.type === 'user') {
        const label = msg.sandboxType === 'group' ? '群聊' : '私聊';
        return `<div style="align-self:flex-end;max-width:70%">
            <div style="font-size:12px;color:var(--mdui-color-on-surface-variant);text-align:right;margin-bottom:4px">${label} - 数据</div>
            <div style="background:var(--mdui-color-primary-container);color:var(--mdui-color-on-primary-container);padding:10px 14px;border-radius:12px 12px 4px 12px;word-break:break-word">${escapeHtml(msg.content)}</div>
        </div>`;
    } else if (msg.type === 'bot') {
        return `<div style="align-self:flex-start;max-width:70%">
            <div style="font-size:12px;color:var(--mdui-color-on-surface-variant);margin-bottom:4px">机器人</div>
            <div style="background:var(--mdui-color-secondary-container);color:var(--mdui-color-on-secondary-container);padding:10px 14px;border-radius:12px 12px 12px 4px;word-break:break-word">${escapeHtml(msg.content)}</div>
        </div>`;
    } else {
        return `<div style="align-self:center;max-width:80%">
            <div style="background:var(--mdui-color-error-container);color:var(--mdui-color-on-error-container);padding:8px 14px;border-radius:8px;font-size:13px;word-break:break-word">${escapeHtml(msg.content)}</div>
        </div>`;
    }
}

// ==================== 工具函数 ====================
function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

function showSnackbar(message) {
    if (window.mdui && mdui.snackbar) {
        mdui.snackbar({ message: message });
    } else if (window.mdui && mdui.alert) {
        mdui.alert({ headline: '提示', description: message, confirmText: '确定' });
    } else {
        window.alert(message);
    }
}

// ==================== 初始化 ====================
document.addEventListener('DOMContentLoaded', () => {
    // 移动端导航项点击
    document.querySelectorAll('.nav-item').forEach(item => {
        item.addEventListener('click', () => {
            navigateTo(item.dataset.route);
            // 点击后关闭移动端抽屉
            const drawer = document.querySelector('.mobile-nav');
            if (drawer) drawer.open = false;
        });
    });

    // 桌面端 NavigationRail change 事件
    const rail = document.querySelector('.desktop-nav');
    if (rail) {
        rail.addEventListener('change', (e) => {
            const selectedItem = rail.querySelector(`mdui-navigation-rail-item[value="${rail.value}"]`);
            if (selectedItem) {
                navigateTo(selectedItem.dataset.route);
            }
        });
    }

    const navDrawer = document.querySelector('.mobile-nav');
    const menuToggle = document.getElementById('menu-toggle');
    if (menuToggle && navDrawer) {
        menuToggle.addEventListener('click', () => {
            navDrawer.open = !navDrawer.open;
        });
    }

    // 登出
    const logoutBtn = document.getElementById('logout-btn');
    if (logoutBtn) {
        logoutBtn.addEventListener('click', () => {
            window.location.href = '/logout';
        });
    }

    // 根据hash路由初始化
    const hash = window.location.hash.replace('#', '') || '/stats';
    navigateTo(hash);
});
